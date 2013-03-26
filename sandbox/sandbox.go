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

	STAT_LIMIT   = C.US_LIM
	STAT_CURRENT = C.US_CUR
	STAT_MAXIMUM = C.US_MAX

	TYPE_MEMORY       = C.UT_MEM
	TYPE_INSTRUCTIONS = C.UT_INS
	TYPE_OUTPUT       = C.UT_OUT
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
	ProcessMessage(msg *message.Message, captures map[string]string) int
	TimerEvent(ns int64) int

	// Go callback
	InjectMessage(f func(s string))
}

type SandboxConfig struct {
	ScriptType       string `json:"type"`
	ScriptFilename   string `json:"filename"`
	PreserveData     bool   `json:"preserve_data"`
	MemoryLimit      uint   `json:"memory_limit"`
	InstructionLimit uint   `json:"instruction_limit"`
	OutputLimit      uint   `json:"output_limit"`
}
