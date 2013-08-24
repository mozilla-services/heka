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
#   Ben Bangert (bbangert@mozilla.com)
#
# ***** END LICENSE BLOCK *****/

package pipeline

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
)

type DasherOutputConfig struct {
	// IP Addres for the dashboard to run on
	Address string
	// Working directory, the dashboard will write data here for between
	// restarts and for data catchup
	WorkingDirectory string `toml:"working_directory"`
	// Static directory for dashboard static resources such as the client
	// CSS/HTML/JS. Defaults to `dasher_static/` under the heka base
	// directory
	StaticDirectory string `toml:"static_directory"`
	// Default interval at which dashboard will update is 5 seconds.
	TickerInterval uint `toml:"ticker_interval"`
	// Default message matcher
	MessageMatcher string
}

func (d *DasherOutput) ConfigStruct() interface{} {
	return &DasherOutputConfig{
		Address:          ":4352",
		WorkingDirectory: "dasher_workdir",
		StaticDirectory:  "dasher_static",
		TickerInterval:   uint(5),
		MessageMatcher:   "Type == 'heka.all-report' || Type == 'heka.sandbox-terminated' || Type == 'heka.sandbox-output'",
	}
}

type DasherOutput struct {
	workingDirectory string
	staticDirectory  string
	server           *http.Server
}

func (d *DasherOutput) Init(config interface{}) (err error) {
	conf := config.(*DasherOutputConfig)

	d.workingDirectory = GetHekaConfigDir(conf.WorkingDirectory)
	if err = os.MkdirAll(d.workingDirectory, 0700); err != nil {
		return fmt.Errorf("Can't create the working directory for the dashboard output: %s",
			err.Error())
	}

	// delete all previous output
	if matches, err := filepath.Glob(filepath.Join(d.workingDirectory, "*.*")); err == nil {
		for _, fn := range matches {
			os.Remove(fn)
		}
	}

	d.staticDirectory = conf.StaticDirectory

	return
}
