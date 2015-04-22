/***** BEGIN LICENSE BLOCK *****
# This Source Code Form is subject to the terms of the Mozilla Public
# License, v. 2.0. If a copy of the MPL was not distributed with this file,
# You can obtain one at http://mozilla.org/MPL/2.0/.
#
# The Initial Developer of the Original Code is the Mozilla Foundation.
# Portions created by the Initial Developer are Copyright (C) 2014-2015
# the Initial Developer. All Rights Reserved.
#
# Contributor(s):
#   Ben Bangert (bbangert@mozilla.com)
#   Rob Miller (rmiller@mozilla.com)
#
# ***** END LICENSE BLOCK *****/

package logstreamer

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

var MonthLookup = map[string]int{
	"january":   1,
	"jan":       1,
	"february":  2,
	"feb":       2,
	"march":     3,
	"mar":       3,
	"april":     4,
	"apr":       4,
	"may":       5,
	"june":      6,
	"jun":       6,
	"july":      7,
	"jul":       7,
	"august":    8,
	"aug":       8,
	"september": 9,
	"sep":       9,
	"october":   10,
	"oct":       10,
	"november":  11,
	"nov":       11,
	"december":  12,
	"dec":       12,
}

var DayLookup = map[string]int{
	"monday":    0,
	"mon":       0,
	"tuesday":   1,
	"tue":       1,
	"wednesday": 2,
	"wed":       2,
	"thursday":  3,
	"thu":       3,
	"friday":    4,
	"fri":       4,
	"saturday":  5,
	"sat":       5,
	"sunday":    6,
	"sun":       6,
}

var digitRegex = regexp.MustCompile(`^\d+$`)

// Custom multiple error type that satisfies Go error interface but has
// alternate printing options
// TODO:: Refactor into a utility package so that other plugins can use
// this instead of everyone inventing their own version
type MultipleError []string

func NewMultipleError() *MultipleError {
	me := make(MultipleError, 0)
	return &me
}

func (m MultipleError) Error() string {
	return strings.Join(m, " :: ")
}

func (m *MultipleError) AddMessage(s string) {
	*m = append(*m, s)
}

func (m MultipleError) IsError() bool {
	return len(m) > 0
}

// Represents an individual Logfile which is part of a Logstream
type Logfile struct {
	FileName string
	// The raw string matches from the filename, keys being the strings, values
	// are the portion that was matched
	StringMatchParts map[string]string
	// The matched portions of the filename and their translated integer value
	// MatchParts maps to integers used for sorting the Logfile within the
	// Logstream
	MatchParts map[string]int
}

// Populate the MatchParts of a Logfile supplied with the sub-expression names,
// and the match parts out of the regex along with a translation map to derive
// sorting ints from
func (l *Logfile) PopulateMatchParts(subexpNames, matches []string,
	translation SubmatchTranslationMap) error {

	var (
		score  int
		ok     bool
		submap MatchTranslationMap
	)
	if l.MatchParts == nil {
		l.MatchParts = make(map[string]int)
	}
	if l.StringMatchParts == nil {
		l.StringMatchParts = make(map[string]string)
	}
	for i, name := range subexpNames {
		matchValue := matches[i]
		// Store the raw string
		l.StringMatchParts[name] = matchValue

		if matchValue == "" {
			matchValue = "missing"
		}
		lowerValue := strings.ToLower(matchValue)
		score = -1
		if name == "" {
			continue
		}

		// Warning: nested ifs ahead. A MatchTranslationMap has to cover every
		// possible value *or* contain only the "missing" value or else it
		// will raise an error.
		if name == "MonthName" {
			if score, ok = MonthLookup[lowerValue]; !ok {
				return fmt.Errorf("Unable to locate month name: %s", matchValue)
			}
		} else if name == "DayName" {
			if score, ok = DayLookup[lowerValue]; !ok {
				return fmt.Errorf("Unable to locate day name: %s", matchValue)
			}
		} else if submap, ok = translation[name]; ok && len(submap) > 1 {
			if score, ok = submap[lowerValue]; !ok {
				return fmt.Errorf("Value '%s' not found in translation map '%s'.",
					matchValue, name)
			}
		} else if ok {
			// 'ok' value falls through from above, i.e. submap exists and has
			// 'len > 1.
			if lowerValue == "missing" {
				if score, ok = submap["missing"]; !ok {
					// This shouldn't happen, config validation should prevent
					// a translation map of len 1 that contains anything other
					// than "missing".
					score = -1
					ok = true
				}
			} else {
				// We're not the "missing" value, '!ok' signals that we should
				// convert from integer if possible.
				ok = false
			}
		}
		if !ok {
			// We didn't match, try to convert to an int.
			if digitRegex.MatchString(matchValue) {
				score, _ = strconv.Atoi(matchValue)
			}
		}
		l.MatchParts[name] = score
	}
	return nil
}

// Alias for a slice of logfiles, used mainly for sorting algorithms
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

// Provided the fileMatch regexp and translation map, populate all the Logfile
// matchparts for use in sorting.
func (l Logfiles) PopulateMatchParts(fileMatch *regexp.Regexp,
	translation SubmatchTranslationMap) error {

	errorlist := NewMultipleError()
	subexpNames := fileMatch.SubexpNames()
	for _, logfile := range l {
		matches := fileMatch.FindStringSubmatch(logfile.FileName)
		if err := logfile.PopulateMatchParts(subexpNames, matches, translation); err != nil {
			errorlist.AddMessage(err.Error())
		}
	}
	if errorlist.IsError() {
		return errorlist
	}
	return nil
}

// Returns a list of all the filenames in their current order
func (l Logfiles) FileNames() []string {
	s := make([]string, len(l))
	for _, logfile := range l {
		s = append(s, logfile.FileName)
	}
	return s
}

// Returns a Logfiles only containing logfiles newer than oldTime
// based on the files last modified file attribute
func (l Logfiles) FilterOld(oldTime time.Time) Logfiles {
	f := make(Logfiles, 0)
	for _, logfile := range l {
		finfo, err := os.Stat(logfile.FileName)
		if err != nil {
			continue
		}
		if finfo.ModTime().After(oldTime) {
			f = append(f, logfile)
		}
	}
	return f
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
		if "^" == part[:1] {
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

// Scans a directory recursively filtering out files that match the fileMatch regexp
func ScanDirectoryForLogfiles(directoryPath string, fileMatch *regexp.Regexp) Logfiles {
	files := make(Logfiles, 0)
	filepath.Walk(directoryPath, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		if fileMatch.MatchString(path) {
			files = append(files, &Logfile{FileName: path})
		}
		return nil
	})
	return files
}

// A single logstream
// Implements io.Reader interface for reading from the logstream
type Logstream struct {
	// Internally used so that logfiles will only be used by a single
	// goroutine at a time. This allows external updates via the
	// UpdateLogfiles call.
	lfMutex  *sync.RWMutex
	logfiles Logfiles
	position *LogstreamLocation
	fd       *os.File
	// If the file is gzipped, reader is a gzip reader. Otherwise,
	// reader is the fd.
	reader io.Reader
	// Buffer data read in blocks so that we can control precisely how
	// much of the data read is saved in the logstream location each time
	// to saving into a record partially.
	saveBuffer []byte
	// Records whether the prior read hit an EOF
	priorEOF bool
}

func NewLogstream(logfiles Logfiles, position *LogstreamLocation) *Logstream {
	return &Logstream{
		lfMutex:    new(sync.RWMutex),
		logfiles:   logfiles,
		position:   position,
		saveBuffer: make([]byte, 0, 200),
	}
}

func (l *Logstream) DumpDebug() string {
	var finfo os.FileInfo
	if l.fd != nil {
		finfo, _ = l.fd.Stat()
	}
	return fmt.Sprintf("Logfiles: %s\n%s\nFileInfo: %s\n",
		l.logfiles.FileNames(),
		l.position.Debug(),
		finfo,
	)
}

func (l *Logstream) ReportPosition() (string, int64) {
	return l.position.Filename, l.position.SeekPosition
}

// Updates the logfiles safely
func (l *Logstream) UpdateLogfiles(logfiles Logfiles) {
	l.lfMutex.Lock()
	defer l.lfMutex.Unlock()
	l.logfiles = logfiles
}

// Save our position in the stream
func (l *Logstream) SavePosition() error {
	return l.position.Save()
}

// Get a copy of the logfiles
func (l *Logstream) GetLogfiles() (logfiles Logfiles) {
	l.lfMutex.RLock()
	defer l.lfMutex.RUnlock()
	logfiles = make(Logfiles, len(l.logfiles))
	for i, v := range l.logfiles {
		logfiles[i] = v
	}
	return
}

// Used for passing a map of logfiles
type LogfilesMap map[string]Logfiles

// A set of logstreams along with utility functions for rescanning and reparsing
// logfiles into each logstream
type LogstreamSet struct {
	logstreams     map[string]*Logstream
	rescanInterval time.Duration // Frequency of full rescan
	oldestDuration time.Duration // Filter logfiles older than this duration ago
	sortPattern    *SortPattern  // Used for creating logstreams and updating logfiles
	logstreamMutex *sync.RWMutex // Locking for manipulation of logstreams
	logRoot        string        // Base path to walk for logfiles (ie, /var/log)
	journalRoot    string        // Base path for journal files (ie, /etc/journals)
	fileMatch      *regexp.Regexp
}

// append a path separator if needed and escape regexp meta characters
func fileMatchRegexp(logRoot, fileMatch string) *regexp.Regexp {
	if !os.IsPathSeparator(logRoot[len(logRoot)-1]) && !os.IsPathSeparator(fileMatch[0]) {
		logRoot += string(os.PathSeparator)
	}

	return regexp.MustCompile("^" + regexp.QuoteMeta(logRoot) + fileMatch)
}

func NewLogstreamSet(sortPattern *SortPattern, oldest time.Duration,
	logRoot, journalRoot string) (*LogstreamSet, error) {
	// Lowercase the actual matching keys.
	newTranslation := make(SubmatchTranslationMap)
	for key, val := range sortPattern.Translation {
		newValMap := make(MatchTranslationMap)
		for matchPart, intVal := range val {
			newValMap[strings.ToLower(matchPart)] = intVal
		}
		newTranslation[key] = newValMap
	}
	sortPattern.Translation = newTranslation

	realLogRoot, err := filepath.EvalSymlinks(logRoot)
	if err != nil {
		return nil, err
	}
	ls := &LogstreamSet{
		logstreams:     make(map[string]*Logstream),
		oldestDuration: oldest,
		sortPattern:    sortPattern,
		logRoot:        realLogRoot,
		journalRoot:    journalRoot,
		logstreamMutex: new(sync.RWMutex),
		fileMatch:      fileMatchRegexp(realLogRoot, sortPattern.FileMatch),
	}
	return ls, nil
}

// Access a logstream by name if it exists
func (ls *LogstreamSet) GetLogstream(name string) (l *Logstream, ok bool) {
	ls.logstreamMutex.RLock()
	defer ls.logstreamMutex.RUnlock()
	l, ok = ls.logstreams[name]
	return
}

// Get a list of all the logstream names
func (ls *LogstreamSet) GetLogstreamNames() []string {
	ls.logstreamMutex.RLock()
	defer ls.logstreamMutex.RUnlock()
	lst := make([]string, len(ls.logstreams))
	for name, _ := range ls.logstreams {
		lst = append(lst, name)
	}
	return lst
}

// Run a scan for logstreams, creating new logstreams as needed
// Returns a list of new logstream names if logstreams were created
// as a result
func (ls *LogstreamSet) ScanForLogstreams() (result []string, errors *MultipleError) {
	var (
		logstream *Logstream
		ok        bool
	)
	result = make([]string, 0, 0)
	errors = NewMultipleError()

	// Scan for all our logfiles
	logfiles := ScanDirectoryForLogfiles(ls.logRoot, ls.fileMatch)

	// Filter out old logfiles
	if ls.oldestDuration != time.Duration(0) {
		logfiles = logfiles.FilterOld(time.Now().Add(-ls.oldestDuration))
	}

	// Setup all the sorting ints in every logfile
	logfiles.PopulateMatchParts(ls.fileMatch, ls.sortPattern.Translation)

	// Split up the logfiles into a map
	mfs := FilterMultipleStreamFiles(logfiles, ls.sortPattern.Differentiator)

	// Lock the logstream map for update
	ls.logstreamMutex.Lock()
	defer ls.logstreamMutex.Unlock()

	for name, newLogfiles := range mfs {
		logstream, ok = ls.logstreams[name]

		// New logstream files found, attempt journal path load and setup
		// the new logstream in the map, recording its newness in result
		if !ok {
			journalPath := filepath.Join(ls.journalRoot, name)
			position, err := LogstreamLocationFromFile(journalPath)
			if err != nil {
				errors.AddMessage(err.Error())
				position.Reset()
			}
			logstream = NewLogstream(nil, position)
		}

		// Add an error if there's multiple logfiles but no priority for sorting
		// Continue the loop, to avoid adding this logstream as its not usable
		if len(ls.sortPattern.Priority) == 0 && len(newLogfiles) > 1 {
			errors.AddMessage(fmt.Sprintf("Found multiple logfiles without Priority to sort "+
				"on for name: %s, filematch: %s, files: %s",
				name, ls.sortPattern.FileMatch, newLogfiles.FileNames()))
			continue
		}

		// Sort the new logfiles if there is a priority, single logfile streams
		// only have a single file to read so no sorting need occur
		// Also don't bother sorting if there's only a single logfile
		if len(ls.sortPattern.Priority) > 0 && len(newLogfiles) > 1 {
			byp := ByPriority{Logfiles: newLogfiles, Priority: ls.sortPattern.Priority}
			sort.Sort(byp)
		}

		// If this is a new logstream, its now safe to add it
		if !ok {
			result = append(result, name)
			ls.logstreams[name] = logstream
		}

		// Update the logstream with the logfiles
		logstream.UpdateLogfiles(newLogfiles)
	}
	return
}

// Filter a single Logfiles into a Logstreams keyed by the
// differentiator.
func FilterMultipleStreamFiles(files Logfiles, differentiator []string) LogfilesMap {
	mfs := make(LogfilesMap)
	for _, logfile := range files {
		name := ResolveDifferentiatedName(logfile, differentiator)
		_, ok := mfs[name]
		if !ok {
			mfs[name] = make(Logfiles, 0)
		}
		mfs[name] = append(mfs[name], logfile)
	}
	return mfs
}

// Resolve a differentiated name from a Logfile and return it.
func ResolveDifferentiatedName(l *Logfile, differentiator []string) (name string) {
	for _, d := range differentiator {
		if value, ok := l.StringMatchParts[d]; ok {
			name += value
		} else {
			name += d
		}
	}
	return
}

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
//     ["Year", "MonthName", "Day", "^Seq"]
//
// Note that "Seq" had "^" prefixed to it. This indicates that it should be sorted in
// descending order rather than the default of ascending order because in our case the
// sequence of the log number indicates how many removed from *current* it is, while the
// higher the year, month, day indicates how close it is to *current*.
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
	// If there is only a single logstream being made, the differentiator raw value
	// will be used as the logstream name
	Differentiator []string
}
