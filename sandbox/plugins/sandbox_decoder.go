/***** BEGIN LICENSE BLOCK *****
# This Source Code Form is subject to the terms of the Mozilla Public
# License, v. 2.0. If a copy of the MPL was not distributed with this file,
# You can obtain one at http://mozilla.org/MPL/2.0/.
#
# The Initial Developer of the Original Code is the Mozilla Foundation.
# Portions created by the Initial Developer are Copyright (C) 2013-2015
# the Initial Developer. All Rights Reserved.
#
# Contributor(s):
#   Mike Trinkala (trink@mozilla.com)
#   Rob Miller (rmiller@mozilla.com)
#
# ***** END LICENSE BLOCK *****/

package plugins

import (
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"regexp"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gogo/protobuf/proto"
	"github.com/mozilla-services/heka/message"
	"github.com/mozilla-services/heka/pipeline"
	. "github.com/mozilla-services/heka/sandbox"
	"github.com/mozilla-services/heka/sandbox/lua"
	"github.com/pborman/uuid"
)

// Decoder for converting structured/unstructured data into Heka messages.
type SandboxDecoder struct {
	processMessageCount    int64
	processMessageFailures int64
	processMessageSamples  int64
	processMessageDuration int64
	sb                     Sandbox
	sbc                    *SandboxConfig
	preservationFile       string
	reportLock             sync.Mutex
	sample                 bool
	pack                   *pipeline.PipelinePack
	packs                  []*pipeline.PipelinePack
	dRunner                pipeline.DecoderRunner
	name                   string
	tz                     *time.Location
	sampleDenominator      int
	pConfig                *pipeline.PipelineConfig
}

func (s *SandboxDecoder) ConfigStruct() interface{} {
	return NewSandboxConfig(s.pConfig.Globals)
}

func (s *SandboxDecoder) SetName(name string) {
	re := regexp.MustCompile("\\W")
	s.name = re.ReplaceAllString(name, "_")
}

// Heka will call this before calling any other methods to give us access to
// the pipeline configuration.
func (s *SandboxDecoder) SetPipelineConfig(pConfig *pipeline.PipelineConfig) {
	s.pConfig = pConfig
}

func (s *SandboxDecoder) Init(config interface{}) (err error) {
	s.sbc = config.(*SandboxConfig)
	globals := s.pConfig.Globals
	s.sbc.ScriptFilename = globals.PrependShareDir(s.sbc.ScriptFilename)
	s.sbc.PluginType = "decoder"
	s.sampleDenominator = globals.SampleDenominator

	s.tz = time.UTC
	if tz, ok := s.sbc.Config["tz"]; ok {
		s.tz, err = time.LoadLocation(tz.(string))
		if err != nil {
			return
		}
	}

	data_dir := globals.PrependBaseDir(DATA_DIR)
	if !fileExists(data_dir) {
		err = os.MkdirAll(data_dir, 0700)
		if err != nil {
			return
		}
	}

	switch s.sbc.ScriptType {
	case "lua":
	default:
		return fmt.Errorf("unsupported script type: %s", s.sbc.ScriptType)
	}

	s.sample = true
	return
}

func copyMessageHeaders(dst *message.Message, src *message.Message) {
	if src == nil || dst == nil || src == dst {
		return
	}

	if src.Timestamp != nil {
		dst.SetTimestamp(*src.Timestamp)
	} else {
		dst.Timestamp = nil
	}
	if src.Type != nil {
		dst.SetType(*src.Type)
	} else {
		dst.Type = nil
	}
	if src.Logger != nil {
		dst.SetLogger(*src.Logger)
	} else {
		dst.Logger = nil
	}
	if src.Severity != nil {
		dst.SetSeverity(*src.Severity)
	} else {
		dst.Severity = nil
	}
	if src.Pid != nil {
		dst.SetPid(*src.Pid)
	} else {
		dst.Pid = nil
	}
	if src.Hostname != nil {
		dst.SetHostname(*src.Hostname)
	} else {
		dst.Hostname = nil
	}
}

func (s *SandboxDecoder) SetDecoderRunner(dr pipeline.DecoderRunner) {
	if s.sb != nil {
		return // no-op already initialized
	}

	s.dRunner = dr
	var original *message.Message
	var err error

	switch s.sbc.ScriptType {
	case "lua":
		s.sb, err = lua.CreateLuaSandbox(s.sbc)
	default:
		err = fmt.Errorf("unsupported script type: %s", s.sbc.ScriptType)
	}

	if err == nil {
		s.preservationFile = filepath.Join(s.pConfig.Globals.PrependBaseDir(DATA_DIR),
			dr.Name()+DATA_EXT)
		if s.sbc.PreserveData && fileExists(s.preservationFile) {
			err = s.sb.Init(s.preservationFile)
		} else {
			err = s.sb.Init("")
		}
	}
	if err != nil {
		dr.LogError(err)
		if s.sb != nil {
			s.sb.Destroy("")
			s.sb = nil
		}
		s.pConfig.Globals.ShutDown()
		return
	}

	s.sb.InjectMessage(func(payload, payload_type, payload_name string) int {
		if s.pack == nil {
			s.pack = dr.NewPack()
			if s.pack == nil {
				return 5 // We're aborting, exit out.
			}
			if original == nil && len(s.packs) > 0 {
				original = s.packs[0].Message // payload injections have the original header data in the first pack
			}
		} else {
			original = nil // processing a new message, clear the old message
		}
		if len(payload_type) == 0 { // heka protobuf message
			// write protobuf encoding to MsgBytes
			needed := len(payload)
			if cap(s.pack.MsgBytes) < needed {
				s.pack.MsgBytes = make([]byte, len(payload))
			} else {
				s.pack.MsgBytes = s.pack.MsgBytes[:len(payload)]
			}
			copy(s.pack.MsgBytes, payload)
			s.pack.TrustMsgBytes = true

			if original == nil {
				original = new(message.Message)
				copyMessageHeaders(original, s.pack.Message) // save off the header values since unmarshal will wipe them out
			}
			if nil != proto.Unmarshal(s.pack.MsgBytes, s.pack.Message) {
				return 1
			}
			if s.tz != time.UTC {
				const layout = "2006-01-02T15:04:05.999999999" // remove the incorrect UTC tz info
				t := time.Unix(0, s.pack.Message.GetTimestamp())
				t = t.In(time.UTC)
				ct, _ := time.ParseInLocation(layout, t.Format(layout), s.tz)
				s.pack.Message.SetTimestamp(ct.UnixNano())
				s.pack.TrustMsgBytes = false
			}
		} else {
			s.pack.TrustMsgBytes = false
			s.pack.Message.SetPayload(payload)
			ptype, _ := message.NewField("payload_type", payload_type, "file-extension")
			s.pack.Message.AddField(ptype)
			pname, _ := message.NewField("payload_name", payload_name, "")
			s.pack.Message.AddField(pname)
		}
		if original != nil {
			// if future injections fail to set the standard headers, use the values
			// from the original message.
			if s.pack.Message.Uuid == nil {
				s.pack.Message.SetUuid(uuid.NewRandom()) // UUID should always be unique
				s.pack.TrustMsgBytes = false
			}
			if s.pack.Message.Timestamp == nil {
				s.pack.Message.SetTimestamp(original.GetTimestamp())
				s.pack.TrustMsgBytes = false
			}
			if s.pack.Message.Type == nil {
				s.pack.Message.SetType(original.GetType())
				s.pack.TrustMsgBytes = false
			}
			if s.pack.Message.Hostname == nil {
				s.pack.Message.SetHostname(original.GetHostname())
				s.pack.TrustMsgBytes = false
			}
			if s.pack.Message.Logger == nil {
				s.pack.Message.SetLogger(original.GetLogger())
				s.pack.TrustMsgBytes = false
			}
			if s.pack.Message.Severity == nil {
				s.pack.Message.SetSeverity(original.GetSeverity())
				s.pack.TrustMsgBytes = false
			}
			if s.pack.Message.Pid == nil {
				s.pack.Message.SetPid(original.GetPid())
				s.pack.TrustMsgBytes = false
			}
		}
		s.packs = append(s.packs, s.pack)
		s.pack = nil
		return 0
	})
}

func (s *SandboxDecoder) Shutdown() {
	err := s.destroy()
	if err != nil {
		s.dRunner.LogError(err)
	}
}

func (s *SandboxDecoder) destroy() error {
	s.reportLock.Lock()

	var err error
	if s.sb != nil {
		if s.sbc.PreserveData {
			err = s.sb.Destroy(s.preservationFile)
		} else {
			err = s.sb.Destroy("")
		}
		s.sb = nil
	}
	s.reportLock.Unlock()
	return err
}

func (s *SandboxDecoder) Decode(pack *pipeline.PipelinePack) (packs []*pipeline.PipelinePack,
	err error) {

	if s.sb == nil {
		err = fmt.Errorf("SandboxDecoder has been terminated")
		return
	}
	s.pack = pack
	atomic.AddInt64(&s.processMessageCount, 1)

	var startTime time.Time
	if s.sample {
		startTime = time.Now()
	}
	retval := s.sb.ProcessMessage(s.pack)
	if s.sample {
		duration := time.Since(startTime).Nanoseconds()
		s.reportLock.Lock()
		s.processMessageDuration += duration
		s.processMessageSamples++
		s.reportLock.Unlock()
	}
	s.sample = 0 == rand.Intn(s.sampleDenominator)
	if retval > 0 {
		err = fmt.Errorf("FATAL: %s", s.sb.LastError())
		s.dRunner.LogError(err)
		s.pConfig.Globals.ShutDown()
	}
	if retval < 0 {
		atomic.AddInt64(&s.processMessageFailures, 1)
		if s.pack != nil {
			err = fmt.Errorf("Failed parsing: %s payload: %s",
				s.sb.LastError(), s.pack.Message.GetPayload())
		} else {
			err = fmt.Errorf("Failed after a successful inject_message call: %s", s.sb.LastError())
		}
		if len(s.packs) > 1 {
			for _, p := range s.packs[1:] {
				p.Recycle(nil)
			}
		}
		s.packs = nil
	}
	if retval == 0 && s.pack != nil {
		// InjectMessage was never called, we're passing the original message
		// through.
		packs = append(packs, pack)
		s.pack = nil
	} else {
		packs = s.packs
	}
	s.packs = nil
	return packs, err
}

func (s *SandboxDecoder) EncodesMsgBytes() bool {
	return true
}

// Satisfies the `pipeline.ReportingPlugin` interface to provide sandbox state
// information to the Heka report and dashboard.
func (s *SandboxDecoder) ReportMsg(msg *message.Message) error {
	s.reportLock.Lock()
	defer s.reportLock.Unlock()

	if s.sb == nil {
		return fmt.Errorf("Decoder is not running")
	}

	message.NewIntField(msg, "Memory", int(s.sb.Usage(TYPE_MEMORY,
		STAT_CURRENT)), "B")
	message.NewIntField(msg, "MaxMemory", int(s.sb.Usage(TYPE_MEMORY,
		STAT_MAXIMUM)), "B")
	message.NewIntField(msg, "MaxInstructions", int(s.sb.Usage(
		TYPE_INSTRUCTIONS, STAT_MAXIMUM)), "count")
	message.NewIntField(msg, "MaxOutput", int(s.sb.Usage(TYPE_OUTPUT,
		STAT_MAXIMUM)), "B")
	message.NewInt64Field(msg, "ProcessMessageCount", atomic.LoadInt64(&s.processMessageCount), "count")
	message.NewInt64Field(msg, "ProcessMessageFailures", atomic.LoadInt64(&s.processMessageFailures), "count")
	message.NewInt64Field(msg, "ProcessMessageSamples", s.processMessageSamples, "count")

	var tmp int64 = 0
	if s.processMessageSamples > 0 {
		tmp = s.processMessageDuration / s.processMessageSamples
	}
	message.NewInt64Field(msg, "ProcessMessageAvgDuration", tmp, "ns")

	return nil
}

func init() {
	pipeline.RegisterPlugin("SandboxDecoder", func() interface{} {
		return new(SandboxDecoder)
	})
	pipeline.RegisterPlugin("SandboxFilter", func() interface{} {
		return new(SandboxFilter)
	})
	pipeline.RegisterPlugin("SandboxManagerFilter", func() interface{} {
		return new(SandboxManagerFilter)
	})
}
