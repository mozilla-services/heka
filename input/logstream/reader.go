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
	"sync"
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

func (l *LogstreamLocation) Reset() {
	l.Filename = ""
	l.SeekPosition = int64(0)
	l.LastLoglineStart = int64(0)
	l.LastLogline = ""
	l.LastLoglineLength = int64(0)
	l.Hash = ""
}

func (l *LogstreamLocation) Save() error {
	// If we don't have a JournalPath, ignore
	if l.JournalPath == "" {
		return nil
	}

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
	for i, logfile := range files {
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

// TODO:: Refactor into a different heka package for use by all plugins
// and have PluginRunner inherit from it
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

// A logfile resumer holds information on the logfiles that are available
// and where in the logstream to start reading.
type LogfileResumer struct {
	// Internally used so that logfiles will only be used by a single
	// goroutine at a time. This allows external updates via the
	// UpdateLogfiles call.
	updateMutex *sync.Mutex
	logfiles    Logfiles
	position    *LogstreamLocation
	// A duration string suitable for time.ParseDuration
	oldestDuration time.Duration
}

func NewLogfileResumer(logfiles Logfiles, position *LogstreamLocation,
	oldestDuration string) (l *LogfileResumer, err error) {
	var d time.Duration
	if oldestDuration != "" {
		d, err = time.ParseDuration(oldestDuration)
		if err != nil {
			return
		}
	}
	l = &LogfileResumer{
		updateMutex:    new(sync.Mutex),
		logfiles:       logfiles,
		position:       position,
		oldestDuration: d,
	}
	return
}

// Finds the file to start reading from, resumes position in the file
// if possible, returns a file descriptor ready for reading.
func (l *LogfileResumer) ResumeFileReading() (f *os.File, err error) {
	l.updateMutex.Lock()
	defer l.updateMutex.Unlock()

	if l.position.Filename != "" {
		f, err = LocatePriorLocation(l.logfiles, l.position)
		if err == nil {
			return
		}
		// Unable to locate prior location, clear out error
		err = nil
	}

	// No prior location, ensure we're reset
	l.position.Reset()

	// If we have a oldest duration, filter the logfiles and grab the
	// oldest, otherwise start with the most recent
	var filename string
	files := l.logfiles

	if l.oldestDuration != 0 {
		files = l.logfiles.FilterOld(time.Now().Add(-l.oldestDuration))
		if len(files) < 1 {
			return nil, errors.New("No file has new enough modifications to read.")
		}
		filename = files[0].FileName
	} else {
		filename = l.logfiles[len(l.logfiles)-1].FileName
	}
	l.position.Filename = filename
	f, err = LocatePriorLocation(files, l.position)
	return
}

// Updates the logfiles safely
func (l *LogfileResumer) UpdateLogfiles(files Logfiles) {
	l.updateMutex.Lock()
	defer l.updateMutex.Unlock()
	l.logfiles = files
}

// PackGenerator will continue to return packs given a LogfileResumer and
// PackCreator as long as the file its reading from hasn't moved. If the file
// its reading gets truncated, it will reset to the beginning of the file. The
// user of PackGenerator should occasionally determine if there's a newer file
// that can be read using ShouldUseNewer and make a new PackGenerator in that
// case after updating the position.
type PackGenerator struct {
	lfr          LogfileResumer
	pc           PackCreator
	statInterval int
	fd           *os.File
}

func NewPackGenerator(lfr LogfileResumer, pc PackCreator, statInterval int) *PackGenerator {
	return &PackGenerator{lfr: lfr, pc: pc, statInterval: statInterval}
}

func (pg *PackGenerator) Run(packFeed <-chan *p.PipelinePack, packReturn chan<- *p.PipelinePack) {
	go pg.ReadFile(packFeed, packReturn)
	return
}

func (pg *PackGenerator) ReadFile(packFeed <-chan *p.PipelinePack, packReturn chan<- *p.PipelinePack) {
	var (
		ok, shouldStat, isRotated bool
		pack                      *p.PipelinePack
		fd                        *os.File
		err                       error
		position                  *LogstreamLocation
		record                    []byte
		bytesRead                 int
		priorRead                 int
	)

	// Check that we haven't been rotated, if we have, update the filename
	// to reflect our new location.
	fd, err = pg.lfr.ResumeFileReading()
	if err != nil {
		close(packReturn)
		return
	}
	position = pg.lfr.position
	record = make([]byte, 0, 200)

	// Determine if we're farther into the file than possible (truncate)
	finfo, err := fd.Stat()
	if err == nil && finfo.Size() < position.SeekPosition {
		fd.Seek(0, 0)
		position.SeekPosition = 0
	}

	pinfo, err := os.Stat(pg.lfr.position.Filename)
	if err != nil || !os.SameFile(pinfo, finfo) {
		filename = pinfo.Name()
		if position.ShouldUseNewer(pg.lfr.logfiles) {
			isRotated = true
		}
	}

	newFilename, isRotated := pg.IsRotated()
	if isRotated {
		position.Filename = newFilename
	}

	for err != nil {
		record = record[:0]
		priorRead = bytesRead

		pack, ok = <-packFeed
		// Told to close down?
		if !ok {
			err = errors.New("Pack channel closed")
			continue
		}

		bytesRead, err = pg.pc.ReadRecord(record, parser, isRotated, fd)
		if err == nil {
			position.LastLogline = string(record)
			position.LastLoglineStart += priorRead
			pg.pc.PopulatePack(record, pack)
			packReturn <- pack
			pack = nil
			shouldStat = false
		} else {
			<-time.After(time.Duration(pg.statInterval) * time.Millisecond)
			shouldStat = true
		}
	}
	// If we have a pack, return it
	if pack != nil {
		packReturn <- pack
	}
}
