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

package pipeline

import (
	"code.google.com/p/gomock/gomock"
	ts "github.com/mozilla-services/heka/testsupport"
	gs "github.com/rafrombrc/gospec/src/gospec"
	"os"
	"path"
)

func DashboardOutputSpec(c gs.Context) {
	t := new(ts.SimpleT)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	c.Specify("A FileOutput", func() {
		dashboardOutput := new(DashboardOutput)

		config := dashboardOutput.ConfigStruct().(*DashboardOutputConfig)
		c.Specify("Init halts if basedirectory is not writable", func() {
			tmpdir := path.Join(os.TempDir(), "tmpdir")
			err := os.MkdirAll(tmpdir, 0400)
			c.Assume(err, gs.IsNil)
			config.WorkingDirectory = tmpdir
			err = dashboardOutput.Init(config)
			c.Assume(err, gs.Not(gs.IsNil))
		})
	})
}
