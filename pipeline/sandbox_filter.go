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
	"fmt"
	"github.com/mozilla-services/heka/message"
	"github.com/mozilla-services/heka/sandbox"
	"github.com/mozilla-services/heka/sandbox/lua"
	"math/rand"
	"os"
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
	injectMessageCount     int64
	processMessageSamples  int64
	processMessageDuration time.Duration
	timerEventSamples      int64
	timerEventDuration     time.Duration
}

func (this *SandboxFilter) ConfigStruct() interface{} {
	return new(sandbox.SandboxConfig)
}

// Determines the script type and creates interpreter sandbox.
func (this *SandboxFilter) Init(config interface{}) (err error) {
	if this.sb != nil {
		return nil // no-op already initialized
	}
	conf := config.(*sandbox.SandboxConfig)
	this.sbc = conf

	switch this.sbc.ScriptType {
	case "lua":
		this.sb, err = lua.CreateLuaSandbox(this.sbc)
		if err != nil {
			return err
		}
	default:
		return fmt.Errorf("unsupported script type: %s", this.sbc.ScriptType)
	}

	this.preservationFile = this.sbc.ScriptFilename + ".data"
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
	newIntField(msg, "Memory", int(this.sb.Usage(sandbox.TYPE_MEMORY,
		sandbox.STAT_CURRENT)))
	newIntField(msg, "MaxMemory", int(this.sb.Usage(sandbox.TYPE_MEMORY,
		sandbox.STAT_MAXIMUM)))
	newIntField(msg, "MaxInstructions", int(this.sb.Usage(
		sandbox.TYPE_INSTRUCTIONS, sandbox.STAT_MAXIMUM)))
	newIntField(msg, "MaxOutput", int(this.sb.Usage(sandbox.TYPE_OUTPUT,
		sandbox.STAT_MAXIMUM)))
	newInt64Field(msg, "ProcessMessageCount", this.processMessageCount)
	newInt64Field(msg, "InjectMessageCount", this.injectMessageCount)

	var tmp int64 = 0
	if this.processMessageSamples > 0 {
		tmp = this.processMessageDuration.Nanoseconds() / this.processMessageSamples
	}
	newInt64Field(msg, "ProcessMessageAvgDuration", tmp)

	tmp = 0
	if this.timerEventSamples > 0 {
		tmp = this.timerEventDuration.Nanoseconds() / this.timerEventSamples
	}
	newInt64Field(msg, "TimerEventAvgDuration", tmp)

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
		plc            *PipelineCapture
		retval         int
		msgLoopCount   uint
		injectionCount uint
		startTime      time.Time
		slowDuration   int64 = 50000 // duration in nanoseconds (20K msg/sec)
		capacity             = cap(inChan) - 1
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
		pack.Message.SetType("heka.sandbox-output")
		pack.Message.SetLogger(fr.Name())
		pack.Message.SetPayload(payload)
		ptype, _ := message.NewField("payload_type", payload_type,
			message.Field_RAW)
		pack.Message.AddField(ptype)
		pname, _ := message.NewField("payload_name", payload_name,
			message.Field_RAW)
		pack.Message.AddField(pname)
		if !fr.Inject(pack) {
			return 1
		}
		this.injectMessageCount++
		return 0
	})

	for ok {
		select {
		case plc, ok = <-inChan:
			if !ok {
				break
			}
			this.processMessageCount++
			injectionCount = Globals().MaxMsgProcessInject
			msgLoopCount = plc.Pack.MsgLoopCount

			// reading a channel length is generally fast ~1ns
			// we need to check the entire chain back to the router
			backpressure = len(inChan) >= capacity ||
				len(fr.MatchRunner().inChan) >= capacity ||
				len(h.PipelineConfig().router.InChan()) >= capacity

			// performing the timing is expensive ~40ns but if we are
			// backpressured we need a decent sample set before triggering
			// termination
			if sample || (backpressure && this.processMessageSamples < int64(capacity)) {
				this.processMessageSamples++
				startTime = time.Now()
				sample = true
			}
			retval = this.sb.ProcessMessage(plc.Pack.Message, plc.Captures)
			if sample {
				this.processMessageDuration += time.Since(startTime)
			}
			if retval == 0 {
				if backpressure &&
					this.processMessageSamples >= int64(capacity) &&
					(this.processMessageDuration.Nanoseconds()/this.processMessageSamples > slowDuration ||
						fr.MatchRunner().matchDuration.Nanoseconds()/fr.MatchRunner().matchSamples > slowDuration/5) {
					terminated = true
					blocking = true
				}
				sample = 0 == rand.Intn(DURATION_SAMPLE_DENOMINATOR)
			} else {
				terminated = true
			}
			plc.Pack.Recycle()

		case t := <-ticker:
			this.timerEventSamples++
			injectionCount = Globals().MaxMsgTimerInject
			startTime = time.Now()
			if retval = this.sb.TimerEvent(t.UnixNano()); retval != 0 {
				terminated = true
			}
			this.timerEventDuration += time.Since(startTime)
		}

		if terminated {
			pack := h.PipelinePack(0)
			pack.Message.SetType("heka.sandbox-terminated")
			pack.Message.SetLogger(fr.Name())
			if blocking {
				pack.Message.SetPayload("sandbox is running slowly and blocking the router")
				newInt64Field(pack.Message, "ProcessMessageCount", this.processMessageCount)
				newInt64Field(pack.Message, "ProcessMessageSamples", this.processMessageSamples)
				newInt64Field(pack.Message, "ProcessMessageAvgDuration",
					this.processMessageDuration.Nanoseconds()/this.processMessageSamples)
				newInt64Field(pack.Message, "MatchSamples", fr.MatchRunner().matchSamples)
				newInt64Field(pack.Message, "MatchAvgDuration",
					fr.MatchRunner().matchDuration.Nanoseconds()/fr.MatchRunner().matchSamples)
				newIntField(pack.Message, "FilterChanLength", len(inChan))
				newIntField(pack.Message, "MatchChanLength", len(fr.MatchRunner().inChan))
				newIntField(pack.Message, "RouterChanLength", len(h.PipelineConfig().router.InChan()))
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
		for plc = range inChan {
			plc.Pack.Recycle()
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
