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

package pipeline

import (
	"code.google.com/p/goprotobuf/proto"
	"errors"
	"fmt"
	"github.com/mozilla-services/heka/message"
	"github.com/mozilla-services/heka/sandbox"
	"github.com/mozilla-services/heka/sandbox/lua"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"
)

// Decoder for converting structured/unstructured data into Heka messages.
type SandboxDecoder struct {
	sb                     sandbox.Sandbox
	sbc                    *sandbox.SandboxConfig
	processMessageCount    int64
	processMessageFailures int64
	processMessageSamples  int64
	processMessageDuration int64
	reportLock             sync.Mutex
	sample                 bool
	err                    error
	pack                   *PipelinePack
}

func (pd *SandboxDecoder) ConfigStruct() interface{} {
	return &sandbox.SandboxConfig{
		MemoryLimit:      8 * 1024 * 1024,
		InstructionLimit: 1e6,
		OutputLimit:      63 * 1024,
	}
}

func (s *SandboxDecoder) Init(config interface{}) (err error) {
	if s.sb != nil {
		return // no-op already initialized
	}
	s.sbc = config.(*sandbox.SandboxConfig)
	s.sbc.ScriptFilename = GetHekaConfigDir(s.sbc.ScriptFilename)
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

func (s *SandboxDecoder) SetDecoderRunner(dr DecoderRunner) {
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

func (s *SandboxDecoder) Decode(pack *PipelinePack) error {
	if s.sb == nil {
		return s.err
	}
	s.pack = pack
	atomic.AddInt64(&s.processMessageCount, 1)

	var startTime time.Time
	if s.sample {
		startTime = time.Now()
	}
	retval := s.sb.ProcessMessage(s.pack.Message)
	if s.sample {
		duration := time.Since(startTime).Nanoseconds()
		s.reportLock.Lock()
		s.processMessageDuration += duration
		s.processMessageSamples++
		s.reportLock.Unlock()
	}
	s.sample = 0 == rand.Intn(DURATION_SAMPLE_DENOMINATOR)
	if retval > 0 {
		s.err = errors.New(s.sb.LastError())
		s.Shutdown()
	}
	if retval < 0 {
		atomic.AddInt64(&s.processMessageFailures, 1)
		return fmt.Errorf("Failed parsing: %s", s.pack.Message.GetPayload())
	}
	return s.err
}

// Satisfies the `pipeline.ReportingPlugin` interface to provide sandbox state
// information to the Heka report and dashboard.
func (s *SandboxDecoder) ReportMsg(msg *message.Message) error {
	s.reportLock.Lock()
	defer s.reportLock.Unlock()

	newIntField(msg, "Memory", int(s.sb.Usage(sandbox.TYPE_MEMORY,
		sandbox.STAT_CURRENT)), "B")
	newIntField(msg, "MaxMemory", int(s.sb.Usage(sandbox.TYPE_MEMORY,
		sandbox.STAT_MAXIMUM)), "B")
	newIntField(msg, "MaxInstructions", int(s.sb.Usage(
		sandbox.TYPE_INSTRUCTIONS, sandbox.STAT_MAXIMUM)), "count")
	newIntField(msg, "MaxOutput", int(s.sb.Usage(sandbox.TYPE_OUTPUT,
		sandbox.STAT_MAXIMUM)), "B")
	newInt64Field(msg, "ProcessMessageCount", atomic.LoadInt64(&s.processMessageCount), "count")
	newInt64Field(msg, "ProcessMessageFailures", atomic.LoadInt64(&s.processMessageFailures), "count")
	newInt64Field(msg, "ProcessMessageSamples", s.processMessageSamples, "count")

	var tmp int64 = 0
	if s.processMessageSamples > 0 {
		tmp = s.processMessageDuration / s.processMessageSamples
	}
	newInt64Field(msg, "ProcessMessageAvgDuration", tmp, "ns")

	return nil
}
