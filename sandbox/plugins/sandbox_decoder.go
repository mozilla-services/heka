/***** BEGIN LICENSE BLOCK *****
# This Source Code Form is subject to the terms of the Mozilla Public
# License, v. 2.0. If a copy of the MPL was not distributed with this file,
# You can obtain one at http://mozilla.org/MPL/2.0/.
#
# The Initial Developer of the Original Code is the Mozilla Foundation.
# Portions created by the Initial Developer are Copyright (C) 2013
# the Initial Developer. All Rights Reserved.
#
# Contributor(s):
#   Mike Trinkala (trink@mozilla.com)
#
# ***** END LICENSE BLOCK *****/

package plugins

import (
	"code.google.com/p/goprotobuf/proto"
	"errors"
	"fmt"
	"github.com/mozilla-services/heka/message"
	"github.com/mozilla-services/heka/pipeline"
	. "github.com/mozilla-services/heka/sandbox"
	"github.com/mozilla-services/heka/sandbox/lua"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"
)

// Decoder for converting structured/unstructured data into Heka messages.
type SandboxDecoder struct {
	sb                     Sandbox
	sbc                    *SandboxConfig
	processMessageCount    int64
	processMessageFailures int64
	processMessageSamples  int64
	processMessageDuration int64
	reportLock             sync.Mutex
	sample                 bool
	err                    error
	pack                   *pipeline.PipelinePack
}

func (pd *SandboxDecoder) ConfigStruct() interface{} {
	return &SandboxConfig{
		ModuleDirectory:  pipeline.GetHekaConfigDir("lua_modules"),
		MemoryLimit:      8 * 1024 * 1024,
		InstructionLimit: 1e6,
		OutputLimit:      63 * 1024,
	}
}

func (s *SandboxDecoder) Init(config interface{}) (err error) {
	if s.sb != nil {
		return // no-op already initialized
	}
	s.sbc = config.(*SandboxConfig)
	s.sbc.ScriptFilename = pipeline.GetHekaConfigDir(s.sbc.ScriptFilename)
	s.sample = true

	switch s.sbc.ScriptType {
	case "lua":
		s.sb, err = lua.CreateLuaSandbox(s.sbc)
		if err != nil {
			return
		}
	default:
		return fmt.Errorf("unsupported script type: %s", s.sbc.ScriptType)
	}
	err = s.sb.Init("")
	return
}

func (s *SandboxDecoder) SetDecoderRunner(dr pipeline.DecoderRunner) {
	s.sb.InjectMessage(func(payload, payload_type, payload_name string) int {
		if len(payload_type) == 0 { // heka protobuf message
			t := s.pack.Message.GetType()
			h := s.pack.Message.GetHostname()
			l := s.pack.Message.GetLogger()
			if nil != proto.Unmarshal([]byte(payload), s.pack.Message) {
				return 1
			}
			if s.pack.Message.GetType() == "" {
				s.pack.Message.SetType(t)
			}
			if s.pack.Message.GetHostname() == "" {
				s.pack.Message.SetHostname(h)
			}
			if s.pack.Message.GetLogger() == "" {
				s.pack.Message.SetLogger(l)
			}
		} else {
			s.pack.Message.SetPayload(payload)
			ptype, _ := message.NewField("payload_type", payload_type, "file-extension")
			s.pack.Message.AddField(ptype)
			pname, _ := message.NewField("payload_name", payload_name, "")
			s.pack.Message.AddField(pname)
		}
		return 0
	})
}

func (s *SandboxDecoder) Shutdown() {
	if s.sb != nil {
		s.sb.Destroy("")
		s.sb = nil
	}
}

func (s *SandboxDecoder) Decode(pack *pipeline.PipelinePack) (packs []*pipeline.PipelinePack, err error) {
	if s.sb == nil {
		err = s.err
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
	s.sample = 0 == rand.Intn(pipeline.DURATION_SAMPLE_DENOMINATOR)
	if retval > 0 {
		s.err = errors.New("fatal: " + s.sb.LastError())
		pipeline.Globals().ShutDown()
	}
	if retval < 0 {
		atomic.AddInt64(&s.processMessageFailures, 1)
		err = fmt.Errorf("Failed parsing: %s", s.pack.Message.GetPayload())
		return
	}
	packs = []*pipeline.PipelinePack{pack}
	err = s.err
	return
}

// Satisfies the `pipeline.ReportingPlugin` interface to provide sandbox state
// information to the Heka report and dashboard.
func (s *SandboxDecoder) ReportMsg(msg *message.Message) error {
	if s.sb == nil {
		return fmt.Errorf("Decoder is not running")
	}
	s.reportLock.Lock()
	defer s.reportLock.Unlock()

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
