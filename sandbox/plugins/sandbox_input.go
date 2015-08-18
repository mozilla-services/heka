/***** BEGIN LICENSE BLOCK *****
# This Source Code Form is subject to the terms of the Mozilla Public
# License, v. 2.0. If a copy of the MPL was not distributed with this file,
# You can obtain one at http://mozilla.org/MPL/2.0/.
#
# The Initial Developer of the Original Code is the Mozilla Foundation.
# Portions created by the Initial Developer are Copyright (C) 2015
# the Initial Developer. All Rights Reserved.
#
# Contributor(s):
#   Mike Trinkala (trink@mozilla.com)
#
# ***** END LICENSE BLOCK *****/

package plugins

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gogo/protobuf/proto"
	"github.com/mozilla-services/heka/message"
	"github.com/mozilla-services/heka/pipeline"
	. "github.com/mozilla-services/heka/sandbox"
	"github.com/mozilla-services/heka/sandbox/lua"
)

// Heka Input plugin that acts as a wrapper for sandboxed input scripts.
type SandboxInput struct {
	processMessageCount    int64
	processMessageFailures int64
	processMessageBytes    int64

	stopChan         chan struct{}
	sb               Sandbox
	sbc              *SandboxConfig
	preservationFile string
	reportLock       sync.Mutex
	name             string
	pConfig          *pipeline.PipelineConfig
	tz               *time.Location
}

// Heka will call this before calling any other methods to give us access to
// the pipeline configuration.
func (s *SandboxInput) SetPipelineConfig(pConfig *pipeline.PipelineConfig) {
	s.pConfig = pConfig
}

func (s *SandboxInput) ConfigStruct() interface{} {
	return NewSandboxConfig(s.pConfig.Globals)
}

func (s *SandboxInput) Init(config interface{}) (err error) {
	s.sbc = config.(*SandboxConfig)
	globals := s.pConfig.Globals
	s.sbc.ScriptFilename = globals.PrependShareDir(s.sbc.ScriptFilename)
	s.sbc.InstructionLimit = 0
	s.sbc.PluginType = "input"

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
		s.sb, err = lua.CreateLuaSandbox(s.sbc)
		if err != nil {
			return
		}
	default:
		return fmt.Errorf("unsupported script type: %s", s.sbc.ScriptType)
	}

	s.preservationFile = filepath.Join(data_dir, s.name+DATA_EXT)
	if s.sbc.PreserveData && fileExists(s.preservationFile) {
		err = s.sb.Init(s.preservationFile)
	} else {
		err = s.sb.Init("")
	}
	s.stopChan = make(chan struct{})

	return
}

func (s *SandboxInput) Run(ir pipeline.InputRunner, h pipeline.PluginHelper) (err error) {
	abortChan := s.pConfig.Globals.AbortChan()
	s.sb.InjectMessage(func(payload, payload_type, payload_name string) int {
		var pack *pipeline.PipelinePack
		select {
		case pack = <-ir.InChan():
		case <-abortChan:
			pack.Recycle(nil)
			return 5
		}
		if err := proto.Unmarshal([]byte(payload), pack.Message); err != nil {
			pack.Recycle(nil)
			return 1
		}
		if s.tz != time.UTC {
			const layout = "2006-01-02T15:04:05.999999999" // remove the incorrect UTC tz info
			t := time.Unix(0, pack.Message.GetTimestamp())
			t = t.In(time.UTC)
			ct, _ := time.ParseInLocation(layout, t.Format(layout), s.tz)
			pack.Message.SetTimestamp(ct.UnixNano())
		}
		if err := ir.Inject(pack); err != nil {
			pack.Recycle(nil)
			return 5
		}
		atomic.AddInt64(&s.processMessageCount, 1)
		atomic.AddInt64(&s.processMessageBytes, int64(len(payload)))
		return 0
	})

	ticker := ir.Ticker()

	for true {
		retval := s.sb.ProcessMessage(nil)
		if retval <= 0 { // Sandbox is in polling mode
			if retval < 0 {
				atomic.AddInt64(&s.processMessageFailures, 1)
				em := s.sb.LastError()
				if len(em) > 0 {
					ir.LogError(errors.New(em))
				}
			}
			if ticker == nil {
				ir.LogMessage("single run completed")
				break
			}
			select { // block until stop or poll interval
			case <-s.stopChan:
			case <-ticker:
			}
		} else { // Sandbox is shutting down
			em := s.sb.LastError()
			if !strings.HasSuffix(em, "shutting down") {
				ir.LogError(errors.New(em))
			}
			break
		}
	}

	return s.destroy()
}

func (s *SandboxInput) destroy() error {
	var err error

	s.reportLock.Lock()
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

func (s *SandboxInput) Stop() {
	s.reportLock.Lock()
	if s.sb != nil {
		s.sb.Stop()
	}
	s.reportLock.Unlock()
	close(s.stopChan)
}

// Satisfies the `pipeline.ReportingPlugin` interface to provide sandbox state
// information to the Heka report and dashboard.
func (s *SandboxInput) ReportMsg(msg *message.Message) error {
	s.reportLock.Lock()
	defer s.reportLock.Unlock()

	if s.sb == nil {
		return fmt.Errorf("Input is not running")
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
	message.NewInt64Field(msg, "ProcessMessageBytes", atomic.LoadInt64(&s.processMessageBytes), "B")

	return nil
}

func init() {
	pipeline.RegisterPlugin("SandboxInput", func() interface{} {
		return new(SandboxInput)
	})
}
