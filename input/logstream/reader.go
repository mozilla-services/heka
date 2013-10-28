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
	"code.google.com/p/go-uuid/uuid"
	"crypto/sha1"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/mozilla-services/heka/message"
	p "github.com/mozilla-services/heka/pipeline"
	"io"
	"os"
	"time"
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

func (l *LogstreamLocation) Save() error {
	// Note: We can't serialize the stat.pinfo in a cross platform way.
	// If you check the os.SameFile api, it only works on pinfo
	// objects created by os itself.
	if l.LastLogline != "" {
		h := sha1.New()
		io.WriteString(h, l.LastLogline)
		l.Hash = fmt.Sprintf("%x", h.Sum(nil))
	} else {
		l.Hash = ""
	}
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

type Logger interface {
	LogError(err error)
	LogMessage(msg string)
}

type PackCreator interface {
	ReadRecord(record []byte, parser p.StreamParser, isRotated bool, fd *os.File) (n int, err error)
	PopulatePack(record []byte, pack *p.PipelinePack)
}

type NewPackCreator func(log Logger) PackCreator

type TextPackCreator struct {
	log         Logger
	loggerIdent string
	hostname    string
}

func NewTextPackCreator(log Logger, loggerIdent, hostname string) *TextPackCreator {
	return &TextPackCreator{log: log, loggerIdent: loggerIdent, hostname: hostname}
}

func (t *TextPackCreator) ReadRecord(record []byte, parser p.StreamParser, isRotated bool, fd *os.File) (n int, err error) {
	n, record, err = parser.Parse(fd)
	if err != nil {
		if err == io.EOF && isRotated {
			record = parser.GetRemainingData()
		} else if err == io.ErrShortBuffer {
			t.log.LogError(fmt.Errorf("record exceeded MAX_RECORD_SIZE %d", message.MAX_RECORD_SIZE))
			err = nil // non-fatal, keep going
		}
	}
	return
}

func (t *TextPackCreator) PopulatePack(record []byte, pack *p.PipelinePack) {
	pack.Message.SetUuid(uuid.NewRandom())
	pack.Message.SetTimestamp(time.Now().UnixNano())
	pack.Message.SetType("logfile")
	pack.Message.SetSeverity(int32(0))
	pack.Message.SetEnvVersion("0.8")
	pack.Message.SetPid(0)
	pack.Message.SetHostname(t.hostname)
	pack.Message.SetLogger(t.loggerIdent)
	pack.Message.SetPayload(string(record))
}

type ProtobufPackCreator struct{}

func NewProtobufPackCreator(log Logger, loggerIdent, hostname string) *ProtobufPackCreator {
	return new(ProtobufPackCreator)
}

func (p *ProtobufPackCreator) ReadRecord(record []byte, parser p.StreamParser, isRotated bool, fd *os.File) (n int, err error) {
	n, record, err = parser.Parse(fd)
	return
}

func (p *ProtobufPackCreator) PopulatePack(record []byte, pack *p.PipelinePack) {
	headerLen := int(record[1]) + 3 // recsep+len+header+unitsep
	messageLen := len(record) - headerLen
	// ignore authentication headers
	if messageLen > cap(pack.MsgBytes) {
		pack.MsgBytes = make([]byte, messageLen)
	}
	pack.MsgBytes = pack.MsgBytes[:messageLen]
	copy(pack.MsgBytes, record[headerLen:])
}
