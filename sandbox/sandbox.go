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

package sandbox

/*
#include "sandbox.h"
*/
import "C"

import "github.com/mozilla-services/heka/message"

const (
	STATUS_UNKNOWN    = C.STATUS_UNKNOWN
	STATUS_RUNNING    = C.STATUS_RUNNING
	STATUS_TERMINATED = C.STATUS_TERMINATED

	STAT_LIMIT   = C.USAGE_STAT_LIMIT
	STAT_CURRENT = C.USAGE_STAT_CURRENT
	STAT_MAXIMUM = C.USAGE_STAT_MAXIMUM

	TYPE_MEMORY       = C.USAGE_TYPE_MEMORY
	TYPE_INSTRUCTIONS = C.USAGE_TYPE_INSTRUCTION
	TYPE_OUTPUT       = C.USAGE_TYPE_OUTPUT
)

type Sandbox interface {
	// Sandbox control
	Init(dataFile string) error
	Destroy(dataFile string) error

	// Sandbox state
	Status() int
	LastError() string
	Usage(utype, ustat int) uint

	// Plugin functions
	ProcessMessage(msg *message.Message) int
	TimerEvent(ns int64) int

	// Go callback
	InjectMessage(f func(payload, payload_type, payload_name string) int)
}

type SandboxConfig struct {
	ScriptType       string `toml:"script_type"`
	ScriptFilename   string `toml:"filename"`
	PreserveData     bool   `toml:"preserve_data"`
	MemoryLimit      uint   `toml:"memory_limit"`
	InstructionLimit uint   `toml:"instruction_limit"`
	OutputLimit      uint   `toml:"output_limit"`
	Profile          bool
	Config           map[string]interface{}
}
