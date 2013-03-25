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
	"github.com/mozilla-services/heka/sandbox"
	"github.com/mozilla-services/heka/sandbox/lua"
	"log"
	"os"
	"sync"
)

func fileExists(path string) bool {
	_, err := os.Stat(path)
	if err == nil {
		return true
	}
	return false
}

type SandboxFilterConfig struct {
	Sbc sandbox.SandboxConfig `json:"sandbox"`
}

type SandboxFilter struct {
	sb  sandbox.Sandbox
	sbc sandbox.SandboxConfig
}

func (this *SandboxFilter) ConfigStruct() interface{} {
	return new(SandboxFilterConfig)
}

func (this *SandboxFilter) Init(config interface{}) (err error) {
	if this.sb != nil {
		return nil // no-op already initialized
	}
	conf := config.(*SandboxFilterConfig)
	this.sbc = conf.Sbc

	switch this.sbc.ScriptType {
	case "lua":
		this.sb, err = lua.CreateLuaSandbox(&this.sbc)
		if err != nil {
			return err
		}
	default:
		return fmt.Errorf("unsupported script type: %s", this.sbc.ScriptType)
	}

	if this.sbc.PreserveData && fileExists(this.sbc.ScriptFilename+".data") {
		err = this.sb.Init(this.sbc.ScriptFilename + ".data")
	} else {
		err = this.sb.Init("")
	}

	this.sb.Output(func(s string) {
		log.Println(s)
		///@todo waiting on output refactor since we no longer have the output list
		// msg := MessageGenerator.Retrieve()
		// msg.Message.SetType("heka_filter")
		// msg.Message.SetLogger(this.sbc.ScriptFilename)
		// msg.Message.SetPayload(s)
		// for _, name := range todoOutputs {
		// 	MessageGenerator.Output(name, msg)
		// }
	})

	this.sb.InjectMessage(func(s string) {
		msg := MessageGenerator.Retrieve()
		msg.Message.SetType("heka.lua_sandbox")
		msg.Message.SetLogger(this.sbc.ScriptFilename)
		msg.Message.SetPayload(s)
		MessageGenerator.Inject(msg)
	})

	return err
}

func (this *SandboxFilter) Start(fr FilterRunner, h PluginHelper,
	wg *sync.WaitGroup) (err error) {

	inChan := fr.InChan()
	ticker := fr.Ticker()
	matchChan := fr.MatchChan()

	go func() {
		var (
			inOk, matchOk, terminated = true, true, false
			pack                      *PipelinePack
			plc                       *PipelineCapture
			captures                  map[string]string
			retval                    int
		)
		for (inOk || matchOk) && !terminated {
			pack = nil
			captures = nil
			select {
			case pack, inOk = <-inChan:
			case plc, matchOk = <-matchChan:
				if matchOk {
					pack = plc.Pack
					captures = plc.Captures
				}
			case t := <-ticker:
				if retval = this.sb.TimerEvent(t.UnixNano()); retval != 0 {
					fr.LogError(fmt.Errorf(
						"Sandbox TimerEvent error code: %d, error message: %s",
						retval, this.sb.LastError()))
					if this.sb.Status() == sandbox.STATUS_TERMINATED {
						terminated = true
					}
				}
			}
			if pack != nil {
				if retval = this.sb.ProcessMessage(pack.Message, captures); retval != 0 {
					fr.LogError(fmt.Errorf(
						"Sandbox ProcessMessage error code: %d, error message: %s",
						retval, this.sb.LastError()))
					if this.sb.Status() == sandbox.STATUS_TERMINATED {
						terminated = true
					}
				}
				pack.Recycle()
			}
		}
		if this.sbc.PreserveData {
			this.sb.Destroy(this.sbc.ScriptFilename + ".data")
		} else {
			this.sb.Destroy("")
		}
		this.sb = nil
		log.Printf("SandboxFilter '%s' stopped.", fr.Name())
		wg.Done()
	}()
	return
}
