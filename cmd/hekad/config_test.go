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
# ***** END LICENSE BLOCK *****/

package main

import (
	"github.com/mozilla-services/heka/pipeline"
	gs "github.com/rafrombrc/gospec/src/gospec"
	"os"
	"path/filepath"
)

func ConfigSpec(c gs.Context) {
	c.Specify("Loading a TOML configuration with scheduled jobs", func() {
		clobber_base_dir, err := os.Getwd()
		c.Expect(err, gs.IsNil)

		configPath := "./testsupport/example.toml"
		config, err := LoadHekadConfig(configPath)
		c.Expect(err, gs.IsNil)

		// Clobber the base directory so that we scope tcollector into
		// the testsupport directory located under
		// src/heka/cmd/hekad
		config.BaseDir = clobber_base_dir
		c.Expect(config.ScheduledJobDir, gs.Equals, "testsupport/tcollector")

		globals, _, _ := setGlobalConfigs(config)
		pipeconf := pipeline.NewPipelineConfig(globals)

		err = pipeconf.LoadFromConfigFile(configPath)
		c.Expect(err, gs.IsNil)
		mysql1 := filepath.Join(clobber_base_dir, "testsupport/tcollector/60/mysql1.py")
		mysql2 := filepath.Join(clobber_base_dir, "testsupport/tcollector/10/mysql2.py")

		c.Expect(pipeconf.ScheduledJobs[mysql1], gs.Equals, uint(60))
		c.Expect(pipeconf.ScheduledJobs[mysql2], gs.Equals, uint(10))

	})
}
