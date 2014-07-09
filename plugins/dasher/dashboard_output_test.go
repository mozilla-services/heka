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

package dasher

import (
	"code.google.com/p/gomock/gomock"
	. "github.com/mozilla-services/heka/pipeline"
	pipeline_ts "github.com/mozilla-services/heka/pipeline/testsupport"
	gs "github.com/rafrombrc/gospec/src/gospec"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestAllSpecs(t *testing.T) {
	r := gs.NewRunner()
	r.Parallel = false

	r.AddSpec(DashboardOutputSpec)

	gs.MainGoTest(r, t)
}

func DashboardOutputSpec(c gs.Context) {
	t := new(pipeline_ts.SimpleT)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	NewPipelineConfig(nil) // Needed for side effect of setting up Globals :P

	if runtime.GOOS != "windows" {
		c.Specify("A DashboardOutput", func() {
			dashboardOutput := new(DashboardOutput)

			config := dashboardOutput.ConfigStruct().(*DashboardOutputConfig)
			c.Specify("Init halts if basedirectory is not writable", func() {
				tmpdir := filepath.Join(os.TempDir(), "tmpdir")
				err := os.MkdirAll(tmpdir, 0400)
				c.Assume(err, gs.IsNil)
				config.WorkingDirectory = tmpdir
				err = dashboardOutput.Init(config)
				c.Assume(err, gs.Not(gs.IsNil))
			})
		})
	}
}
