/***** BEGIN LICENSE BLOCK *****
# This Source Code Form is subject to the terms of the Mozilla Public
# License, v. 2.0. If a copy of the MPL was not distributed with this file,
# You can obtain one at http://mozilla.org/MPL/2.0/.
#
# The Initial Developer of the Original Code is the Mozilla Foundation.
# Portions created by the Initial Developer are Copyright (C) 2012-2014
# the Initial Developer. All Rights Reserved.
#
# Contributor(s):
#   Ben Bangert (bbangert@mozilla.com)
#   Rob Miller (rmiller@mozilla.com)
#
# ***** END LICENSE BLOCK *****/

package logstreamer

import (
	"bytes"
	"compress/gzip"
	"crypto/sha1"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"

	"github.com/mozilla-services/heka/ringbuf"
)

// A location in a logstream indicating the farthest that has been read
type LogstreamLocation struct {
	SeekPosition int64  `json:"seek"`
	Filename     string `json:"file_name"`
	Hash         string `json:"last_hash"`
	JournalPath  string `json:"-"`
	lastLine     *ringbuf.Ringbuf
}

var LINEBUFFERLEN = 500

// Returns whether an error is a OS file related error
func IsFileError(err error) (fileError bool) {
	switch err.(type) {
	case nil:
	case *os.SyscallError:
		fileError = true
	case *os.PathError:
		fileError = true
	case *os.LinkError:
		fileError = true
	}
	return
}

// Loads a logstreamlocation from a file or returns an empty one if no journal
// record was found.
func LogstreamLocationFromFile(path string) (l *LogstreamLocation, err error) {
	l = new(LogstreamLocation)
	l.JournalPath = path
	l.lastLine = ringbuf.New(LINEBUFFERLEN)

	// So that we can check to see if it exists or not.
	var seekJournal *os.File
	if seekJournal, err = os.Open(l.JournalPath); err != nil {
		// The logfile doesn't exist, nothing special to do.
		if os.IsNotExist(err) {
			// File doesn't exist, but that's ok, not a real error.
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

	cBytes := contents.Bytes()
	cBytes = bytes.TrimSpace(cBytes)
	if len(cBytes) == 0 {
		// File is empty, skip it.
		return
	}
	err = json.Unmarshal(cBytes, l)
	return
}

func (l *LogstreamLocation) Debug() string {
	return fmt.Sprintf("Location:\n\tFilename: %s\n\tJournal: %s\n\tSeek: %d\n\tHash: %s\n",
		l.Filename,
		l.JournalPath,
		l.SeekPosition,
		l.Hash,
	)
}

// GenerateHash generates a hash value from the contents of the ringbuffer.
// Uses the entire buffer if it is full. If we haven't advanced far enough in
// the file to fill the ring buffer, we prepend 0 bytes to pad out to the
// intended buffer length (500 bytes) before hashing.
func (l *LogstreamLocation) GenerateHash() {
	lastLine := make([]byte, LINEBUFFERLEN)
	var logline string
	rBufSize := l.lastLine.Size()
	if rBufSize == LINEBUFFERLEN {
		n := l.lastLine.Read(lastLine)
		logline = string(lastLine[:n])
	} else {
		idx := LINEBUFFERLEN - rBufSize
		n := l.lastLine.Read(lastLine[idx:])
		logline = string(lastLine[:idx+n])
	}

	if logline != "" {
		h := sha1.New()
		io.WriteString(h, logline)
		l.Hash = fmt.Sprintf("%x", h.Sum(nil))
	}
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

	l.GenerateHash()

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

// Determine if a newer file is available, if it is, return the filename of it.
func (l *Logstream) NewerFileAvailable() (file string, ok bool) {
	/* Formula for determining if a newer file is available
		   1. Are we the file we think we are?
		       NO - Find out what file we are, if we're unable to locate where we
		            are and there's logfiles, there is a newer file available
	                (the oldest).
	                If we can locate where we are, update our filename with our
	                new filename and proceed to Step 2.
		       YES - Step 2
		   2. Is there a newer file in our list ahead of us?
		       NO - No newer file available.
		       YES - return ok and the new filename.
	*/
	currentInfo, err := l.fd.Stat()
	if err != nil {
		return "", false
	}
	fInfo, err := os.Stat(l.position.Filename)
	if err != nil {
		return "", false
	}

	// 1. If our size is greater than the file at this filename, we're not the
	// same file
	if currentInfo.Size() > fInfo.Size() {
		ok = true
	} else if l.FileHashMismatch() {
		// Our file-hash didn't verify, not the same file
		ok = true
	}

	if ok {
		// 1. NO - Try and find our location
		fd, _, err := l.LocatePriorLocation(false)

		if err != nil && IsFileError(err) {
			return "", false
		}

		if fd != nil {
			fd.Close()
		}

		// Unable to locate prior position in our file-stream, are there
		// any logfiles?
		if err != nil {
			l.lfMutex.RLock()
			defer l.lfMutex.RUnlock()
			if len(l.logfiles) > 0 {
				file = l.logfiles[0].FileName
				return
			} else {
				// Apparently no logfiles at all, retain this fd
				ok = false
				return
			}
		}

		// We were able to locate our prior location, our filename was
		// updated
		ok = false
	}

	// 2. Newer file ahead of us?
	l.lfMutex.RLock()
	defer l.lfMutex.RUnlock()
	fileIndex := l.logfiles.IndexOf(l.position.Filename)
	if fileIndex == -1 {
		// We couldn't find our filename in the list? Then there's nothing
		// newer
		return
	}

	if fileIndex+1 < len(l.logfiles) {
		// There's a newer file!
		return l.logfiles[fileIndex+1].FileName, true
	}

	return
}

// FileHashMismatch uses the file path, seek location, and hash stored in the
// Logstream's `position` attribute. It checks to see if the hash matches the
// contents of the stored file path at the specified position. It returns true
// if and only if it can be *concretely* shown that the stored hash in fact
// does *not* match the contents of the file. If there is a match, or if for
// any reason we can't determine if there is a match, then FileHashMismatch
// will return false.
func (l *Logstream) FileHashMismatch() bool {
	// We always match our hash if we have no hash
	if l.position.Hash == "" {
		return false
	}

	fd, _, err := SeekInFile(l.position.Filename, l.position)
	if err == ErrorCantSeekPosition {
		return true
	} else if err == nil {
		fd.Close()
	}
	return false
}

// Locate and return a file handle seeked to the appropriate location. An error will be
// returned if the prior location cannot be located.
// If the logfile this location for has changed names, the position will be updated to
// reflect the move.
func (l *Logstream) LocatePriorLocation(checkFilename bool) (fd *os.File, reader io.Reader, err error) {
	var info os.FileInfo
	l.lfMutex.RLock()
	defer l.lfMutex.RUnlock()

	if checkFilename {
		fileIndex := l.logfiles.IndexOf(l.position.Filename)
		if fileIndex != -1 {
			fd, reader, err = SeekInFile(l.position.Filename, l.position)
			if err == nil {
				return
			}
			// Check to see whether its a file error, return if it is
			if IsFileError(err) {
				return
			}
			err = nil // Reset our error to nil
		}
	}

	// Unable to locate the file, or the position wasn't where we thought it should be.
	// Start systematically searching all the files for this location to see if it was
	// shuffled around.
	// TODO: Would be more efficient to start searching backwards from where we are
	//       in the logstream at the moment.
	for _, logfile := range l.logfiles {
		// Check that the file is large enough for our seek position
		info, err = os.Stat(logfile.FileName)
		if err != nil {
			return
		}
		if !isGzipFile(logfile.FileName) {
			if info.Size() < l.position.SeekPosition {
				continue
			}
		}

		fd, reader, err = SeekInFile(logfile.FileName, l.position)
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
	// Set our default error since we were unable to locate the position
	err = errors.New("Unable to locate position in the stream")
	return
}

// Returns an io.Reader. If file is gzipped, returns a gzip.Reader.
func createFileReader(path string, fd *os.File) (reader io.Reader, err error) {
	if isGzipFile(path) {
		reader, err = gzip.NewReader(fd)
	} else {
		reader = fd
	}
	return
}

// Guesses if the given file is gzipped.
func isGzipFile(path string) bool {
	file, err := os.Open(path)
	if err != nil {
		return false
	}
	defer file.Close()

	magic := make([]byte, 2)
	numbytes, err := file.Read(magic)
	if numbytes != 2 || err != nil {
		return false
	}

	return (magic[0] == 0x1f && magic[1] == 0x8b)
}

var ErrorCantSeekPosition = errors.New("Unable to locate position")
var ErrorCantGzipToPosition = errors.New("Couldn't read gzip to seek position")

// SeekInFile opens the file at the given path, seeks to the location
// specified in the given position, hashes the contents of the file at that
// position, and compares that to the hash value stored in the position
// argument. If the hashes match, or if the seek position is the beginning of
// the file so there's no hash to compare, then it will return the open file
// descriptor and related io.Reader. If they do not, or anything goes wrong
// along the way, then an error will be returned. Note that the fd and the
// io.Reader will be the same except in cases where the file was gzipped, in
// which case the io.Reader will be the gzip reader and not the raw file
// descriptor.
func SeekInFile(path string, position *LogstreamLocation) (*os.File, io.Reader, error) {
	fd, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}

	var reader io.Reader

	// Try to get to our seek position, if our seek is 0, then start at the
	// beginning.
	if position.SeekPosition == 0 {
		reader, err = createFileReader(path, fd)
		return fd, reader, err
	}

	seekPos := position.SeekPosition - int64(LINEBUFFERLEN)
	gzipped := isGzipFile(path)
	if gzipped {
		reader, err = gzip.NewReader(fd)
		if err != nil {
			return nil, nil, err
		}
		if seekPos > 0 {
			n, err := io.CopyN(ioutil.Discard, reader, seekPos)
			if err != nil {
				return nil, nil, err
			}
			if n != seekPos {
				return nil, nil, ErrorCantGzipToPosition
			}
		}
	} else {
		reader = fd
		if seekPos > 0 {
			_, err = fd.Seek(seekPos, 0)
			if err != nil {
				return nil, nil, err
			}
		}
	}

	// At this point we have seeked into the file to the beginning of the
	// content used to generate the hash last time Heka ran. Now we copy that
	// data into a byte slice (prepending zero bytes if necessary) and
	// recalculate the hash to see if we're in fact looking at the same
	// content.
	buf := make([]byte, LINEBUFFERLEN)
	var (
		n         int
		expectedN int64
	)
	if seekPos >= 0 {
		n, err = reader.Read(buf)
		expectedN = int64(LINEBUFFERLEN)
	} else {
		n, err = reader.Read(buf[-seekPos:])
		expectedN = position.SeekPosition
	}
	if err == nil && int64(n) == expectedN {
		h := sha1.New()
		h.Write(buf)
		tmp := fmt.Sprintf("%x", h.Sum(nil))
		if tmp == position.Hash {
			position.lastLine.Write(buf)
			return fd, reader, nil
		}
	}
	return nil, nil, ErrorCantSeekPosition
}

// TODO:: Refactor into a different heka package for use by all plugins
// and have PluginRunner inherit from it
type Logger interface {
	LogError(err error)
	LogMessage(msg string)
}

// Flushes the save buffer to the position tracking to control how far
// into a file the Logstream tracks itself at regardless of read buffers
// that may have read farther than we wish to actually save.
func (l *Logstream) FlushBuffer(n int) {
	if n == 0 || n > len(l.saveBuffer) {
		n = len(l.saveBuffer)
	}

	l.position.SeekPosition += int64(n)
	l.position.lastLine.Write(l.saveBuffer[:n])

	// Copy the remainder over the portion that was saved
	copy(l.saveBuffer, l.saveBuffer[n:len(l.saveBuffer)])
	l.saveBuffer = l.saveBuffer[:len(l.saveBuffer)-n]
}

// Save the location to the buffer
func (l *Logstream) BufferSave(p []byte) {
	n := len(p)

	// Enlarge our save buffer to match what we were handed, and copy
	// our current buffer over
	sbLen := len(l.saveBuffer)
	if cap(l.saveBuffer) < cap(p) {
		newBuf := make([]byte, sbLen, cap(p))
		copy(newBuf, l.saveBuffer)
		l.saveBuffer = newBuf
	}

	// Flush the current buffer if the new data won't fit
	if sbLen+n > cap(l.saveBuffer) {
		l.FlushBuffer(sbLen)
		sbLen = 0
	}

	// Copy the new data into our saveBuffer
	l.saveBuffer = l.saveBuffer[:sbLen+n]
	copy(l.saveBuffer[sbLen:], p)
}

func (l *Logstream) Read(p []byte) (n int, err error) {
	// If we have a fd, read it
	if l.fd != nil {
		return l.readBytes(p)
	}

	// This is a fresh read attempt with no existing file descriptor
	// If we have a position, attempt to restore it
	var fd *os.File
	var reader io.Reader
	if l.position.Filename != "" {
		if fd, reader, err = l.LocatePriorLocation(true); err == nil {
			l.fd = fd
			l.reader = reader
			return l.readBytes(p)
		}
		// Did we get an OS level error attempting to open a file somewhere?
		// These syscall errors should be retried and reported, but not
		// clear out the position since it may still be valid.
		if IsFileError(err) {
			return 0, err
		}
	}

	// No position to recover from, use oldest file if there is one
	if len(l.logfiles) < 1 {
		// No oldest file, so right now we can't proceed
		return 0, io.EOF
	}

	// Reset the position, attempt to start in the oldest file
	l.position.Reset()
	l.position.Filename = l.logfiles[0].FileName
	return l.Read(p)
}

// Called to actually read from the file descriptor if possible
func (l *Logstream) readBytes(p []byte) (n int, err error) {
	// Before we read, we check to see if there's a newer file
	// If there is a newer file, then we know that if we hit EOF here
	// the new one has already started getting data so its safe to move
	// on. If we did this check after hitting EOF, then its possible we
	// could move on without doing a last read of the fd.
	var (
		newerFilename string
		ok            bool
	)

	// If we had an EOF last time, we check for a new file before trying
	// to read again
	if l.priorEOF {
		l.position.GenerateHash()
		newerFilename, ok = l.NewerFileAvailable()
	}

	// We're ready to read, commit the read and update our position
	// TODO: should anything be done with the fd?
	n, err = l.reader.Read(p)

	// If we read any bytes, write them to our saveBuffer.
	// If our saveBuffer is smaller than the buffer we were just passed,
	// then enlarge the saveBuffer first.
	// If the current bytes in the saveBuffer plus the new bytes are
	// larger than the saveBuffer, flush the existing saveBuffer first.
	if n > 0 {
		l.BufferSave(p[:n])
	}

	// Return now if we didn't get an error
	if err == nil {
		// Had an EOF before, clear it
		if l.priorEOF {
			l.priorEOF = false
		}
		return
	}

	if err != io.EOF {
		// Had an EOF before, clear it
		if l.priorEOF {
			l.priorEOF = false
		}

		// Some unexpected error, reset everything
		l.fd.Close()
		l.fd = nil
		l.reader = nil
		l.position.Reset()
		return
	}

	// At this point, it must be an EOF, determine if we previously had
	// one
	if !l.priorEOF {
		// Record that we got an EOF and try again to see if there's
		// newer since we only bump an EOF back on read if its not
		// possible to proceed to a new file and we're at the end
		l.priorEOF = true
		// We also need to attempt to save our current location at the
		// possible EOF
		l.position.Save()
		return l.Read(p)
	}

	if !ok {
		// We don't have a newer file, so we will keep checking for a newer
		// file and return the EOF to now indicating we can proceed no
		// further
		return
	}

	// At this point in time, we have a prior EOF, and an EOF this time
	// and we have located a newer file. We will clear our error and attempt
	// to rotate the file we're reading since we've hit the end and can
	// proceed.
	err = nil

	// Attempt to grab the file handle of the new file
	var fd *os.File
	fd, err = os.Open(newerFilename)
	if err != nil {
		// Return the error, keep our existing handle
		fd.Close()
		return
	}

	// Create a gzip reader if needed.
	var reader io.Reader
	reader, err = createFileReader(newerFilename, fd)
	if err != nil {
		return
	}

	// Verify that our newerFilename is still what we think it should
	// be and our files didn't move around between calls, if we were
	// rotated after the other NewerFileAvailable call then the filename
	// here will be different
	verifyFilename, vOk := l.NewerFileAvailable()
	if verifyFilename != newerFilename || !vOk {
		fd.Close()
		// Now try again, hopefully we capture it after rotation this
		// time, or maybe there's a last batch of data to read
		return l.Read(p)
	}

	// Ok, we have the handle for the right file, even if it might've
	// been rotated by now. Commit a flush first.
	l.FlushBuffer(0)
	l.fd.Close()
	l.position.Reset()

	l.position.Filename = newerFilename
	l.fd = fd
	l.reader = reader
	l.priorEOF = false

	// Now attempt to read
	return l.Read(p)
}
