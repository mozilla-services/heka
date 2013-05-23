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
#
# ***** END LICENSE BLOCK *****/

package pipeline

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path"
	"strings"
	"syscall"
	"time"
)

// ConfigStruct for LogfileInput plugin.
type LogfileInputConfig struct {
	// Paths for all of the log files that this input should be reading.
	LogFiles []string

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
}

func (lw *LogfileInput) ConfigStruct() interface{} {
	return &LogfileInputConfig{
		DiscoverInterval: 5000,
		StatInterval:     500,
		SeekJournal:      "",
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
	if err = lw.Monitor.Init(conf.LogFiles, conf.DiscoverInterval,
		conf.StatInterval, conf.SeekJournal); err != nil {
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
		pack.Message.SetType("logfile")
		pack.Message.SetPayload(logline.Line)
		pack.Message.SetLogger(logline.Path)
		pack.Message.SetHostname(lw.hostname)
		for _, decoder := range decoders {
			if e = decoder.Decode(pack); e == nil {
				break
			}
		}
		if e == nil {
			ir.Inject(pack)
		} else {
			ir.LogError(fmt.Errorf("Couldn't parse log line: %s", logline))
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

	seekJournalPath string

	stopChan         chan bool
	seek             map[string]int64
	discover         map[string]bool
	fds              map[string]*os.File
	checkStat        <-chan time.Time
	discoverInterval time.Duration
	statInterval     time.Duration
}

// Serialize to JSON
func (fm *FileMonitor) MarshalJSON() ([]byte, error) {
	// Note: We can't serialize the stat.pinfo in a cross platform way.
	// If you check the os.SameFile api, it only works on pinfo
	// objects created by os itself.

	var btimes map[string]int64
	btimes = make(map[string]int64)

	var tmp = map[string]interface{}{
		"seek":        fm.seek,
		"birth_times": btimes,
	}

	for filename, _ := range fm.seek {
		info, err := os.Stat(filename)
		if err != nil {
			// Can't stat it, but meh.  Just don't serialize.
			// This should be a warning, not an error
			log.Printf("Can't get stat() info for [%s]\n", filename)
			continue
		}
		sys_info, _ := info.Sys().(*syscall.Stat_t)
		ctime := sys_info.Ctimespec.Nano()
		btime := sys_info.Birthtimespec.Nano()

		// Assume that if ctime and btime are the same, then
		// the underlying filesystem doesn't really support
		// birthtime
		if ctime != btime {
			btimes[filename] = btime
		}
	}

	return json.Marshal(tmp)
}

func (fm *FileMonitor) UnmarshalJSON(data []byte) error {
	var dec = json.NewDecoder(bytes.NewReader(data))
	var m map[string]interface{}

	err := dec.Decode(&m)
	if err != nil {
		return fmt.Errorf("Caught error while decoding json blob: %s", err.Error())
	}

	var seek_map = m["seek"].(map[string]interface{})
	for k, v := range seek_map {
		// Just do a linear scan to match seek positions to actual
		// logfiles.  This only happens at startup anyway
		for logfile, _ := range fm.discover {
			if logfile == k {
				fm.seek[k] = int64(v.(float64))
				log.Printf("Setting seek position to: %s %d\n", k, int64(v.(float64)))
				break
			}
		}
	}

	var btime_map = m["birth_times"].(map[string]interface{})
	for k, v := range btime_map {
		// Just do a linear scan to match seek positions to actual
		// logfiles.  This only happens at startup anyway
		for logfile, _ := range fm.discover {
			if logfile == k {
				last_btime := int64(v.(float64))

				info, err := os.Stat(logfile)
				if err != nil {
					return fmt.Errorf("Can't get stat() info for [%s]", logfile)
				}
				sys_info := info.Sys().(*syscall.Stat_t)

				btime := sys_info.Birthtimespec.Nano()

				log.Println("Stat() got birthtime: ", btime)
				log.Println("Loaded birthtime: ", last_btime)

				if btime != last_btime {
					fm.seek[logfile] = 0
				}
			}
		}
	}
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
	fm.fds[fileName] = fd

	// Seek as needed
	begin := 0
	offset := fm.seek[fileName]
	_, err = fd.Seek(offset, begin)
	if err != nil {
		// Unable to seek in, start at beginning
		fm.seek[fileName] = 0
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
			for fileName, _ := range fm.fds {
				ok = fm.ReadLines(fileName)
				if !ok {
					break
				}
			}
		case <-discovery:
			// Check to see if the files exist now, start reading them
			// if we can, and watch them
			for fileName, _ := range fm.discover {
				if fm.OpenFile(fileName) == nil {
					delete(fm.discover, fileName)
				}
			}
		}
	}

	for _, fd := range fm.fds {
		fd.Close()
	}
}

// Reads all unread lines out of the specified file, creates a LogLine object
// for each line, and puts it on the NewLine channel for processing.
func (fm *FileMonitor) ReadLines(fileName string) (ok bool) {

	ok = true
	defer func() {
		// Capture send on close chan as this is a shut-down
		if r := recover(); r != nil {
			rStr := fmt.Sprintf("%s", r)
			if strings.Contains(rStr, "send on closed channel") {
				ok = false
			} else {
				panic(rStr)
			}
		}
	}()

	fd, _ := fm.fds[fileName]

	// Determine if we're farther into the file than possible (truncate)
	finfo, err := fd.Stat()
	if err == nil {
		if finfo.Size() < fm.seek[fileName] {
			fd.Seek(0, 0)
			fm.seek[fileName] = 0
		}
	}

	// Attempt to read lines from where we are
	reader := bufio.NewReader(fd)
	readLine, err := reader.ReadString('\n')
	var bytes_read int64
	bytes_read = 0
	for err == nil {
		line := Logline{Path: fileName, Line: readLine}
		fm.NewLines <- line
		bytes_read += int64(len(readLine))
		readLine, err = reader.ReadString('\n')
	}
	bytes_read += int64(len(readLine))
	fm.seek[fileName] += bytes_read
	fm.updateJournal(bytes_read)

	// Check that we haven't been rotated, if we have, put this back on
	// discover
	pinfo, err := os.Stat(fileName)
	if err != nil || !os.SameFile(pinfo, finfo) {
		fd.Close()
		delete(fm.fds, fileName)
		delete(fm.seek, fileName)
		fm.discover[fileName] = true
	}
	return
}

func (fm *FileMonitor) updateJournal(bytes_read int64) {
	var seekJournal *os.File
	var file_err error

	if bytes_read == 0 || fm.seekJournalPath == "." {
		return
	}

	if seekJournal, file_err = os.OpenFile(fm.seekJournalPath,
		os.O_CREATE|os.O_RDWR|os.O_APPEND,
		0660); file_err != nil {
		log.Printf("Error opening seek recovery log for append: %s", file_err.Error())
		return
	}
	defer seekJournal.Close()
	seekJournal.Seek(0, os.SEEK_END)

	var filemon_bytes []byte
	filemon_bytes, _ = json.Marshal(fm)

	seekJournal.WriteString(string(filemon_bytes) + "\n")
}

func (fm *FileMonitor) Init(files []string, discoverInterval int,
	statInterval int, seekJournalPath string) (err error) {

	fm.NewLines = make(chan Logline)
	fm.stopChan = make(chan bool)
	fm.seek = make(map[string]int64)
	fm.fds = make(map[string]*os.File)
	fm.discover = make(map[string]bool)

	for _, fileName := range files {
		fm.discover[fileName] = true
	}

	fm.discoverInterval = time.Millisecond * time.Duration(discoverInterval)
	fm.statInterval = time.Millisecond * time.Duration(statInterval)
	fm.seekJournalPath = path.Clean(seekJournalPath)

	err = fm.recoverSeekPosition()
	if err != nil {
		return err
	}

	go fm.Watcher()
	return
}

func (fm *FileMonitor) recoverSeekPosition() error {

	// Check if the file exists first,

	if fm.seekJournalPath == "." {
		return nil
	}

	var f *os.File
	var err error

	if f, err = os.Open(fm.seekJournalPath); err != nil {
		// The logfile doesn't exist, nothing special to do
		if os.IsNotExist(err) {
			// file doesn't exist, but that's ok, not a real error
			return nil
		} else {
			return err
		}
	}
	defer f.Close()

	var seek_err error
	var seekJournal *os.File
	// First try to restore the line position
	if seekJournal, seek_err = os.OpenFile(fm.seekJournalPath,
		os.O_RDWR, 0660); seek_err != nil {
		return seek_err
	}
	defer seekJournal.Close()

	var scanner = bufio.NewScanner(seekJournal)
	var tmp string
	for scanner.Scan() {
		tmp = scanner.Text() // Println will add back the final '\n'
	}
	if len(tmp) > 0 {
		json.Unmarshal([]byte(tmp), &fm)
	}

	return nil
}
