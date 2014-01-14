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
	"bytes"
	"crypto/sha1"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/mozilla-services/heka/ringbuf"
	"io"
	"os"
)

// A location in a logstream indicating the farthest that has been read
type LogstreamLocation struct {
	Filename     string           `json:"file_name"`
	SeekPosition int64            `json:"seek"`
	Hash         string           `json:"last_hash"`
	JournalPath  string           `json:"-"`
	lastLine     *ringbuf.Ringbuf `json:"-"`
}

var LINEBUFFERLEN = 500

// Loads a logstreamlocation from a file or returns an empty one if no journal
// record was found.
func LogstreamLocationFromFile(path string) (l *LogstreamLocation, err error) {
	l = new(LogstreamLocation)
	l.JournalPath = path
	l.lastLine = ringbuf.New(LINEBUFFERLEN)

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
	contents := bytes.NewBuffer(nil)
	defer seekJournal.Close()
	io.Copy(contents, seekJournal)

	defer func() {
		if r := recover(); r != nil {
			err = errors.New("Error parsing the journal file")
		}
	}()

	err = json.Unmarshal(contents.Bytes(), l)
	return
}

func (l *LogstreamLocation) Reset() {
	l.Filename = ""
	l.SeekPosition = int64(0)
	l.Hash = ""
	l.lastLine = ringbuf.New(LINEBUFFERLEN)
}

func (l *LogstreamLocation) Save() error {
	// If we don't have a JournalPath, ignore
	if l.JournalPath == "" {
		return nil
	}

	// Don't save if we had a prior has and haven't read more than
	// LINEBUFFERLEN bytes into the file
	if l.lastLine.Size() < LINEBUFFERLEN {
		return nil
	}

	lastLine := make([]byte, 0, LINEBUFFERLEN)
	n := l.lastLine.Read(lastLine)
	logline := string(lastLine[:n])

	// Note: We can't serialize the stat.pinfo in a cross platform way.
	// If you check the os.SameFile api, it only works on pinfo
	// objects created by os itself.
	if logline != "" {
		h := sha1.New()
		io.WriteString(h, logline)
		l.Hash = fmt.Sprintf("%x", h.Sum(nil))
	} else {
		l.Hash = ""
	}

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

// Determine given a position and logfiles whether there's a newer logfile available and
// we've read to the end of the prior.
//
// Rationale: This functionality is not built into LocatePriorLocation because we don't
// want to have to keep opening/closing a file when we don't know if there is another file
// available that is newer yet. This way we can keep attempting to read the last file we
// have open until we know there's a newer one and we've hit the end of the older one.
func (l *LogstreamLocation) ShouldUseNewer(files Logfiles) bool {
	lastFilename := files[len(files)-1].FileName
	if lastFilename == l.Filename {
		return false
	}
	// There are newer files, see if we're at the end of the file of our
	// position
	finfo, err := os.Stat(l.Filename)
	if err != nil {
		return false
	}
	return l.SeekPosition >= finfo.Size()
}

// Determine if a newer file is available
func (l *Logstream) NewerFileAvailable() (file string, ok bool) {
	same := false
	currentInfo := l.fd.Stat()
	fInfo := os.Stat(l.position.Filename)

	if currentInfo.Size() != fInfo.Size() {
		same = false
	}
}

// Locate and return a file handle seeked to the appropriate location. An error will be
// returned if the prior location cannot be located.
// If the logfile this location for has changed names, the position will be updated to
// reflect the move.
func (l *Logstream) LocatePriorLocation() (err error) {
	l.lfMutex.RLock()
	defer l.lfMutex.RUnlock()

	fileIndex := l.logfiles.IndexOf(l.position.Filename)
	if fileIndex != -1 {
		l.fd, err = SeekInFile(l.position.Filename, l.position)
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
	// TODO: Would be more efficient to start searching backwards from where we are
	//       in the logstream at the moment.
	for _, logfile := range l.logfiles {
		// Check that the file is large enough for our seek position
		info := os.Stat(logfile.FileName)
		if info.Size() < l.position.SeekPosition {
			continue
		}

		l.fd, err = SeekInFile(logfile.FileName, position)
		if err == nil {
			// Located the position! Update the filename in the position
			l.position.Filename = logfile.FileName
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
	if _, err = fd.Seek(position.SeekPosition-LINEBUFFERLEN, 0); err == nil {
		// We should be at the beginning of the last line read the last
		// time Heka ran.
		reader := bufio.NewReader(fd)
		buf := make([]byte, LINEBUFFERLEN)
		_, err := io.ReadAtLeast(reader, buf, int(LINEBUFFERLEN))
		if err == nil {
			h := sha1.New()
			h.Write(buf)
			tmp := fmt.Sprintf("%x", h.Sum(nil))
			if tmp == position.Hash {
				position.lastLine.Write(buf)
				return fd, nil
			}
		}
	}
	return nil, errors.New("Unable to locate position")
}

// TODO:: Refactor into a different heka package for use by all plugins
// and have PluginRunner inherit from it
type Logger interface {
	LogError(err error)
	LogMessage(msg string)
}

func (l *Logstream) Read(p []byte) (n int, err error) {
	// Do we have a file descriptor already?
	fd := l.fd

	// If we have a fd, read it
	if fd != nil {
		return l.readBytes(p)
	}

	// This is a fresh read attempt with no existing file descriptor
	// If we have a position, attempt to restore it
	if l.position.Filename != "" {
		if err = l.LocatePriorLocation(); err != nil {
			return
		} else {
			fd = l.fd
		}
	} else {
		// No position to recover from, use oldest file if there is one
		if len(l.logfiles) < 1 {
			return nil, errors.New("No files found to read from")
		}

		// Reset the position, attempt to start in the oldest file
		l.position.Reset()
		l.position.Filename = l.logfiles[0].FileName
		if err = l.LocatePriorLocation(); err != nil {
			return
		}
		fd = l.fd
	}

	return l.readBytes(p)
}

// Called to actually read from the file descriptor if possible
func (l *Logstream) readBytes(p []byte) (n int, err error) {
	// We're ready to read, commit the read and update our position
	n, err = l.fd.Read(p)

	if err != io.EOF {
		// Some unexpected error, reset everything
		// but don't kill the watcher
		l.fd.Close()
		if l.fd != nil {
			l.fd = nil
		}
		l.position.Reset()
		return
	}

	if n > 0 {
		l.position.SeekPosition += int64(n)
		l.position.lastLine.Write(p[:n])
	}

	if err == io.EOF {
		err = nil
		// We hit EOF, this is ok, but if there's a newer file switch
		// to it
		newFile, ok := l.NewerFileAvailable()

		// We do have a new file, switch to it
		if ok {
			l.fd.Close()
			l.position.Reset()
			l.position.Filename = newFile
			l.fd, err = os.Open(name)
		}
	}
	return
}

/*

Formula for EoF

- Are we the same filename?
    - Check byte length on 'filename' to ensure match
    - Check seek/last_hash on 'filename' to ensure match
- If we are file we think we are, and there's newer, proceed to newer
- If we are NOT file we think we are, locate ourself if possible, otherwise
  go to oldest in list and attempt to open

*/
