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
	"github.com/mozilla-services/heka/message"
	"io"
	"os"
	"path/filepath"
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
	DiscoverInterval int `toml:"discover_interval"`
	// Interval btn reads from open file handles, in milliseconds, default
	// 500.
	StatInterval int `toml:"stat_interval"`
	// Name of configured decoder instance.
	Decoder string
	// Specifies whether to use a seek journal to keep track of where we are
	// in a file to be able to resume parsing from the same location upon
	// restart. Defaults to true.
	UseSeekJournal bool `toml:"use_seek_journal"`
	// Name to use for the seek journal file, if one is used. Only refers to
	// the file name itself, not the full path; Heka will store all seek
	// journals in a `seekjournal` folder relative to the Heka base directory.
	// Defaults to a sanitized version of the `logger` value (which itself
	// defaults to the filesystem path of the input file). This value is
	// ignored if `use_seek_journal` is set to false.
	SeekJournalName string `toml:"seek_journal_name"`
	// Default value to use for the `logger` attribute on the generated Heka
	// messages. Note that this value might be modified by a decoder. Defaults
	// to the full filesystem path of the input file.
	Logger string
	// On failure to resume from last known position, LogfileInput
	// will resume reading from either the start of file or the end of
	// file. Defaults to false.
	ResumeFromStart bool `toml:"resume_from_start"`
	// Type of parser used to break the log file up into messages
	ParserType string `toml:"parser_type"`
	// Delimiter used to split the log stream into log messages
	Delimiter string
	// String indicating if the delimiter is at the start or end of the line,
	// only used for regexp delimiters
	DelimiterLocation string `toml:"delimiter_location"`
}

// Heka Input plugin that reads files from the filesystem, converts each line
// into a fully decoded Message object with the line contents as the payload,
// and passes the generated message on to the Router for delivery to any
// matching Filter or Output plugins.
type LogfileInput struct {
	// Encapsulates actual file finding / listening / reading mechanics.
	Monitor     *FileMonitor
	stopped     bool
	decoderName string
}

func getDefaultLogfileInputConfig() interface{} {
	return &LogfileInputConfig{
		DiscoverInterval: 5000,
		StatInterval:     500,
		UseSeekJournal:   true,
		ResumeFromStart:  true,
		ParserType:       "token",
	}
}

func (lw *LogfileInput) ConfigStruct() interface{} {
	return getDefaultLogfileInputConfig()
}

func (lw *LogfileInput) Init(config interface{}) (err error) {
	conf := config.(*LogfileInputConfig)
	lw.Monitor = new(FileMonitor)
	if err = lw.Monitor.Init(conf); err != nil {
		return err
	}
	lw.decoderName = conf.Decoder

	return nil
}

func (lw *LogfileInput) Run(ir InputRunner, h PluginHelper) (err error) {
	var (
		pack    *PipelinePack
		dRunner DecoderRunner
		ok      bool
	)
	lw.Monitor.ir = ir
	go lw.Monitor.Watcher()

	for _, msg := range lw.Monitor.pendingMessages {
		lw.Monitor.LogMessage(msg)
	}

	for _, msg := range lw.Monitor.pendingErrors {
		lw.Monitor.LogError(msg)
	}

	// Clear out all the errors
	lw.Monitor.pendingMessages = make([]string, 0)
	lw.Monitor.pendingErrors = make([]string, 0)

	if lw.decoderName != "" {
		if dRunner, ok = h.DecoderSet().ByName(lw.decoderName); !ok {
			return fmt.Errorf("Decoder not found: %s", lw.decoderName)
		}
	}

	for pack = range lw.Monitor.outChan {
		if dRunner == nil {
			ir.Inject(pack)
		} else {
			dRunner.InChan() <- pack
		}
	}
	return
}

func (lw *LogfileInput) Stop() {
	close(lw.Monitor.stopChan) // stops the monitor's watcher
	close(lw.Monitor.outChan)
}

// FileMonitor, manages a group of FileTailers
//
// Handles the actual mechanics of finding, watching, and reading from file
// system files.
type FileMonitor struct {
	// Channel onto which FileMonitor will place PipelinePack objects as the file
	// is being read.
	outChan  chan *PipelinePack
	stopChan chan bool
	seek     int64

	logfile         string
	seekJournalPath string
	logger_ident    string

	fd               *os.File
	discoverInterval time.Duration
	statInterval     time.Duration

	ir              InputRunner
	pendingMessages []string
	pendingErrors   []string

	last_logline       string
	last_logline_start int64
	resumeFromStart    bool

	parser        StreamParser
	parseFunction func(fm *FileMonitor, isRotated bool) (bytesRead int64, err error)
	hostname      string
}

// Serialize to JSON
func (fm *FileMonitor) MarshalJSON() ([]byte, error) {
	// Note: We can't serialize the stat.pinfo in a cross platform way.
	// If you check the os.SameFile api, it only works on pinfo
	// objects created by os itself.

	h := sha1.New()
	io.WriteString(h, fm.last_logline)
	tmp := map[string]interface{}{
		"seek":       fm.seek,
		"last_start": fm.last_logline_start,
		"last_len":   len(fm.last_logline),
		"last_hash":  fmt.Sprintf("%x", h.Sum(nil)),
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

	defer func() {
		if r := recover(); r != nil {
			fm.LogMessage("Error parsing the journal file")
		}
	}()

	err = dec.Decode(&m)
	if err != nil {
		return fmt.Errorf("Caught error while decoding json blob: %s", err.Error())
	}

	var seek_pos = int64(m["seek"].(float64))
	var last_start = int64(m["last_start"].(float64))
	var last_len = int64(m["last_len"].(float64))
	var last_hash = m["last_hash"].(string)

	var fd *os.File
	if fd, err = os.Open(fm.logfile); err != nil {
		return
	}
	defer fd.Close()

	// Try to get to our seek position.
	if _, err = fd.Seek(last_start, 0); err == nil {
		// We should be at the beginning of the last line read the last
		// time Heka ran.
		reader := bufio.NewReader(fd)
		buf := make([]byte, last_len)
		_, err := io.ReadAtLeast(reader, buf, int(last_len))
		if err == nil {
			h := sha1.New()
			h.Write(buf)
			tmp := fmt.Sprintf("%x", h.Sum(nil))
			if tmp == last_hash {
				fm.seek = seek_pos
				msg := fmt.Sprintf("Line matches, continuing from byte pos: %d", seek_pos)
				fm.LogMessage(msg)
				return nil
			}
		}
		fm.LogMessage("Line mismatch.")
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
func (fm *FileMonitor) OpenFile() (err error) {
	// Attempt to open the file
	fd, err := os.Open(fm.logfile)
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
				ok = fm.ReadLines()
				if !ok {
					break
				}
			}
		case <-discovery:
			if fm.fd == nil {
				// Check to see if the files exist now, start reading them
				// if we can, and watch them
				fm.OpenFile()
			}
		}
	}
	if fm.fd != nil {
		fm.fd.Close()
		fm.fd = nil
	}
}

func (fm *FileMonitor) updateJournal(bytes_read int64) (ok bool) {
	var seekJournal *os.File
	var file_err error

	if bytes_read == 0 || fm.seekJournalPath == "" {
		return true
	}

	if seekJournal, file_err = os.OpenFile(fm.seekJournalPath,
		os.O_CREATE|os.O_RDWR|os.O_TRUNC,
		0660); file_err != nil {
		fm.LogError(fmt.Sprintf("Error opening seek recovery log: %s", file_err.Error()))
		return false
	}
	defer seekJournal.Close()

	var filemon_bytes []byte
	filemon_bytes, _ = json.Marshal(fm)
	if _, file_err = seekJournal.Write(filemon_bytes); file_err != nil {
		fm.LogError(fmt.Sprintf("Error writing seek recovery log: %s", file_err.Error()))
		return false
	}

	return true
}

// Standard text log file parser
func payloadParser(fm *FileMonitor, isRotated bool) (bytesRead int64, err error) {
	var (
		n      int
		pack   *PipelinePack
		record []byte
	)
	for err == nil {
		n, record, err = fm.parser.Parse(fm.fd)
		if err != nil {
			if err == io.EOF && isRotated {
				record = fm.parser.GetRemainingData()
			} else if err == io.ErrShortBuffer {
				fm.ir.LogError(fmt.Errorf("record exceeded MAX_RECORD_SIZE %d", message.MAX_RECORD_SIZE))
				err = nil // non-fatal, keep going
			}
		}
		if len(record) > 0 {
			payload := string(record)
			pack = <-fm.ir.InChan()
			pack.Message.SetUuid(uuid.NewRandom())
			pack.Message.SetTimestamp(time.Now().UnixNano())
			pack.Message.SetType("logfile")
			pack.Message.SetSeverity(int32(0))
			pack.Message.SetEnvVersion("0.8")
			pack.Message.SetPid(0)
			pack.Message.SetHostname(fm.hostname)
			pack.Message.SetLogger(fm.logger_ident)
			pack.Message.SetPayload(payload)
			fm.outChan <- pack
			fm.last_logline_start = fm.seek + bytesRead
			fm.last_logline = payload
		}
		bytesRead += int64(n)
	}
	return
}

// Framed protobuf message parser
func messageProtoParser(fm *FileMonitor, isRotated bool) (bytesRead int64, err error) {
	var (
		n      int
		pack   *PipelinePack
		record []byte
	)
	for err == nil {
		n, record, err = fm.parser.Parse(fm.fd)
		if len(record) > 0 {
			pack = <-fm.ir.InChan()
			headerLen := int(record[1]) + 3 // recsep+len+header+unitsep
			messageLen := len(record) - headerLen
			// ignore authentication headers
			if messageLen > cap(pack.MsgBytes) {
				pack.MsgBytes = make([]byte, messageLen)
			}
			pack.MsgBytes = pack.MsgBytes[:messageLen]
			copy(pack.MsgBytes, record[headerLen:])
			fm.outChan <- pack
			fm.last_logline_start = fm.seek + bytesRead
			fm.last_logline = string(record)
		}
		bytesRead += int64(n)
	}
	return
}

// Reads all unread lines out of the monitored file, creates a PipelinePack object
// for each line, and puts it on the NewPack channel for processing.
// Returning false from ReadLines will kill the watcher
func (fm *FileMonitor) ReadLines() (ok bool) {
	ok = true
	var bytesRead int64

	defer func() {
		// Capture send on close chan as this is a shut-down
		if r := recover(); r != nil {
			rStr := fmt.Sprintf("%s", r)
			if strings.Contains(rStr, "send on closed channel") {
				ok = false
				// We're only partially through a file, write to the seekjournal.
				fm.seek += bytesRead
				fm.updateJournal(bytesRead)
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
	pinfo, err := os.Stat(fm.logfile)
	if err != nil || !os.SameFile(pinfo, finfo) {
		isRotated = true
		defer func() {
			if fm.fd != nil {
				fm.fd.Close()
			}
			fm.fd = nil
			fm.seek = 0
		}()
	}

	// Attempt to read lines from where we are.
	bytesRead, err = fm.parseFunction(fm, isRotated)

	if err != io.EOF {
		// Some unexpected error, reset everything
		// but don't kill the watcher
		fm.LogError(err.Error())
		fm.fd.Close()
		if fm.fd != nil {
			fm.fd = nil
		}
		fm.seek = 0
		return true
	}

	fm.seek += bytesRead
	return fm.updateJournal(bytesRead)
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
	if conf.Hostname == "" {
		conf.Hostname, err = os.Hostname()
		if err != nil {
			return
		}
	}
	fm.hostname = conf.Hostname

	fm.resumeFromStart = conf.ResumeFromStart
	if conf.ParserType == "" || conf.ParserType == "token" {
		tp := NewTokenParser()
		fm.parser = tp
		fm.parseFunction = payloadParser
		switch len(conf.Delimiter) {
		case 0: // use default
		case 1:
			tp.SetDelimiter(conf.Delimiter[0])
		default:
			return fmt.Errorf("invalid delimiter: %s", conf.Delimiter)
		}
	} else if conf.ParserType == "regexp" {
		rp := NewRegexpParser()
		fm.parser = rp
		fm.parseFunction = payloadParser
		if err = rp.SetDelimiter(conf.Delimiter); err != nil {
			return err
		}
		if err = rp.SetDelimiterLocation(conf.DelimiterLocation); err != nil {
			return err
		}
	} else if conf.ParserType == "message.proto" {
		mp := NewMessageProtoParser()
		fm.parser = mp
		fm.parseFunction = messageProtoParser
		if conf.Decoder == "" {
			return fmt.Errorf("The message.proto parser must have a decoder")
		}
	} else {
		return fmt.Errorf("unknown parser type: %s", conf.ParserType)
	}

	fm.outChan = make(chan *PipelinePack)
	fm.stopChan = make(chan bool)
	fm.seek = 0
	fm.fd = nil

	fm.logfile = file

	fm.pendingMessages = make([]string, 0)
	fm.pendingErrors = make([]string, 0)

	if logger != "" {
		fm.logger_ident = logger
	} else {
		fm.logger_ident = file
	}

	fm.discoverInterval = time.Millisecond * time.Duration(discoverInterval)
	fm.statInterval = time.Millisecond * time.Duration(statInterval)

	if conf.UseSeekJournal {
		seekJournalName := conf.SeekJournalName
		if seekJournalName == "" {
			seekJournalName = fm.logger_ident
		}
		if err = fm.setupJournalling(seekJournalName); err != nil {
			return
		}
	}
	return
}

func (fm *FileMonitor) recoverSeekPosition() (err error) {
	// No seekJournalPath means we're not tracking file location.
	if fm.seekJournalPath == "" {
		return
	}

	var seekJournal *os.File
	if seekJournal, err = os.Open(fm.seekJournalPath); err != nil {
		// The logfile doesn't exist, nothing special to do
		if os.IsNotExist(err) {
			// file doesn't exist, but that's ok, not a real error
			return nil
		} else {
			return
		}
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

// Initialize the seek journal file for keeping track of our place in a log
// file.
func (fm *FileMonitor) setupJournalling(journalName string) (err error) {
	// Check that the `seekjournals` directory exists, try to create it if
	// not.
	journalDir := GetHekaConfigDir("seekjournals")
	var dirInfo os.FileInfo
	if dirInfo, err = os.Stat(journalDir); err != nil {
		if os.IsNotExist(err) {
			if err = os.MkdirAll(journalDir, 0700); err != nil {
				fm.LogMessage(fmt.Sprintf("Error creating seek journal folder %s: %s",
					journalDir, err))
				return
			}
		} else {
			fm.LogMessage(fmt.Sprintf("Error accessing seek journal folder %s: %s",
				journalDir, err))
			return
		}
	} else if !dirInfo.IsDir() {
		return fmt.Errorf("%s doesn't appear to be a directory", journalDir)
	}

	// Generate the full file path and save it on the FileMonitor struct.
	r := strings.NewReplacer(string(os.PathSeparator), "_", ".", "_")
	journalName = r.Replace(journalName)
	fm.seekJournalPath = filepath.Join(journalDir, journalName)

	return fm.recoverSeekPosition()
}

type LogfileDirectoryManagerInput struct {
	conf    *LogfileInputConfig
	stopped chan bool
	logList map[string]bool
}

func (ldm *LogfileDirectoryManagerInput) Init(config interface{}) (err error) {
	ldm.conf = config.(*LogfileInputConfig)
	ldm.stopped = make(chan bool)
	ldm.logList = make(map[string]bool)
	fn := filepath.Base(ldm.conf.LogFile)
	if strings.ContainsAny(fn, "*?[]") {
		err = fmt.Errorf("Globs are not allowed in the file name: %s", fn)
	}
	if fn == "." || fn == string(os.PathSeparator) {
		err = fmt.Errorf("A logfile name must be specified.")
	}
	if ldm.conf.SeekJournalName != "" {
		err = fmt.Errorf("LogfileDirectoryManagerInput doesn't support `seek_journal_name` option.")
	}
	return
}

func (ldm *LogfileDirectoryManagerInput) ConfigStruct() interface{} {
	return getDefaultLogfileInputConfig()
}

// Expands the path glob and spins up a new LogfileInput if necessary
func (ldm *LogfileDirectoryManagerInput) scanPath(ir InputRunner, h PluginHelper) (err error) {
	if matches, err := filepath.Glob(ldm.conf.LogFile); err == nil {
		for _, fn := range matches {
			if _, ok := ldm.logList[fn]; !ok {
				ldm.logList[fn] = true
				ir.LogMessage(fmt.Sprintf("Starting LogfileInput for %s", fn))
				config := *ldm.conf
				config.LogFile = fn

				var pluginGlobals PluginGlobals
				pluginGlobals.Typ = "LogfileInput"
				pluginGlobals.Retries = RetryOptions{
					MaxDelay:   "30s",
					Delay:      "250ms",
					MaxRetries: -1,
				}
				wrapper := new(PluginWrapper)
				wrapper.name = fmt.Sprintf("%s-%s", ir.Name(), fn)
				wrapper.pluginCreator, _ = AvailablePlugins[pluginGlobals.Typ]
				plugin := wrapper.pluginCreator()
				wrapper.configCreator = func() interface{} { return config }
				if err = plugin.(Plugin).Init(&config); err != nil {
					ir.LogError(fmt.Errorf("Initialization failed for '%s': %s", wrapper.name, err))
					return err
				}
				lfir := NewInputRunner(wrapper.name, plugin.(Input), &pluginGlobals)
				err = h.PipelineConfig().AddInputRunner(lfir, wrapper)
			}
		}
	}
	return
}

// Heka Input plugin that scans the path glob looking for new directories.
// When a new directory is found with the specified log a LogfileInput plugin
// is started.
func (ldm *LogfileDirectoryManagerInput) Run(ir InputRunner, h PluginHelper) (err error) {
	var ok = true
	ticker := ir.Ticker()

	if err = ldm.scanPath(ir, h); err != nil {
		return
	}
	for ok {
		select {
		case _, ok = <-ldm.stopped:
		case _ = <-ticker:
			if err = ldm.scanPath(ir, h); err != nil {
				return
			}
		}
	}
	return
}

func (ldm *LogfileDirectoryManagerInput) Stop() {
	close(ldm.stopped)
}
