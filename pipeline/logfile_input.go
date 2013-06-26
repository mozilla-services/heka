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
#   Rob Miller (rmiller@mozilla.com)
#   Victor Ng (vng@mozilla.com)
#
# ***** END LICENSE BLOCK *****/

package pipeline

import (
	"bufio"
	"bytes"
	"code.google.com/p/go-uuid/uuid"
	"crypto/sha1"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"
	"strings"
	"time"
)

// ConfigStruct for LogfileInput plugin.
type LogfileInputConfig struct {
	// Paths for the log file that this input should be reading.
	LogFile string
	// Hostname to use for the generated logfile message objects.
	Hostname string
	// Interval btn hd scans for existence of watched files, in milliseconds,
	// default 5000 (i.e. 5 seconds).
	DiscoverInterval int
	// Interval btn reads from open file handles, in milliseconds, default
	// 500.
	StatInterval int
	// Names of configured `LoglineDecoder` instances.
	Decoders []string

	// Name of the seek journal
	SeekJournal string

	Logger string

	// On failure to resume from last known position, LogfileInput
	// will resume reading from either the start of file or the end of
	// file.
	ResumeFromStart bool
}

// Heka Input plugin that reads files from the filesystem, converts each line
// into a fully decoded Message object with the line contents as the payload,
// and passes the generated message on to the Router for delivery to any
// matching Filter or Output plugins.
type LogfileInput struct {
	// Encapsulates actual file finding / listening / reading mechanics.
	Monitor      *FileMonitor
	hostname     string
	stopped      bool
	decoderNames []string
}

// Represents a single line from a log file.
type Logline struct {
	// Path to the file from which the line was extracted.
	Path string
	// Log file line contents.
	Line string

	// Name of the logger we're using
	Logger string
}

func (lw *LogfileInput) ConfigStruct() interface{} {
	return &LogfileInputConfig{
		DiscoverInterval: 5000,
		StatInterval:     500,
		SeekJournal:      "",
		ResumeFromStart:  true,
	}
}

func (lw *LogfileInput) Init(config interface{}) (err error) {
	conf := config.(*LogfileInputConfig)
	lw.Monitor = new(FileMonitor)
	val := conf.Hostname
	if val == "" {
		val, err = os.Hostname()
		if err != nil {
			return
		}
	}
	lw.hostname = val
	if err = lw.Monitor.Init(conf); err != nil {
		return err
	}
	lw.decoderNames = conf.Decoders

	return nil
}

func (lw *LogfileInput) Run(ir InputRunner, h PluginHelper) (err error) {
	var (
		pack    *PipelinePack
		dRunner DecoderRunner
		e       error
		ok      bool
	)
	packSupply := ir.InChan()

	lw.Monitor.ir = ir

	for _, msg := range lw.Monitor.pendingMessages {
		lw.Monitor.LogMessage(msg)
	}

	for _, msg := range lw.Monitor.pendingErrors {
		lw.Monitor.LogError(msg)
	}

	// Clear out all the errors
	lw.Monitor.pendingMessages = make([]string, 0)
	lw.Monitor.pendingErrors = make([]string, 0)

	dSet := h.DecoderSet()
	decoders := make([]Decoder, len(lw.decoderNames))
	for i, name := range lw.decoderNames {
		if dRunner, ok = dSet.ByName(name); !ok {
			return fmt.Errorf("Decoder not found: %s", name)
		}
		decoders[i] = dRunner.Decoder()
	}

	for logline := range lw.Monitor.NewLines {
		pack = <-packSupply
		pack.Message.SetUuid(uuid.NewRandom())
		pack.Message.SetTimestamp(time.Now().UnixNano())
		pack.Message.SetType("logfile")
		pack.Message.SetLogger(logline.Logger)
		pack.Message.SetSeverity(int32(0))
		pack.Message.SetEnvVersion("0.8")
		pack.Message.SetPid(0)
		pack.Message.SetPayload(logline.Line)
		pack.Message.SetHostname(lw.hostname)
		for _, decoder := range decoders {
			if e = decoder.Decode(pack); e == nil {
				break
			}
		}
		if e == nil {
			ir.Inject(pack)
		} else {
			ir.LogError(fmt.Errorf("Couldn't parse log line: %s", logline.Line))
			pack.Recycle()
		}
	}
	return
}

func (lw *LogfileInput) Stop() {
	close(lw.Monitor.stopChan) // stops the monitor's watcher
	close(lw.Monitor.NewLines)
}

// FileMonitor, manages a group of FileTailers
//
// Handles the actual mechanics of finding, watching, and reading from file
// system files.
type FileMonitor struct {
	// Channel onto which FileMonitor will place LogLine objects as the file
	// is being read.
	NewLines chan Logline
	stopChan chan bool
	seek     int64

	logfile         string
	seekJournalPath string
	discover        bool
	logger_ident    string

	fd               *os.File
	checkStat        <-chan time.Time
	discoverInterval time.Duration
	statInterval     time.Duration

	ir              InputRunner
	pendingMessages []string
	pendingErrors   []string

	last_logline    string
	resumeFromStart bool
}

// Serialize to JSON
func (fm *FileMonitor) MarshalJSON() ([]byte, error) {
	// Note: We can't serialize the stat.pinfo in a cross platform way.
	// If you check the os.SameFile api, it only works on pinfo
	// objects created by os itself.

	h := sha1.New()
	io.WriteString(h, fm.last_logline)
	tmp := map[string]interface{}{
		"seek":      fm.seek,
		"last_hash": fmt.Sprintf("%x", h.Sum(nil)),
	}

	return json.Marshal(tmp)
}

func sha1_hexdigest(data string) (result string) {
	h := sha1.New()
	io.WriteString(h, data)
	return fmt.Sprintf("%x", h.Sum(nil))
}

func (fm *FileMonitor) UnmarshalJSON(data []byte) (err error) {
	var dec = json.NewDecoder(bytes.NewReader(data))
	var m map[string]interface{}

	err = dec.Decode(&m)
	if err != nil {
		return fmt.Errorf("Caught error while decoding json blob: %s", err.Error())
	}

	var seek_pos = int64(m["seek"].(float64))
	var last_hash = m["last_hash"].(string)

	var fd *os.File
	if fd, err = os.Open(fm.logfile); err != nil {
		return
	}
	defer fd.Close()

	// Try to get to our seek position.
	if _, err = fd.Seek(seek_pos, 0); err == nil {
		// We got there, now move backwards through the file until we get to
		// the beginning of the line.
		char := make([]byte, 1)
		for char[0] != []byte("\n")[0] {

			// Our first backwards seek skips over what should be a trailing
			// "\n", subsequent ones skip over the byte that we just read.
			if _, err = fd.Seek(-2, 1); err != nil {
				break
			}
			if _, err = fd.Read(char); err != nil {
				break
			}
		}

		if err == nil {
			// We should be at the beginning of the last line read the last
			// time Heka ran.
			reader := bufio.NewReader(fd)
			var readLine string
			if readLine, err = reader.ReadString('\n'); err == nil {
				if sha1_hexdigest(readLine) == last_hash {
					// woot.  same log file
					fm.seek = seek_pos
					msg := fmt.Sprintf("Line matches, continuing from byte pos: %d", seek_pos)
					fm.LogMessage(msg)
					return nil
				}
				fm.LogMessage("Line mismatch.")
			}
		}
	}
	var msg string
	if fm.resumeFromStart {
		fm.seek = 0
		msg = "Restarting from start of file."
	} else {
		fm.seek, _ = fd.Seek(0, 2)
		msg = fmt.Sprintf("Restarting from end of file [%d].", fm.seek)
	}
	fm.LogMessage(msg)
	return nil
}

// Tries to open specified file, adding file descriptor to the FileMonitor's
// set of open descriptors.
func (fm *FileMonitor) OpenFile(fileName string) (err error) {
	// Attempt to open the file
	fd, err := os.Open(fileName)
	if err != nil {
		return
	}
	fm.fd = fd

	// Seek as needed
	begin := 0
	offset := fm.seek
	_, err = fd.Seek(offset, begin)
	if err != nil {
		// Unable to seek in, start at beginning
		fm.seek = 0
		if _, err = fd.Seek(0, 0); err != nil {
			return
		}
	}
	return nil
}

// Runs in its own goroutine, listens for interval tickers which trigger it to
// a) try to open any upopened files and b) read any new data from already
// opened files.
func (fm *FileMonitor) Watcher() {
	discovery := time.Tick(fm.discoverInterval)
	checkStat := time.Tick(fm.statInterval)

	ok := true

	for ok {
		select {
		case _, ok = <-fm.stopChan:
			break
		case <-checkStat:
			if fm.fd != nil {
				ok = fm.ReadLines(fm.logfile)
				if !ok {
					break
				}
			}
		case <-discovery:
			// Check to see if the files exist now, start reading them
			// if we can, and watch them
			if fm.OpenFile(fm.logfile) == nil {
				fm.discover = false
			}
		}
	}
	if fm.fd != nil {
		fm.fd.Close()
		fm.fd = nil
	}
}

func (fm *FileMonitor) updateJournal(bytes_read int64) (ok bool) {
	var msg string
	var seekJournal *os.File
	var file_err error

	if bytes_read == 0 || fm.seekJournalPath == "." {
		return true
	}

	if seekJournal, file_err = os.OpenFile(fm.seekJournalPath,
		os.O_CREATE|os.O_RDWR|os.O_APPEND,
		0660); file_err != nil {
		msg = fmt.Sprintf("Error opening seek recovery log for append: %s", file_err.Error())
		fm.LogError(msg)
		return false
	}
	defer seekJournal.Close()
	seekJournal.Seek(0, os.SEEK_END)

	var filemon_bytes []byte
	filemon_bytes, _ = json.Marshal(fm)

	msg = string(filemon_bytes) + "\n"
	seekJournal.WriteString(msg)

	return true
}

// Reads all unread lines out of the specified file, creates a LogLine object
// for each line, and puts it on the NewLine channel for processing.
// Returning false from ReadLines will kill the watcher
func (fm *FileMonitor) ReadLines(fileName string) (ok bool) {
	ok = true
	var bytes_read int64

	defer func() {
		// Capture send on close chan as this is a shut-down
		if r := recover(); r != nil {
			rStr := fmt.Sprintf("%s", r)
			if strings.Contains(rStr, "send on closed channel") {
				ok = false
				// We're only partially through a file, write to the seekjournal.
				fm.seek += bytes_read
				fm.updateJournal(bytes_read)
			} else {
				panic(rStr)
			}
		}
	}()

	// Determine if we're farther into the file than possible (truncate)
	finfo, err := fm.fd.Stat()
	if err == nil {
		if finfo.Size() < fm.seek {
			fm.fd.Seek(0, 0)
			fm.seek = 0
		}
	}

	// Check that we haven't been rotated, if we have, put this
	// back on discover
	isRotated := false
	pinfo, err := os.Stat(fileName)
	if err != nil || !os.SameFile(pinfo, finfo) {
		isRotated = true
		defer func() {
			if fm.fd != nil {
				fm.fd.Close()
			}
			fm.fd = nil
			fm.seek = 0
			fm.discover = true
		}()
	}

	// Attempt to read lines from where we are.
	reader := bufio.NewReader(fm.fd)
	readLine, err := reader.ReadString('\n')
	for err == nil {
		line := Logline{Path: fileName, Line: readLine, Logger: fm.logger_ident}
		fm.NewLines <- line
		bytes_read += int64(len(readLine))
		fm.last_logline = readLine

		// If file rotation happens after the last
		// reader.ReadString() in this loop, the remaining logfile
		// data will be picked up the next time that ReadLines() is
		// invoked
		readLine, err = reader.ReadString('\n')
	}

	if err == io.EOF {
		if isRotated {
			if len(readLine) > 0 {
				line := Logline{Path: fileName, Line: readLine, Logger: fm.logger_ident}
				fm.NewLines <- line
				bytes_read += int64(len(readLine))
				fm.last_logline = readLine
			}
		} else {
			fm.fd.Seek(-int64(len(readLine)), os.SEEK_CUR)
		}
	} else {
		// Some unexpected error, reset everything
		// but don't kill the watcher
		fm.LogError(err.Error())
		fm.fd.Close()
		if fm.fd != nil {
			fm.fd = nil
		}
		fm.seek = 0
		fm.discover = true
		return true
	}

	fm.seek += bytes_read
	return fm.updateJournal(bytes_read)
}

func (fm *FileMonitor) LogError(msg string) {
	if fm.ir == nil {
		fm.pendingErrors = append(fm.pendingErrors, msg)
	} else {
		fm.ir.LogError(fmt.Errorf(msg))
	}
}

func (fm *FileMonitor) LogMessage(msg string) {
	if fm.ir == nil {
		fm.pendingMessages = append(fm.pendingMessages, msg)
	} else {
		fm.ir.LogMessage(msg)
	}
}

func (fm *FileMonitor) Init(conf *LogfileInputConfig) (err error) {
	file := conf.LogFile
	discoverInterval := conf.DiscoverInterval
	statInterval := conf.StatInterval
	logger := conf.Logger

	fm.resumeFromStart = conf.ResumeFromStart

	fm.NewLines = make(chan Logline)
	fm.stopChan = make(chan bool)
	fm.seek = 0
	fm.fd = nil

	fm.logfile = file
	fm.discover = true

	fm.pendingMessages = make([]string, 0)
	fm.pendingErrors = make([]string, 0)
	fm.seekJournalPath = conf.SeekJournal

	if logger != "" {
		fm.logger_ident = logger
	} else {
		fm.logger_ident = file
	}

	fm.discoverInterval = time.Millisecond * time.Duration(discoverInterval)
	fm.statInterval = time.Millisecond * time.Duration(statInterval)

	if err = fm.setupJournalling(); err != nil {
		return
	}

	go fm.Watcher()

	return
}

func (fm *FileMonitor) recoverSeekPosition() (err error) {
	// Check if the file exists first,
	if fm.seekJournalPath == "." {
		return
	}

	var f *os.File

	if f, err = os.Open(fm.seekJournalPath); err != nil {
		// The logfile doesn't exist, nothing special to do
		if os.IsNotExist(err) {
			// file doesn't exist, but that's ok, not a real error
			return nil
		} else {
			return
		}
	}
	defer f.Close()

	var seekJournal *os.File
	if seekJournal, err = os.OpenFile(fm.seekJournalPath,
		os.O_RDWR, 0660); err != nil {
		return
	}
	defer seekJournal.Close()

	var scanner = bufio.NewScanner(seekJournal)
	var tmp string
	for scanner.Scan() {
		tmp = scanner.Text()
	}
	if len(tmp) > 0 {
		json.Unmarshal([]byte(tmp), &fm)
	}

	return
}

func (fm *FileMonitor) setupJournalling() (err error) {
	var dirInfo os.FileInfo

	fm.cleanJournalPath()

	// Check that the directory to seekJournalPath actually exists
	journalDir := path.Dir(fm.seekJournalPath)

	if dirInfo, err = os.Stat(journalDir); err != nil {
		fm.LogMessage(fmt.Sprintf("%s parent dir doesn't exist", fm.seekJournalPath))
		return
	}

	if !dirInfo.IsDir() {
		return fmt.Errorf("%s doesn't appear to be a directory", journalDir)
	}

	err = fm.recoverSeekPosition()
	return
}

func (fm *FileMonitor) cleanJournalPath() {
	// if the seekJournalPath is empty, write out the default
	if fm.seekJournalPath == "" {
		r := strings.NewReplacer(string(os.PathSeparator), "_", ".", "_")
		journal_name := r.Replace(fm.logger_ident)
		defaultPath := path.Join("/var/run/hekad/seekjournals", journal_name)
		fm.seekJournalPath = defaultPath
	}
	fm.seekJournalPath = path.Clean(fm.seekJournalPath)
}
