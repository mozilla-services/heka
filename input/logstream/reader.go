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

package logstream

import (
	"bufio"
	"crypto/sha1"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
)

// A location in a logstream indicating the farthest that has been read
type LogstreamLocation struct {
	Filename          string `json:"file_name"`
	SeekPosition      int64  `json:"seek"`
	LastLoglineStart  int64  `json:"last_start"`
	LastLogline       string `json:"-"`
	LastLoglineLength int64  `json:"last_len"`
	Hash              string `json:"last_hash"`
	JournalPath       string `json:"-"`
}

// Loads a logstreamlocation from a file or returns an empty one if no journal
// record was found.
func LogstreamLocationFromFile(path string) (l *LogstreamLocation, err error) {
	l = new(LogstreamLocation)
	l.JournalPath = path

	// So that we can check to see if it exists or not
	var seekJournal *os.File
	if seekJournal, err = os.Open(l.JournalPath); err != nil {
		// The logfile doesn't exist, nothing special to do
		if os.IsNotExist(err) {
			// file doesn't exist, but that's ok, not a real error
			err = nil
		}
		return
	}
	seekJournal.Close()

	defer func() {
		if r := recover(); r != nil {
			err = errors.New("Error parsing the journal file")
		}
	}()

	contents, err := ioutil.ReadFile(l.JournalPath)
	err = json.Unmarshal(contents, l)
	return
}

func (l *LogstreamLocation) Save() error {
	// Note: We can't serialize the stat.pinfo in a cross platform way.
	// If you check the os.SameFile api, it only works on pinfo
	// objects created by os itself.
	h := sha1.New()
	io.WriteString(h, l.LastLogline)
	l.Hash = fmt.Sprintf("%x", h.Sum(nil))
	l.LastLoglineLength = int64(len(l.LastLogline))

	b, err := json.Marshal(l)
	if err != nil {
		return err
	}

	seekJournal, file_err := os.OpenFile(l.JournalPath, os.O_CREATE|os.O_RDWR|os.O_TRUNC, 0660)
	if file_err != nil {
		return fmt.Errorf("Error opening seek recovery log: %s", file_err.Error())
	}
	defer seekJournal.Close()

	if _, file_err = seekJournal.Write(b); file_err != nil {
		return fmt.Errorf("Error writing seek recovery log: %s", file_err.Error())
	}
	return nil
}

// Locate and return a file handle seeked to the appropriate location. An error will be
// returned if the prior location cannot be located.
// If the logfile this location for has changed names, the position will be updated to
// reflect the move.
func LocatePriorLocation(files Logfiles, position *LogstreamLocation) (fd *os.File, err error) {
	fileIndex := files.IndexOf(position.Filename)
	if fileIndex != -1 {
		fd, err = SeekInFile(position.Filename, position)
		if err == nil {
			return
		}
		// Check to see whether its a file permission error, return if it is
		if os.IsPermission(err) {
			return
		}
		err = nil // Reset our error to nil
	}

	// Unable to locate the file, or the position wasn't where we thought it should be.
	// Start systematically searching all the files for this location to see if it was
	// shuffled around.
	for _, logfile := range files {
		fd, err = SeekInFile(logfile.FileName, position)
		if err == nil {
			// Located the position! Update the filename in the position
			position.Filename = logfile.FileName
			return
		}
		// Check to see whether its a file permission error, return if it is
		if os.IsPermission(err) {
			return
		}
		err = nil // Reset our error to nil
	}
	return
}

// Seek into a file, return an error if a match wasn't found
func SeekInFile(path string, position *LogstreamLocation) (fd *os.File, err error) {
	if fd, err = os.Open(path); err != nil {
		return
	}

	// Try to get to our seek position.
	if _, err = fd.Seek(position.LastLoglineStart, 0); err == nil {
		// We should be at the beginning of the last line read the last
		// time Heka ran.
		reader := bufio.NewReader(fd)
		buf := make([]byte, position.LastLoglineLength)
		_, err := io.ReadAtLeast(reader, buf, int(position.LastLoglineLength))
		if err == nil {
			h := sha1.New()
			h.Write(buf)
			tmp := fmt.Sprintf("%x", h.Sum(nil))
			if tmp == position.Hash {
				position.LastLogline = string(buf)
				return fd, nil
			}
		}
	}
	return nil, errors.New("Unable to locate position")
}
