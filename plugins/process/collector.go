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
#   Victor Ng (vng@mozilla.com)
#
#***** END LICENSE BLOCK *****/

/*
The ProcessManager input plugin accepts a BasePath directory path to
find a cron-like directory structure.

Example layout:

collector/                       -- The BasePath
collector/0/                     -- This is used for long running processes
collector/<delay_in_seconds>/    -- Other scripts can be run intermittently

The ProcessManager will execute scripts by using the ProcessInput plugins

Configurable parameters for each script:

You must specify a default decoder to be used for any active
processes.

You must specify a default timeout in seconds for any misbehaved
scripts.

You must specify a default type to be used for each message.

Each process which is found will be registered with a name of
/delay/script_name.ext.

You may optionally specify the name of the decoder instance to be used
with each registered script name.

You may optionally specify the timeout in seconds to be used
with each registered script name.

You may optionally specify the message type to be used
with each registered script name.
*/

package process

import (
	"fmt"
)

type scriptconfig struct {
	Bin string

	// Command arguments
	Args []string

	// Enviroment variables
	Env []string

	// Dir specifies the working directory of Command.  Defaults to
	// the directory where the program resides.
	Directory string
}

type ProcessManagerConfig struct {
	// The root path to find the tcollector-like directory structure
	BasePath string

	// Name of configured decoder instance.
	Decoder string

	TimeoutSeconds uint `toml:"timeout"`

	MsgType string

	// Extra configuration per active script
	// Any names that aren't matched to an active script are simply
	// ignored, but the configuration error is logged to stderr.
	Process map[string]scriptconfig
}

// Heka Input plugin that runs external programs and processes their
// output as a stream into Message objects to be passed into
// the Router for delivery to matching Filter or Output plugins.
type ProcessManager struct {
	BasePath       string
	Decoder        string
	TimeoutSeconds uint
	MsgType        string

	ProcessMap map[string]interface{}
}

func (p *ProcessManager) ConfigStruct() interface{} {
	return &ProcessManagerConfig{
		BasePath:       "process_manager",
		TimeoutSeconds: 30,
		MsgType:        "ProcessManager",
	}
}

func (p *ProcessManager) Init(config interface{}) (err error) {
	var script_path string
	var script_config scriptconfig

	conf := config.(*ProcessManagerConfig)

	p.BasePath = conf.BasePath
	p.Decoder = conf.Decoder
	p.TimeoutSeconds = conf.TimeoutSeconds
	p.MsgType = conf.MsgType
	p.ProcessMap = make(map[string]interface{})

	for script_path, script_config = range conf.Process {
		pInput := ProcessInput{}
		pInputConfig := pInput.ConfigStruct().(*ProcessInputConfig)
		pInputConfig.Command = make(map[string]cmd_config)

		pInputConfig.Command["0"] = cmd_config{Bin: script_config.Bin, Args: []string{}}
		err := pInput.Init(config)
		p.ProcessMap[script_path] = pInput

		// TODO: handle the timeouts, set the msgtype, set the decoder
		fmt.Printf("Created plugin: %s  Error: %s\n", pInput, err)
	}
	return nil
}
