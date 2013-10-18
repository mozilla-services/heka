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

package logstreaminput

import (
	"os"
	"path/filepath"
	"regexp"
)

type SubexpName string
type SortValue int

type Logfile struct {
	FileName string
	// The matched portions of the filename and their translated integer value
	MatchParts map[SubexpName]SortValue
}

type Logfiles []*Logfile

// Implement two of the sort.Interface methods needed
func (l Logfiles) Len() int      { return len(l) }
func (l Logfiles) Swap(i, j int) { l[i], l[j] = l[j], l[i] }

// Locate the index of a given filename if its present in the logfiles for this stream
// Returns -1 if the filename is not present
func (l Logfiles) IndexOf(s string) int {
	for i := 0; i < len(l); i++ {
		if l[i].FileName == s {
			return i
		}
	}
	return -1
}

// ByPriority implements the final method of the sort.Interface so that the embedded
// LogfileMatches may be sorted by the priority of their matches parts
type ByPriority struct {
	Logfiles
	Priority []string
}

// Determine based on priority if which of the two is 'less' than the other
func (b ByPriority) Less(i, j int) bool {
	var convert func(bool) bool
	first := b.Logfiles[i]
	second := b.Logfiles[j]
	for _, part := range b.Priority {
		convert = func(a bool) bool { return a }
		if "^" == part[0] {
			part = part[1:]
			convert = func(a bool) bool { return !a }
		}
		if first.MatchParts[part] < second.MatchParts[part] {
			return convert(true)
		} else if first.MatchParts[part] > second.MatchParts[part] {
			return convert(false)
		}
	}
	// If we get here, it means all the parts are exactly equal, consider
	// the first not less than the second
	return false
}

func ScanDirectoryForLogfiles(directoryPath string, fileMatch *regexp.Regexp) Logfiles {
	files := make(Logfiles)
	filepath.Walk(directoryPath, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		if fileMatch.MatchString(path) {
			files = append(files, &Logfile{FileName: path})
		}
	})
	return files
}

type MultipleLogstreamFileList map[string]Logfiles

func FilterMultipleStreamFiles(files Logfiles, fileMatch string, diferentiator []string) MultipleLogstreamFileList {
}

type LogstreamLocation struct {
	Filename     string
	SeekPosition float64
	Start        float64
	Length       float64
	Hash         string
}

func NewLogstreamLocationFromJSONFile(path string) *LogstreamLocation {}

func (l *LogstreamLocation) SaveProgressToJSONFile(path string) error {}

func LocatePriorLocation(files LogstreamFileList, position LogstreamLocation) (*os.File, error) {}

// Match translation map for a matched section that maps the string value to the integer to
// sort on.
type MatchTranslationMap map[string]int

// SubmatchTranslationMap holds a map of MatchTranslationMap's for every submatch that
// should be translated to an int.
type SubmatchTranslationMap map[string]MatchTranslationMap

// Sort pattern is a construct used to return an ordered list of filenames based
// on the supplied sorting criteria
//
// Example:
//
//     Assume that all logfiles are named '2013/August/08/xyz-11.log'. First a
// FileMatch pattern is needed to recognize these parts, which would look like:
//
//     (?P<Year>\d{4})/(?P<MonthName>\w+)/(?P<Day>\d+)/\w+-(?P<Seq>\d+).log
//
//     The above pattern will match the logfiles and break them down to 4 key parts
// used to sort the list. The Year, MonthName (which will be converted to ints), Day,
// and Seq. No Translation mapping is needed in this case, as standard English month
// and day names are translated to integers. The priority for sorting should be done
// with Year first, then MonthName, Day, and finally Seq, so the Priority would be
//
//     ["Year", "MonthName", "Day", "Seq"]
type SortPattern struct {
	// Regular expression for the files to match and parts of the filename to use for
	// sorting. All parts to be sorted on must be captured and named. Special handling is
	// provided for parts with names: MonthName, DayName.
	// These names will be translated from short/long month/day names to the appropriate
	// integer value.
	FileMatch string
	// Translation is used for custom ordering lookups where a custom value needs to be
	// translated to a value for sorting. ie. a different tool using weekdays with values
	// causing the wrong day of the week to be parsed first
	Translation SubmatchTranslationMap
	// Priority list which should be provided to determine the most important parts of
	// the matches to sort on. Most important captured name should be first. These will
	// be sorted in ascending order representing *oldest first*. If this portions value
	// increasing means its *older*, then it should be sorted in descending order by
	// adding a ^ to the beginning.
	Priority []string
	// Differentiators are used on portions of the file match to indicate unique
	// non-changing portions that combined will yield an identifier for this 'logfile'
	// If the name is not a subregex name, its raw value will be used to identify
	// the log stream.
	Differentiator []string
}

// CollectFileNames will collect all matches using a glob if a '*' is in the
// dirname, otherwise it walks an entire dirname contents and subdirectories
// collecting all the filenames found.
func CollectFileNames(dirname string) []string {}

type LogfileLists map[string]LogfileMatches

// ParseCollectedFileNames parses a slice of strings representing all the
// filenames found against the SortPattern to return a map keyed by the
// combined differentiator with the value of LogfileMatches
func ParseCollectedFileNames(filenames []string, pattern *SortPattern) LogfileLists {}
