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
	"code.google.com/p/go-uuid/uuid"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"strings"
	"syscall"
	"time"
)

// ConfigStruct for LogfileInput plugin.
type LogfileInputConfig struct {
	// Paths for all of the log files that this input should be reading.
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
	if err = lw.Monitor.Init(conf.LogFile, conf.DiscoverInterval,
		conf.StatInterval, conf.SeekJournal); err != nil {
		return err
	}

	lw.decoderNames = conf.Decoders
	return nil
}

func (fm *FileMonitor) setupJournalling() (err error) {
	var dirInfo os.FileInfo

	// if the seekJournalPath is empty, write out the default
	if fm.seekJournalPath == "" {
		defaultPath := path.Join("/var/run/hekad/seekjournals", fmt.Sprintf("%s.log", "DummyName"))
		fm.seekJournalPath = defaultPath
	}
	fm.seekJournalPath = path.Clean(fm.seekJournalPath)

	// Check that the directory to seekJournalPath actually exists
	journalDir := path.Dir(fm.seekJournalPath)

	if dirInfo, err = os.Stat(journalDir); err != nil {
		return err
	}

	if !dirInfo.IsDir() {
		return fmt.Errorf("%s doesn't appear to be a directory", journalDir)
	}

	if err = fm.recoverSeekPosition(); err != nil {
		return err
	}

	go fm.Watcher()

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

	if err = lw.Monitor.setupJournalling(); err != nil {
		ir.LogError(err)
		return err
	}

	for _, msg := range lw.Monitor.pendingMessages {
		lw.Monitor.LogMessage(msg)
	}
	lw.Monitor.pendingMessages = make([]string, 0)

	for _, msg := range lw.Monitor.pendingErrors {
		lw.Monitor.LogError(msg)
	}
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
		pack.Message.SetLogger(logline.Path)
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

	ir              InputRunner
	pendingMessages []string
	pendingErrors   []string
}

// Serialize to JSON
func (fm *FileMonitor) MarshalJSON() ([]byte, error) {
	// Note: We can't serialize the stat.pinfo in a cross platform way.
	// If you check the os.SameFile api, it only works on pinfo
	// objects created by os itself.

	btimes := make(map[string]int64)

	tmp := map[string]interface{}{
		"seek":        fm.seek,
		"birth_times": btimes,
	}

	for filename, _ := range fm.seek {
		info, err := os.Stat(filename)
		if err != nil {
			// Can't stat it, but meh.  Just don't serialize.

			fm.LogError(fmt.Sprintf("Can't get stat() info for [%s]\n", filename))
			continue
		}
		sys_info, _ := info.Sys().(*syscall.Stat_t)
		stat_t_attr := attributes(sys_info)

        // This may break under windows
		if _, ok := stat_t_attr["Ctimespec"]; ok == true {
			ctime := sys_info.Ctimespec.Nano()
			btime := sys_info.Birthtimespec.Nano()
		} else if _, ok := stat_t_attr["Ctim"]; ok == true {
			ctime := sys_info.Ctim.Nano()
			btime := sys_info.Btim.Nano()
		}

		// Assume that if ctime and btime are the same, then
		// the underlying filesystem doesn't really support
		// birthtime
		if ctime != btime {
			btimes[filename] = btime
		}
	}

	return json.Marshal(tmp)
}

func current_ctime(filename string) (result int64, err error) {
	info, err := os.Stat(filename)
	if err != nil {
		return 0, fmt.Errorf("Can't get stat() info for [%s]", filename)
	}
	sys_info := info.Sys().(*syscall.Stat_t)

	return sys_info.Ctimespec.Nano(), nil
}

func current_btime(filename string) (result int64, err error) {
	info, err := os.Stat(filename)
	if err != nil {
		return 0, fmt.Errorf("Can't get stat() info for [%s]", filename)
	}
	sys_info := info.Sys().(*syscall.Stat_t)

	return sys_info.Birthtimespec.Nano(), nil
}

func (fm *FileMonitor) UnmarshalJSON(data []byte) error {
	var dec = json.NewDecoder(bytes.NewReader(data))
	var m map[string]interface{}
	var cur_btime int64

	err := dec.Decode(&m)
	if err != nil {
		return fmt.Errorf("Caught error while decoding json blob: %s", err.Error())
	}

	var btime_map = m["birth_times"].(map[string]interface{})
	var seek_map = m["seek"].(map[string]interface{})
	for seek_filename, seek_pos := range seek_map {
		// Just do a linear scan to match seek positions to actual
		// logfiles.  This only happens at startup
		for discover_logfile, _ := range fm.discover {
			if discover_logfile == seek_filename {
				if btime_map[discover_logfile] == nil {
					continue
				}
				lst_btime := int64(btime_map[discover_logfile].(float64))
				if cur_btime, err = current_btime(discover_logfile); err != nil {
					return err
				}
				if cur_btime == lst_btime {
					fm.seek[seek_filename] = int64(seek_pos.(float64))
					fm.LogMessage(fmt.Sprintf("Setting seek position to: %s %d\n", seek_filename, fm.seek[seek_filename]))
				} else {
					msg := fmt.Sprintf("Skipping setting seek position as birthtime doesn't match. [%s] [%d] [%d]", discover_logfile, lst_btime, cur_btime)
					fm.LogMessage(msg)
				}
				break
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

	if ok = fm.updateJournal(bytes_read); ok == false {
		return false
	}

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

/*
 * Compute the attributes of a struct and return it as a map
 */
func attributes(m interface{}) map[string]reflect.Type {
	typ := reflect.TypeOf(m)
	// if a pointer to a struct is passed, get the type of the dereferenced object
	if typ.Kind() == reflect.Ptr {
		typ = typ.Elem()
	}

	// create an attribute data structure as a map of types keyed by a string.
	attrs := make(map[string]reflect.Type)
	// Only structs are supported so return an empty result if the passed object
	// isn't a struct
	if typ.Kind() != reflect.Struct {
		fmt.Printf("%v type can't have attributes inspected\n", typ.Kind())
		return attrs
	}

	// loop through the struct's fields and set the map
	for i := 0; i < typ.NumField(); i++ {
		p := typ.Field(i)
		if !p.Anonymous {
			attrs[p.Name] = p.Type
		}
	}

	return attrs
}
func (fm *FileMonitor) Init(file string, discoverInterval int,
	statInterval int, seekJournalPath string) (err error) {

	fm.NewLines = make(chan Logline)
	fm.stopChan = make(chan bool)
	fm.seek = make(map[string]int64)
	fm.fds = make(map[string]*os.File)
	fm.discover = make(map[string]bool)
	fm.discover[file] = true

	fm.pendingMessages = make([]string, 0)
	fm.pendingErrors = make([]string, 0)
	fm.seekJournalPath = seekJournalPath

	fm.discoverInterval = time.Millisecond * time.Duration(discoverInterval)
	fm.statInterval = time.Millisecond * time.Duration(statInterval)

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
