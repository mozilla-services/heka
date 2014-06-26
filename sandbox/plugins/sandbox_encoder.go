/***** BEGIN LICENSE BLOCK *****
# This Source Code Form is subject to the terms of the Mozilla Public
# License, v. 2.0. If a copy of the MPL was not distributed with this file,
# You can obtain one at http://mozilla.org/MPL/2.0/.
#
# The Initial Developer of the Original Code is the Mozilla Foundation.
# Portions created by the Initial Developer are Copyright (C) 2014
# the Initial Developer. All Rights Reserved.
#
# Contributor(s):
#   Mike Trinkala (trink@mozilla.com)
#   Rob Miller (rmiller@mozilla.com)
#
# ***** END LICENSE BLOCK *****/
package plugins

import (
	"errors"
	"fmt"
	"github.com/mozilla-services/heka/client"
	"github.com/mozilla-services/heka/message"
	"github.com/mozilla-services/heka/pipeline"
	"github.com/mozilla-services/heka/sandbox"
	"github.com/mozilla-services/heka/sandbox/lua"
	"math/rand"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"
)

type SandboxEncoder struct {
	processMessageCount    int64
	processMessageFailures int64
	processMessageSamples  int64
	processMessageDuration int64
	sb                     sandbox.Sandbox
	sbc                    *sandbox.SandboxConfig
	preservationFile       string
	reportLock             sync.Mutex
	sample                 bool
	name                   string
	tz                     *time.Location
	sampleDenominator      int
	output                 []byte
	injected               bool
	cEncoder               *client.ProtobufEncoder
}

// This duplicates most of the SandboxConfig just so we can add a single
// additional config option because struct embedding doesn't work with the
// TOML parser. :(
type SandboxEncoderConfig struct {
	ScriptType       string `toml:"script_type"`
	ScriptFilename   string `toml:"filename"`
	ModuleDirectory  string `toml:"module_directory"`
	PreserveData     bool   `toml:"preserve_data"`
	MemoryLimit      uint   `toml:"memory_limit"`
	InstructionLimit uint   `toml:"instruction_limit"`
	OutputLimit      uint   `toml:"output_limit"`
	Profile          bool
	Config           map[string]interface{}
}

func (s *SandboxEncoder) ConfigStruct() interface{} {
	return &SandboxEncoderConfig{
		ModuleDirectory:  pipeline.PrependShareDir("lua_modules"),
		MemoryLimit:      8 * 1024 * 1024,
		InstructionLimit: 1e6,
		OutputLimit:      63 * 1024,
		ScriptType:       "lua",
	}
}

// Implements WantsName interface so we'll have access to the plugin name
// before the Init method is called.
func (s *SandboxEncoder) SetName(name string) {
	s.name = name
}

func (s *SandboxEncoder) Init(config interface{}) (err error) {
	conf := config.(*SandboxEncoderConfig)
	s.sbc = &sandbox.SandboxConfig{
		ScriptType:       conf.ScriptType,
		ScriptFilename:   conf.ScriptFilename,
		ModuleDirectory:  conf.ModuleDirectory,
		PreserveData:     conf.PreserveData,
		MemoryLimit:      conf.MemoryLimit,
		InstructionLimit: conf.InstructionLimit,
		OutputLimit:      conf.OutputLimit,
		Profile:          conf.Profile,
		Config:           conf.Config,
	}
	s.sbc.ScriptFilename = pipeline.PrependShareDir(s.sbc.ScriptFilename)
	s.sampleDenominator = pipeline.Globals().SampleDenominator

	s.tz = time.UTC
	if tz, ok := s.sbc.Config["tz"]; ok {
		if s.tz, err = time.LoadLocation(tz.(string)); err != nil {
			return
		}
	}

	dataDir := pipeline.PrependBaseDir(sandbox.DATA_DIR)
	if !fileExists(dataDir) {
		if err = os.MkdirAll(dataDir, 0700); err != nil {
			return
		}
	}

	switch s.sbc.ScriptType {
	case "lua":
		s.sb, err = lua.CreateLuaSandbox(s.sbc)
	default:
		return fmt.Errorf("Unsupported script type: %s", s.sbc.ScriptType)
	}

	if err != nil {
		return fmt.Errorf("Sandbox creation failed: '%s'", err)
	}

	s.preservationFile = filepath.Join(dataDir, s.name+sandbox.DATA_EXT)
	if s.sbc.PreserveData && fileExists(s.preservationFile) {
		err = s.sb.Init(s.preservationFile, "encoder")
	} else {
		err = s.sb.Init("", "encoder")
	}
	if err != nil {
		return fmt.Errorf("Sandbox initialization failed: %s", err)
	}

	s.sb.InjectMessage(func(payload, payload_type, payload_name string) int {
		s.injected = true
		s.output = []byte(payload)
		return 0
	})
	s.sample = true
	s.cEncoder = client.NewProtobufEncoder(nil)
	return
}

func (s *SandboxEncoder) Stop() {
	if s.sb != nil {
		if s.sbc.PreserveData {
			s.sb.Destroy(s.preservationFile)
		} else {
			s.sb.Destroy("")
		}
	}
}

func (s *SandboxEncoder) Encode(pack *pipeline.PipelinePack) (output []byte, err error) {
	if s.sb == nil {
		err = errors.New("No sandbox.")
		return
	}
	atomic.AddInt64(&s.processMessageCount, 1)
	s.injected = false

	var startTime time.Time
	if s.sample {
		startTime = time.Now()
	}
	cowpack := new(pipeline.PipelinePack)
	cowpack.Message = pack.Message
	retval := s.sb.ProcessMessage(cowpack)
	if retval == 0 && !s.injected {
		// `inject_message` was never called, protobuf encode the copy on write
		// message.
		if s.output, err = s.cEncoder.EncodeMessage(cowpack.Message); err != nil {
			return
		}
	}

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
		return
	}
	if retval < 0 {
		atomic.AddInt64(&s.processMessageFailures, 1)
		err = errors.New("Failed serializing.")
		return
	}
	return s.output, nil
}

// Satisfies the `pipeline.ReportingPlugin` interface to provide sandbox state
// information to the Heka report and dashboard.
func (s *SandboxEncoder) ReportMsg(msg *message.Message) error {
	if s.sb == nil {
		return fmt.Errorf("Encoder is not running")
	}
	s.reportLock.Lock()
	defer s.reportLock.Unlock()

	message.NewIntField(msg, "Memory", int(s.sb.Usage(sandbox.TYPE_MEMORY,
		sandbox.STAT_CURRENT)), "B")
	message.NewIntField(msg, "MaxMemory", int(s.sb.Usage(sandbox.TYPE_MEMORY,
		sandbox.STAT_MAXIMUM)), "B")
	message.NewIntField(msg, "MaxInstructions", int(s.sb.Usage(
		sandbox.TYPE_INSTRUCTIONS, sandbox.STAT_MAXIMUM)), "count")
	message.NewIntField(msg, "MaxOutput", int(s.sb.Usage(sandbox.TYPE_OUTPUT,
		sandbox.STAT_MAXIMUM)), "B")
	message.NewInt64Field(msg, "ProcessMessageCount",
		atomic.LoadInt64(&s.processMessageCount), "count")
	message.NewInt64Field(msg, "ProcessMessageFailures",
		atomic.LoadInt64(&s.processMessageFailures), "count")
	message.NewInt64Field(msg, "ProcessMessageSamples",
		s.processMessageSamples, "count")

	var tmp int64 = 0
	if s.processMessageSamples > 0 {
		tmp = s.processMessageDuration / s.processMessageSamples
	}
	message.NewInt64Field(msg, "ProcessMessageAvgDuration", tmp, "ns")

	return nil
}

func init() {
	pipeline.RegisterPlugin("SandboxEncoder", func() interface{} {
		return new(SandboxEncoder)
	})
}
