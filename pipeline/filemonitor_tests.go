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
#   Rob Miller (rmiller@mozilla.com)
#
# ***** END LICENSE BLOCK *****/

package pipeline

import (
	"bufio"
	"encoding/json"
	gs "github.com/rafrombrc/gospec/src/gospec"
	"os"
)

func FileMonitorSpec(c gs.Context) {

	c.Specify("A FileMonitor", func() {
		c.Specify("serializes to JSON", func() {
			fm := new(FileMonitor)
			fm.Init([]string{"/tmp/foo.txt", "/tmp/bar.txt"}, 10, 10, "")
			fm.seek["/tmp/foo.txt"] = 200
			fm.seek["/tmp/bar.txt"] = 300

			c.Expect(fm, gs.Not(gs.Equals), nil)

			// Serialize to JSON
			fbytes, _ := json.Marshal(fm)

			newFM := new(FileMonitor)
			newFM.InitStructs()
			json.Unmarshal(fbytes, &newFM)
			c.Expect(len(newFM.seek), gs.Equals, len(fm.seek))
			c.Expect(newFM.seek["/tmp/foo.txt"], gs.Equals, fm.seek["/tmp/foo.txt"])
			c.Expect(newFM.seek["/tmp/bar.txt"], gs.Equals, fm.seek["/tmp/bar.txt"])
		})
	})

	c.Specify("os.stat", func() {
		c.Specify("is serializable", func() {
			// Setup a goroutine to accept messages to write and flush
			// to disk
			fo, _ := os.Create("/tmp/filemonitor_test.txt")
			w := bufio.NewWriter(fo)
			w.WriteString("blah")
			w.Flush()

		})
	})
}
