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
#
# ***** END LICENSE BLOCK *****/

package pipeline

import (
	"fmt"
	"github.com/mozilla-services/heka/message"
	"github.com/mozilla-services/heka/sandbox"
	"github.com/mozilla-services/heka/sandbox/lua"
	"log"
)

func defaultOutput(s string) {
	log.Println(s)
}

func defaultInjectMessage(s string) {
	log.Println(s)
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
	err = this.sb.Init()
	this.SetOutput(defaultOutput)
	this.SetInjectMessage(defaultInjectMessage)
	return err
}

func (this *SandboxFilter) Destroy() {
	this.sb.Destroy()
	this.sb = nil
}

func (this *SandboxFilter) ProcessMessage(msg *message.Message) int {
	return this.sb.ProcessMessage(msg)
}

func (this *SandboxFilter) TimerEvent() int {
	return this.sb.TimerEvent()
}

func (this *SandboxFilter) SetOutput(f func(s string)) {
	this.sb.Output(f)
}

func (this *SandboxFilter) SetInjectMessage(f func(s string)) {
	this.sb.InjectMessage(f)
}
