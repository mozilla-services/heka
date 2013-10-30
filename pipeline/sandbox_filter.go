/***** BEGIN LICENSE BLOCK *****
# This Source Code Form is subject to the terms of the Mozilla Public
# License, v. 2.0. If a copy of the MPL was not distributed with this file,
# You can obtain one at http://mozilla.org/MPL/2.0/.
#
# The Initial Developer of the Original Code is the Mozilla Foundation.
# Portions created by the Initial Developer are Copyright (C) 2012
# the Initial Developer. All Rights Reserved.
#
# Contributor(s):
#   Mike Trinkala (trink@mozilla.com)
#   Rob Miller (rmiller@mozilla.com)
#
# ***** END LICENSE BLOCK *****/

package pipeline

import (
	"code.google.com/p/goprotobuf/proto"
	"fmt"
	"github.com/mozilla-services/heka/message"
	"github.com/mozilla-services/heka/sandbox"
	"github.com/mozilla-services/heka/sandbox/lua"
	"math/rand"
	"os"
	"path/filepath"
	"regexp"
	"sync"
	"sync/atomic"
	"time"
)

func fileExists(path string) bool {
	_, err := os.Stat(path)
	if err == nil {
		return true
	}
	return false
}

// Heka Filter plugin that acts as a wrapper for sandboxed filter scripts.
// Each sanboxed filter (whether statically defined in the config or
// dynamically loaded through the sandbox manager) maps to exactly one
// SandboxFilter instance.
type SandboxFilter struct {
	sb                     sandbox.Sandbox
	sbc                    *sandbox.SandboxConfig
	preservationFile       string
	processMessageCount    int64
	processMessageFailures int64
	injectMessageCount     int64
	processMessageSamples  int64
	processMessageDuration int64
	profileMessageSamples  int64
	profileMessageDuration int64
	timerEventSamples      int64
	timerEventDuration     int64
	reportLock             sync.Mutex
	name                   string
}

func (this *SandboxFilter) ConfigStruct() interface{} {
	return &sandbox.SandboxConfig{
		MemoryLimit:      32767,
		InstructionLimit: 1000,
		OutputLimit:      1024,
	}
}

func (this *SandboxFilter) SetName(name string) {
	re := regexp.MustCompile("\\W")
	this.name = re.ReplaceAllString(name, "_")
}

// Determines the script type and creates interpreter sandbox.
func (this *SandboxFilter) Init(config interface{}) (err error) {
	if this.sb != nil {
		return nil // no-op already initialized
	}
	this.sbc = config.(*sandbox.SandboxConfig)
	this.sbc.ScriptFilename = GetHekaConfigDir(this.sbc.ScriptFilename)

	switch this.sbc.ScriptType {
	case "lua":
		this.sb, err = lua.CreateLuaSandbox(this.sbc)
		if err != nil {
			return err
		}
	default:
		return fmt.Errorf("unsupported script type: %s", this.sbc.ScriptType)
	}

	this.preservationFile = filepath.Join(filepath.Dir(this.sbc.ScriptFilename), this.name+".data")
	if this.sbc.PreserveData && fileExists(this.preservationFile) {
		err = this.sb.Init(this.preservationFile)
	} else {
		err = this.sb.Init("")
	}

	return err
}

// Satisfies the `pipeline.ReportingPlugin` interface to provide sandbox state
// information to the Heka report and dashboard.
func (this *SandboxFilter) ReportMsg(msg *message.Message) error {
	this.reportLock.Lock()
	defer this.reportLock.Unlock()

	newIntField(msg, "Memory", int(this.sb.Usage(sandbox.TYPE_MEMORY,
		sandbox.STAT_CURRENT)), "B")
	newIntField(msg, "MaxMemory", int(this.sb.Usage(sandbox.TYPE_MEMORY,
		sandbox.STAT_MAXIMUM)), "B")
	newIntField(msg, "MaxInstructions", int(this.sb.Usage(
		sandbox.TYPE_INSTRUCTIONS, sandbox.STAT_MAXIMUM)), "count")
	newIntField(msg, "MaxOutput", int(this.sb.Usage(sandbox.TYPE_OUTPUT,
		sandbox.STAT_MAXIMUM)), "B")
	newInt64Field(msg, "ProcessMessageCount", atomic.LoadInt64(&this.processMessageCount), "count")
	newInt64Field(msg, "ProcessMessageFailures", atomic.LoadInt64(&this.processMessageFailures), "count")
	newInt64Field(msg, "InjectMessageCount", atomic.LoadInt64(&this.injectMessageCount), "count")
	newInt64Field(msg, "ProcessMessageSamples", this.processMessageSamples, "count")
	newInt64Field(msg, "TimerEventSamples", this.timerEventSamples, "count")

	var tmp int64 = 0
	if this.processMessageSamples > 0 {
		tmp = this.processMessageDuration / this.processMessageSamples
	}
	newInt64Field(msg, "ProcessMessageAvgDuration", tmp, "ns")

	tmp = 0
	if this.profileMessageSamples > 0 {
		newInt64Field(msg, "ProfileMessageSamples", this.profileMessageSamples, "count")
		tmp = this.profileMessageDuration / this.profileMessageSamples
		newInt64Field(msg, "ProfileMessageAvgDuration", tmp, "ns")
	}

	tmp = 0
	if this.timerEventSamples > 0 {
		tmp = this.timerEventDuration / this.timerEventSamples
	}
	newInt64Field(msg, "TimerEventAvgDuration", tmp, "ns")

	return nil
}

func (this *SandboxFilter) Run(fr FilterRunner, h PluginHelper) (err error) {
	inChan := fr.InChan()
	ticker := fr.Ticker()

	var (
		ok             = true
		terminated     = false
		sample         = true
		blocking       = false
		backpressure   = false
		pack           *PipelinePack
		retval         int
		msgLoopCount   uint
		injectionCount uint
		startTime      time.Time
		slowDuration   int64 = int64(Globals().MaxMsgProcessDuration)
		duration       int64
		capacity       = cap(inChan) - 1
	)

	this.sb.InjectMessage(func(payload, payload_type, payload_name string) int {
		if injectionCount == 0 {
			fr.LogError(fmt.Errorf("exceeded InjectMessage count"))
			return 1
		}
		injectionCount--
		pack := h.PipelinePack(msgLoopCount)
		if pack == nil {
			fr.LogError(fmt.Errorf("exceeded MaxMsgLoops = %d",
				Globals().MaxMsgLoops))
			return 1
		}
		if len(payload_type) == 0 { // heka protobuf message
			hostname := pack.Message.GetHostname()
			err := proto.Unmarshal([]byte(payload), pack.Message)
			if err == nil {
				// do not allow filters to override the following
				pack.Message.SetType("heka.sandbox." + pack.Message.GetType())
				pack.Message.SetLogger(fr.Name())
				pack.Message.SetHostname(hostname)
			} else {
				return 1
			}
		} else {
			pack.Message.SetType("heka.sandbox-output")
			pack.Message.SetLogger(fr.Name())
			pack.Message.SetPayload(payload)
			ptype, _ := message.NewField("payload_type", payload_type, "file-extension")
			pack.Message.AddField(ptype)
			pname, _ := message.NewField("payload_name", payload_name, "")
			pack.Message.AddField(pname)
		}
		if !fr.Inject(pack) {
			return 1
		}
		atomic.AddInt64(&this.injectMessageCount, 1)
		return 0
	})

	for ok {
		select {
		case pack, ok = <-inChan:
			if !ok {
				break
			}
			atomic.AddInt64(&this.processMessageCount, 1)
			injectionCount = Globals().MaxMsgProcessInject
			msgLoopCount = pack.MsgLoopCount

			// reading a channel length is generally fast ~1ns
			// we need to check the entire chain back to the router
			backpressure = len(inChan) >= capacity ||
				len(fr.MatchRunner().inChan) >= capacity ||
				len(h.PipelineConfig().router.InChan()) >= capacity

			// performing the timing is expensive ~40ns but if we are
			// backpressured we need a decent sample set before triggering
			// termination
			if sample ||
				(backpressure && this.processMessageSamples < int64(capacity)) ||
				this.sbc.Profile {
				startTime = time.Now()
				sample = true
			}
			retval = this.sb.ProcessMessage(pack.Message)
			if sample {
				duration = time.Since(startTime).Nanoseconds()
				this.reportLock.Lock()
				this.processMessageDuration += duration
				this.processMessageSamples++
				if this.sbc.Profile {
					this.profileMessageDuration = this.processMessageDuration
					this.profileMessageSamples = this.processMessageSamples
					if this.profileMessageSamples == int64(capacity)*10 {
						this.sbc.Profile = false
						// reset the normal sampling so it isn't heavily skewed by the profile values
						// i.e. process messages fast during profiling and then switch to malicious code
						this.processMessageDuration = this.profileMessageDuration / this.profileMessageSamples
						this.processMessageSamples = 1
					}
				}
				this.reportLock.Unlock()
			}
			if retval <= 0 {
				if backpressure && this.processMessageSamples >= int64(capacity) {
					fr.MatchRunner().reportLock.Lock()
					if this.processMessageDuration/this.processMessageSamples > slowDuration ||
						fr.MatchRunner().matchDuration/fr.MatchRunner().matchSamples > slowDuration/5 {
						terminated = true
						blocking = true
					}
					fr.MatchRunner().reportLock.Unlock()
				}
				if retval < 0 {
					atomic.AddInt64(&this.processMessageFailures, 1)
				}
				sample = 0 == rand.Intn(DURATION_SAMPLE_DENOMINATOR)
			} else {
				terminated = true
			}
			pack.Recycle()

		case t := <-ticker:
			injectionCount = Globals().MaxMsgTimerInject
			startTime = time.Now()
			if retval = this.sb.TimerEvent(t.UnixNano()); retval != 0 {
				terminated = true
			}
			duration = time.Since(startTime).Nanoseconds()
			this.reportLock.Lock()
			this.timerEventDuration += duration
			this.timerEventSamples++
			this.reportLock.Unlock()
		}

		if terminated {
			pack := h.PipelinePack(0)
			pack.Message.SetType("heka.sandbox-terminated")
			pack.Message.SetLogger(fr.Name())
			if blocking {
				pack.Message.SetPayload("sandbox is running slowly and blocking the router")
				// no lock on the ProcessMessage variables here because there are no active writers
				newInt64Field(pack.Message, "ProcessMessageCount", this.processMessageCount, "count")
				newInt64Field(pack.Message, "ProcessMessageFailures", this.processMessageFailures, "count")
				newInt64Field(pack.Message, "ProcessMessageSamples", this.processMessageSamples, "count")
				newInt64Field(pack.Message, "ProcessMessageAvgDuration",
					this.processMessageDuration/this.processMessageSamples, "ns")
				newInt64Field(pack.Message, "MatchSamples", fr.MatchRunner().matchSamples, "count")
				fr.MatchRunner().reportLock.Lock()
				newInt64Field(pack.Message, "MatchAvgDuration",
					fr.MatchRunner().matchDuration/fr.MatchRunner().matchSamples, "ns")
				fr.MatchRunner().reportLock.Unlock()
				newIntField(pack.Message, "FilterChanLength", len(inChan), "count")
				newIntField(pack.Message, "MatchChanLength", len(fr.MatchRunner().inChan), "count")
				newIntField(pack.Message, "RouterChanLength", len(h.PipelineConfig().router.InChan()), "count")
			} else {
				pack.Message.SetPayload(this.sb.LastError())
			}
			fr.Inject(pack)
			break
		}
	}

	if terminated {
		go h.PipelineConfig().RemoveFilterRunner(fr.Name())
		// recycle any messages until the matcher is torn down
		for pack = range inChan {
			pack.Recycle()
		}
	}

	if this.sbc.PreserveData {
		this.sb.Destroy(this.preservationFile)
	} else {
		this.sb.Destroy("")
	}
	this.sb = nil
	return
}
