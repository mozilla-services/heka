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

package logstreamer

import (
	gs "github.com/rafrombrc/gospec/src/gospec"
	"os"
	"path/filepath"
)

func ReaderSpec(c gs.Context) {
	here, err := os.Getwd()
	c.Assume(err, gs.IsNil)
	dirPath := filepath.Join(here, "testdir", "reader")

	c.Specify("A journal file can be read and saved", func() {
		l, err := LogstreamLocationFromFile(dirPath + "/location.json")
		l.Filename = dirPath + "/2010/07/error.log.2"
		err = l.Save()
		c.Expect(err, gs.IsNil)
	})
}
