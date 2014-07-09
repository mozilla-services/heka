/***** BEGIN LICENSE BLOCK *****
# This Source Code Form is subject to the terms of the Mozilla Public
# License, v. 2.0. If a copy of the MPL was not distributed with this file,
# You can obtain one at http://mozilla.org/MPL/2.0/.
#
# The Initial Developer of the Original Code is the Mozilla Foundation.
# Portions created by the Initial Developer are Copyright (C) 2014
# the Initial Developer. All Rights Reserved.
#
# Contributor(s):
#   Ben Bangert (bbangert@mozilla.com)
#   Rob Miller (rmiller@mozilla.com)
#
# ***** END LICENSE BLOCK *****/

package logstreamer

import (
	gs "github.com/rafrombrc/gospec/src/gospec"
	"os"
	"path/filepath"
	"runtime"
	"sort"
)

func FilehandlingSpec(c gs.Context) {
	here, err := os.Getwd()
	c.Assume(err, gs.IsNil)
	dirPath := filepath.Join(here, "testdir", "filehandling")
	regex1, regex2 := `/subdir/.*\.log(\..*)?`, "/subdir/.*.logg(.*)?"
	if runtime.GOOS == "windows" {
		regex1, regex2 = `\\subdir\\.*\.log(\..*)?`, `\\subdir\\.*.logg(.*)?`
	}

	c.Specify("The directory scanner", func() {

		c.Specify("scans a directory properly", func() {
			results := ScanDirectoryForLogfiles(dirPath, fileMatchRegexp(dirPath, regex1))
			c.Expect(len(results), gs.Equals, 3)
		})

		c.Specify("scans a directory with a bad regexp", func() {
			results := ScanDirectoryForLogfiles(dirPath, fileMatchRegexp(dirPath, regex2))
			c.Expect(len(results), gs.Equals, 0)
		})
	})

	c.Specify("Populating logfile with match parts", func() {
		logfile := Logfile{}

		c.Specify("works without errors", func() {
			subexpNames := []string{"MonthName", "LogNumber"}
			matches := []string{"October", "24"}
			translation := make(SubmatchTranslationMap)
			logfile.PopulateMatchParts(subexpNames, matches, translation)
			c.Expect(logfile.MatchParts["MonthName"], gs.Equals, 10)
			c.Expect(logfile.MatchParts["LogNumber"], gs.Equals, 24)
		})

		c.Specify("works with bad month name", func() {
			subexpNames := []string{"MonthName", "LogNumber"}
			matches := []string{"Octoberrr", "24"}
			translation := make(SubmatchTranslationMap)
			err := logfile.PopulateMatchParts(subexpNames, matches, translation)
			c.Assume(err, gs.Not(gs.IsNil))
			c.Expect(err.Error(), gs.Equals, "Unable to locate month name: Octoberrr")
		})

		c.Specify("works with missing value in submatch translation map", func() {
			subexpNames := []string{"MonthName", "LogNumber"}
			matches := []string{"October", "24"}
			translation := make(SubmatchTranslationMap)
			translation["LogNumber"] = make(MatchTranslationMap)
			translation["LogNumber"]["23"] = 22
			translation["LogNumber"]["999"] = 999 // Non-"missing" submatches must be len > 1.
			err := logfile.PopulateMatchParts(subexpNames, matches, translation)
			c.Assume(err, gs.Not(gs.IsNil))
			c.Expect(err.Error(), gs.Equals, "Value '24' not found in translation map 'LogNumber'.")
		})

		c.Specify("works with custom value in submatch translation map", func() {
			subexpNames := []string{"MonthName", "LogNumber"}
			matches := []string{"October", "24"}
			translation := make(SubmatchTranslationMap)
			translation["LogNumber"] = make(MatchTranslationMap)
			translation["LogNumber"]["24"] = 2
			translation["LogNumber"]["999"] = 999 // Non-"missing" submatches must be len > 1.
			logfile.PopulateMatchParts(subexpNames, matches, translation)
			c.Expect(logfile.MatchParts["MonthName"], gs.Equals, 10)
			c.Expect(logfile.MatchParts["LogNumber"], gs.Equals, 2)
		})
	})

	c.Specify("Populating logfiles with match parts", func() {
		translation := make(SubmatchTranslationMap)
		regex := `/subdir/.*\.log\.?(?P<FileNumber>.*)?`
		if runtime.GOOS == "windows" {
			regex = `\\subdir\\.*\.log\.?(?P<FileNumber>.*)?`
		}
		matchRegex := fileMatchRegexp(dirPath, regex)
		logfiles := ScanDirectoryForLogfiles(dirPath, matchRegex)

		c.Specify("is populated", func() {
			logfiles.PopulateMatchParts(matchRegex, translation)
			c.Expect(len(logfiles), gs.Equals, 3)
			c.Expect(logfiles[0].MatchParts["FileNumber"], gs.Equals, -1)
			c.Expect(logfiles[1].MatchParts["FileNumber"], gs.Equals, 1)
			c.Expect(logfiles[1].StringMatchParts["FileNumber"], gs.Equals, "1")
		})

		c.Specify("returns errors", func() {
			translation["FileNumber"] = make(MatchTranslationMap)
			translation["FileNumber"]["23"] = 22
			translation["FileNumber"]["999"] = 999 // Non-"missing" submatches must be len > 1.
			err := logfiles.PopulateMatchParts(matchRegex, translation)
			c.Assume(err, gs.Not(gs.IsNil))
			c.Expect(len(logfiles), gs.Equals, 3)
		})
	})

	c.Specify("Sorting logfiles", func() {
		translation := make(SubmatchTranslationMap)
		regex := `/subdir/.*\.log\.?(?P<FileNumber>.*)?`
		if runtime.GOOS == "windows" {
			regex = `\\subdir\\.*\.log\.?(?P<FileNumber>.*)?`
		}
		matchRegex := fileMatchRegexp(dirPath, regex)
		logfiles := ScanDirectoryForLogfiles(dirPath, matchRegex)

		c.Specify("with no 'missing' translation value", func() {
			err := logfiles.PopulateMatchParts(matchRegex, translation)
			c.Assume(err, gs.IsNil)
			c.Expect(len(logfiles), gs.Equals, 3)

			c.Specify("can be sorted newest to oldest", func() {
				byp := ByPriority{Logfiles: logfiles, Priority: []string{"FileNumber"}}
				sort.Sort(byp)
				c.Expect(logfiles[0].MatchParts["FileNumber"], gs.Equals, -1)
				c.Expect(logfiles[1].MatchParts["FileNumber"], gs.Equals, 1)
			})

			c.Specify("can be sorted oldest to newest", func() {
				byp := ByPriority{Logfiles: logfiles, Priority: []string{"^FileNumber"}}
				sort.Sort(byp)
				c.Expect(logfiles[0].MatchParts["FileNumber"], gs.Equals, 2)
				c.Expect(logfiles[1].MatchParts["FileNumber"], gs.Equals, 1)
			})
		})

		c.Specify("with 'missing' translation value", func() {
			translation["FileNumber"] = make(MatchTranslationMap)
			translation["FileNumber"]["missing"] = 5
			err := logfiles.PopulateMatchParts(matchRegex, translation)
			c.Assume(err, gs.IsNil)
			c.Expect(len(logfiles), gs.Equals, 3)

			c.Specify("honors 'missing' translation value", func() {
				byp := ByPriority{Logfiles: logfiles, Priority: []string{"FileNumber"}}
				sort.Sort(byp)
				c.Expect(logfiles[0].MatchParts["FileNumber"], gs.Equals, 1)
				c.Expect(logfiles[1].MatchParts["FileNumber"], gs.Equals, 2)
				c.Expect(logfiles[2].MatchParts["FileNumber"], gs.Equals, 5)
			})
		})
	})

	c.Specify("Sorting out a directory of access/error logs", func() {
		translation := make(SubmatchTranslationMap)
		regex := `/(?P<Year>\d+)/(?P<Month>\d+)/(?P<Type>\w+)\.log(\.(?P<Seq>\d+))?`
		if runtime.GOOS == "windows" {
			regex = `\\(?P<Year>\d+)\\(?P<Month>\d+)\\(?P<Type>\w+)\.log(\.(?P<Seq>\d+))?`
		}
		matchRegex := fileMatchRegexp(dirPath, regex)
		logfiles := ScanDirectoryForLogfiles(dirPath, matchRegex)
		err := logfiles.PopulateMatchParts(matchRegex, translation)
		c.Assume(err, gs.IsNil)
		c.Expect(len(logfiles), gs.Equals, 52)

		c.Specify("can result in multiple logfile streams", func() {
			mfs := FilterMultipleStreamFiles(logfiles, []string{"Type"})
			c.Expect(len(mfs), gs.Equals, 2)
			access, ok := mfs["access"]
			c.Assume(ok, gs.IsTrue)
			c.Expect(len(access), gs.Equals, 26)
			error, ok := mfs["error"]
			c.Assume(ok, gs.IsTrue)
			c.Expect(len(error), gs.Equals, 26)

			c.Specify("can be individually sorted properly by access", func() {
				byp := ByPriority{Logfiles: mfs["access"], Priority: []string{"Year", "Month", "^Seq"}}
				sort.Sort(byp)
				lf := mfs["access"]
				c.Expect(lf[0].FileName, gs.Equals, dirPath+filepath.FromSlash("/2010/05/access.log.3"))
				c.Expect(lf[len(lf)-1].FileName, gs.Equals, dirPath+filepath.FromSlash("/2013/08/access.log"))
			})

			c.Specify("can be individually sorted properly by error", func() {
				byp := ByPriority{Logfiles: mfs["error"], Priority: []string{"Year", "Month", "^Seq"}}
				sort.Sort(byp)
				lf := mfs["error"]
				c.Expect(lf[0].FileName, gs.Equals, dirPath+filepath.FromSlash("/2010/07/error.log.2"))
				c.Expect(lf[len(lf)-1].FileName, gs.Equals, dirPath+filepath.FromSlash("/2013/08/error.log"))
			})
		})

		c.Specify("Can result in multiple logfile streams with a prefix", func() {
			mfs := FilterMultipleStreamFiles(logfiles, []string{"website-", "Type"})
			c.Expect(len(mfs), gs.Equals, 2)
			access, ok := mfs["website-access"]
			c.Assume(ok, gs.IsTrue)
			c.Expect(len(access), gs.Equals, 26)
			error, ok := mfs["website-error"]
			c.Assume(ok, gs.IsTrue)
			c.Expect(len(error), gs.Equals, 26)
		})
	})
}
