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
	"regexp"
	"sort"
)

func ReaderSpec(c gs.Context) {
	here, err := os.Getwd()
	c.Assume(err, gs.IsNil)
	dirPath := filepath.Join(here, "testdir", "reader")
	testDirPath := filepath.Join(here, "testdir", "filehandling")

	c.Specify("A journal file can be read", func() {
		l, err := LogstreamLocationFromFile(dirPath + "/location.json")
		fileName := "testdir/reader/2010/07/error.log.2"
		c.Expect(l.Filename, gs.Equals, fileName)
		c.Expect(err, gs.IsNil)

		l.Filename = filepath.Join(testDirPath, "/2010/07/error.log.2")

		translation := make(SubmatchTranslationMap)
		matchRegex := regexp.MustCompile(testDirPath + `/(?P<Year>\d+)/(?P<Month>\d+)/error.log(\.(?P<Seq>\d+))?`)
		logfiles := ScanDirectoryForLogfiles(testDirPath, matchRegex)
		err = logfiles.PopulateMatchParts(matchRegex, translation)
		c.Assume(err, gs.IsNil)
		c.Expect(len(logfiles), gs.Equals, 26)

		byp := ByPriority{Logfiles: logfiles, Priority: []string{"Year", "Month", "^Seq"}}
		sort.Sort(byp)

		stream := NewLogstream(logfiles, l)
		b := make([]byte, 500)
		n, err := stream.Read(b)
		c.Expect(err, gs.IsNil)
		c.Expect(n, gs.Equals, 500)
		l.Filename = "testdir/reader/2010/07/error.log.2"
	})

}
