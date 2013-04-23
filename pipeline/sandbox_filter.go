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
	"os"
)

func fileExists(path string) bool {
	_, err := os.Stat(path)
	if err == nil {
		return true
	}
	return false
}

type SandboxFilter struct {
	sb               sandbox.Sandbox
	sbc              *sandbox.SandboxConfig
	preservationFile string
}

func (this *SandboxFilter) ConfigStruct() interface{} {
	return new(sandbox.SandboxConfig)
}

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

func (this *SandboxFilter) ReportMsg(msg *message.Message) error {
	newIntField(msg, "Memory", int(this.sb.Usage(sandbox.TYPE_MEMORY,
		sandbox.STAT_CURRENT)))
	newIntField(msg, "MaxMemory", int(this.sb.Usage(sandbox.TYPE_MEMORY,
		sandbox.STAT_MAXIMUM)))
	newIntField(msg, "MaxInstructions", int(this.sb.Usage(
		sandbox.TYPE_INSTRUCTIONS, sandbox.STAT_MAXIMUM)))
	newIntField(msg, "MaxOutput", int(this.sb.Usage(sandbox.TYPE_OUTPUT,
		sandbox.STAT_MAXIMUM)))
	return nil
}

func (this *SandboxFilter) Run(fr FilterRunner, h PluginHelper) (err error) {
	inChan := fr.InChan()
	ticker := fr.Ticker()

	var (
		ok, terminated = true, false
		plc            *PipelineCapture
		retval         int
		msgLoopCount   uint
	)

	this.sb.InjectMessage(func(payload, payload_type, payload_name string) int {
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
		return 0
	})

	for ok {
		select {
		case plc, ok = <-inChan:
			if !ok {
				break
			}
			msgLoopCount = plc.Pack.MsgLoopCount
			retval = this.sb.ProcessMessage(plc.Pack.Message, plc.Captures)
			if retval != 0 {
				terminated = true
			}
			plc.Pack.Recycle()
		case t := <-ticker:
			if retval = this.sb.TimerEvent(t.UnixNano()); retval != 0 {
				terminated = true
			}
		}

		if terminated {
			pack := h.PipelinePack(0)
			pack.Message.SetType("heka.sandbox-terminated")
			pack.Message.SetLogger(fr.Name())
			pack.Message.SetPayload(this.sb.LastError())
			fr.Inject(pack)
			break
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
