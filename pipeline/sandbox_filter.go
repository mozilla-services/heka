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
	"os"
	"path"
)

func fileExists(path string) bool {
	_, err := os.Stat(path)
	if err == nil {
		return true
	}
	return false
}

type SandboxFilterConfig struct {
	Sbc sandbox.SandboxConfig `toml:"settings"`
}

type SandboxFilter struct {
	sb               sandbox.Sandbox
	sbc              sandbox.SandboxConfig
	preservationFile string
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

	this.preservationFile = this.sbc.ScriptFilename + ".data"
	if this.sbc.PreserveData && fileExists(this.preservationFile) {
		err = this.sb.Init(this.preservationFile)
	} else {
		err = this.sb.Init("")
	}

	this.sb.InjectMessage(func(s string) {
		msg := MessageGenerator.Retrieve()
		msg.Message.SetType("heka.sandbox")
		msg.Message.SetLogger(path.Base(this.sbc.ScriptFilename))
		msg.Message.SetPayload(s)
		MessageGenerator.Inject(msg)
	})

	return err
}

func (this *SandboxFilter) Run(fr FilterRunner, h PluginHelper) (err error) {
	inChan := fr.InChan()
	ticker := fr.Ticker()

	var (
		ok, terminated = true, false
		plc            *PipelineCapture
		retval         int
	)
	for ok && !terminated {
		select {
		case plc, ok = <-inChan:
			if !ok {
				break
			}
			retval = this.sb.ProcessMessage(plc.Pack.Message, plc.Captures)
			if retval != 0 {
				fr.LogError(fmt.Errorf(
					"Sandbox ProcessMessage error code: %d, error message: %s",
					retval, this.sb.LastError()))
				if this.sb.Status() == sandbox.STATUS_TERMINATED {
					terminated = true
				}
			}
			plc.Pack.Recycle()
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
	}
	if this.sbc.PreserveData {
		this.sb.Destroy(this.preservationFile)
	} else {
		this.sb.Destroy("")
	}
	this.sb = nil
	return
}
