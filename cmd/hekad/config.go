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
#   Rob Miller (rmiller@mozilla.com)
#
# ***** END LICENSE BLOCK *****/

// Hekad configuration.

package main

import (
	"fmt"
	"github.com/bbangert/toml"
	"io/ioutil"
	"os"
	"path/filepath"
)

type HekadConfig struct {
	Maxprocs            int    `toml:"maxprocs"`
	PoolSize            int    `toml:"poolsize"`
	DecoderPoolSize     int    `toml:"decoder_poolsize"`
	ChanSize            int    `toml:"plugin_chansize"`
	CpuProfName         string `toml:"cpuprof"`
	MemProfName         string `toml:"memprof"`
	MaxMsgLoops         uint   `toml:"max_message_loops"`
	MaxMsgProcessInject uint   `toml:"max_process_inject"`
	MaxMsgTimerInject   uint   `toml:"max_timer_inject"`
	BaseDir             string `toml:"base_dir"`
}

func LoadHekadConfig(configPath string) (config *HekadConfig, err error) {
	config = &HekadConfig{Maxprocs: 1,
		PoolSize:            100,
		DecoderPoolSize:     4,
		ChanSize:            50,
		CpuProfName:         "",
		MemProfName:         "",
		MaxMsgLoops:         4,
		MaxMsgProcessInject: 1,
		MaxMsgTimerInject:   10,
		BaseDir:             filepath.FromSlash("/var/cache/hekad"),
	}

	var configFile map[string]toml.Primitive
	var filename string
	p, err := os.Open(configPath)
	fi, err := p.Stat()

	if fi.IsDir() {
		files, _ := ioutil.ReadDir(configPath)
		for _, f := range files {
			filename = filepath.Join(configPath, f.Name())
			if _, err = toml.DecodeFile(filename, &configFile); err != nil {
				return nil, fmt.Errorf("Error decoding config file: %s", err)
			}
		}
	} else {
		if _, err = toml.DecodeFile(configPath, &configFile); err != nil {
			return nil, fmt.Errorf("Error decoding config file: %s", err)
		}
	}

	empty_ignore := map[string]interface{}{}
	parsed_config, ok := configFile["hekad"]
	if ok {
		if err = toml.PrimitiveDecodeStrict(parsed_config, &config, empty_ignore); err != nil {
			err = fmt.Errorf("Can't unmarshal config: %s", err)
		}
	}

	return
}
