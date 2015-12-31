/***** BEGIN LICENSE BLOCK *****
# This Source Code Form is subject to the terms of the Mozilla Public
# License, v. 2.0. If a copy of the MPL was not distributed with this file,
# You can obtain one at http://mozilla.org/MPL/2.0/.
#
# The Initial Developer of the Original Code is the Mozilla Foundation.
# Portions created by the Initial Developer are Copyright (C) 2012-2014
# the Initial Developer. All Rights Reserved.
#
# Contributor(s):
#   Mike Trinkala (trink@mozilla.com)
#
# ***** END LICENSE BLOCK *****/

package sandbox

import "github.com/mozilla-services/heka/pipeline"

const (
	STATUS_UNKNOWN    = 0
	STATUS_RUNNING    = 1
	STATUS_TERMINATED = 2

	STAT_LIMIT   = 0
	STAT_CURRENT = 1
	STAT_MAXIMUM = 2

	TYPE_MEMORY       = 0
	TYPE_INSTRUCTIONS = 1
	TYPE_OUTPUT       = 2

	DATA_DIR = "sandbox_preservation"
	DATA_EXT = ".data"
)

type Sandbox interface {
	// Sandbox control
	Init(dataFile string) error
	Stop()
	Destroy(dataFile string) error

	// Sandbox state
	Status() int
	LastError() string
	Usage(utype, ustat int) uint

	// Plugin functions
	ProcessMessage(pack *pipeline.PipelinePack) int
	TimerEvent(ns int64) int

	// Go callback
	InjectMessage(f func(payload, payload_type, payload_name string) int)
}

type SandboxConfig struct {
	ScriptType           string `toml:"script_type"`
	ScriptFilename       string `toml:"filename"`
	ModuleDirectory      string `toml:"module_directory"`
	PreserveData         bool   `toml:"preserve_data"`
	MemoryLimit          uint   `toml:"memory_limit"`
	InstructionLimit     uint   `toml:"instruction_limit"`
	OutputLimit          uint   `toml:"output_limit"`
	CanExit              bool   `toml:"can_exit"`
	TimerEventOnShutdown bool   `toml:"timer_event_on_shutdown"`
	Profile              bool
	Config               map[string]interface{}
	Globals              *pipeline.GlobalConfigStruct
	PluginType           string
}

func NewSandboxConfig(globals *pipeline.GlobalConfigStruct) interface{} {
	return &SandboxConfig{
		ModuleDirectory:  globals.PrependShareDir("lua_modules"),
		MemoryLimit:      8 * 1024 * 1024,
		InstructionLimit: 1e6,
		OutputLimit:      63 * 1024,
		ScriptType:       "lua",
		Globals:          globals,
		CanExit:          true,
	}
}
