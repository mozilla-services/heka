/***** BEGIN LICENSE BLOCK *****
# This Source Code Form is subject to the terms of the Mozilla Public
# License, v. 2.0. If a copy of the MPL was not distributed with this file,
# You can obtain one at http://mozilla.org/MPL/2.0/.
#
# The Initial Developer of the Original Code is the Mozilla Foundation.
# Portions created by the Initial Developer are Copyright (C) 2012-2015
# the Initial Developer. All Rights Reserved.
#
# Contributor(s):
#   Rob Miller (rmiller@mozilla.com)
#   Mike Trinkala (trink@mozilla.com)
#
# ***** END LICENSE BLOCK *****/

package pipeline

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/gogo/protobuf/proto"
	"github.com/mozilla-services/heka/client"
	"github.com/mozilla-services/heka/message"
)

var QueueIsFull = errors.New("Queue is full")
var QueueCorrupt = errors.New("Queue is corrupt")
var QueueNoRecord = errors.New("No record")
var QueueNeedData = errors.New("Need data")
var QueueCursorPast = errors.New("Queue has already advanced past provided cursor")
var QueueInvalidRecord = errors.New("Invalid record")
var ErrStopping = errors.New("Stopping")

var _wordre = regexp.MustCompile("\\W")

type BufferSize struct {
	size uint64
}

func (bs *BufferSize) Get() uint64 {
	return atomic.LoadUint64(&bs.size)
}

func (bs *BufferSize) Set(val uint64) {
	atomic.StoreUint64(&bs.size, val)
}

func (bs *BufferSize) Add(delta uint64) {
	atomic.AddUint64(&bs.size, delta)
}

type QueueBufferConfig struct {
	MaxFileSize       uint64 `toml:"max_file_size"`
	MaxBufferSize     uint64 `toml:"max_buffer_size"`
	FullAction        string `toml:"full_action"`
	CursorUpdateCount uint   `toml:"cursor_update_count"`
}

func NewBufferSet(queueDir, queueName string, config *QueueBufferConfig,
	runner *foRunner, pConfig *PipelineConfig) (*BufferFeeder, *BufferReader, error) {

	globals := pConfig.Globals
	queueName = _wordre.ReplaceAllString(queueName, "_")
	queue := globals.PrependBaseDir(filepath.Join(queueDir, queueName))
	if !fileExists(queue) {
		err := os.MkdirAll(queue, 0766)
		if err != nil {
			return nil, nil, fmt.Errorf("can't make queue directory: %s", err)
		}
	}

	queueSize := &BufferSize{
		size: getQueueBufferSize(queue),
	}

	if config.CursorUpdateCount == 0 {
		config.CursorUpdateCount = 1
	}

	if config.MaxFileSize < uint64(message.MAX_RECORD_SIZE) {
		err := fmt.Errorf("`max_file_size` must be greater than maximum record size of %d",
			message.MAX_RECORD_SIZE)
		return nil, nil, err
	}

	bf, err := NewBufferFeeder(queue, config, queueSize)
	if err != nil {
		return nil, nil, fmt.Errorf("can't create BufferFeeder: %s", err)
	}

	br, err := NewBufferReader(queue, config, queueSize, runner, pConfig)
	if err != nil {
		return nil, nil, fmt.Errorf("can't create BufferReader: %s", err)
	}

	return bf, br, nil
}

type BufferFeeder struct {
	writeFile     *os.File
	writeFileSize uint64
	writeId       uint
	queue         string
	queueSize     *BufferSize
	Config        *QueueBufferConfig
}

func NewBufferFeeder(queue string, config *QueueBufferConfig, queueSize *BufferSize) (
	*BufferFeeder, error) {

	if config.MaxFileSize == 0 {
		config.MaxFileSize = 512 * 1024 * 1024 // 512 MiB
	}
	bf := &BufferFeeder{
		queue:     queue,
		queueSize: queueSize,
		Config:    config,
	}

	var err error
	if !fileExists(bf.queue) {
		if err = os.MkdirAll(bf.queue, 0766); err != nil {
			return nil, fmt.Errorf("can't make queue directory: %s", err)
		}
	}
	bf.writeId = findBufferId(bf.queue, true)
	filename := getQueueFilename(bf.queue, bf.writeId)
	if bf.writeId > 0 || fileExists(filename) {
		bf.writeId++
	}
	bf.writeFile, err = os.OpenFile(getQueueFilename(bf.queue, bf.writeId),
		os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0644)
	if err != nil {
		return nil, fmt.Errorf("can't open write file: %s", err)
	}
	writeFileInfo, err := bf.writeFile.Stat()
	if err != nil {
		return nil, fmt.Errorf("can't stat write file: %s", err)
	}
	bf.writeFileSize = uint64(writeFileInfo.Size())
	return bf, nil
}

func (bf *BufferFeeder) RollQueue() (err error) {
	if bf.writeFile != nil {
		bf.writeFile.Close()
		bf.writeFile = nil
	}
	bf.writeId++
	bf.writeFile, err = os.OpenFile(getQueueFilename(bf.queue, bf.writeId),
		os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0644)
	bf.writeFileSize = 0
	return err
}

// QueueRecord adds a new record to the end of the current queue buffer. Note
// that QueueRecord is *not* thread safe, it should only ever be called by one
// goroutine at a time.
func (bf *BufferFeeder) QueueRecord(pack *PipelinePack) error {
	maxQueueSize := bf.Config.MaxBufferSize
	if maxQueueSize > 0 && (bf.queueSize.Get()+uint64(len(pack.MsgBytes)) > maxQueueSize) {
		return QueueIsFull
	}
	maxQueueFileSize := bf.Config.MaxFileSize
	if bf.writeFileSize+uint64(len(pack.MsgBytes)) > maxQueueFileSize {
		if err := bf.RollQueue(); err != nil {
			return fmt.Errorf("queue file rotation error: %s", err)
		}
	}

	var outBytes []byte
	err := client.CreateHekaStream(pack.MsgBytes, &outBytes, nil)
	if err != nil {
		return fmt.Errorf("message framing error: %s", err)
	}

	n, err := bf.writeFile.Write(outBytes)
	if err != nil {
		if n > 0 {
			// If we wrote some data but there was an error, that data is
			// suspect so we need to seek back to before the bogus write
			// happened.
			ret, e := bf.writeFile.Seek(int64(-n), 2)
			if e != nil {
				return QueueCorrupt
			}
			if e = bf.writeFile.Truncate(ret); e != nil {
				return QueueCorrupt
			}
		}
		return fmt.Errorf("can't write to queue: %s", err)
	}
	bf.queueSize.Add(uint64(n))
	bf.writeFileSize += uint64(n)
	return nil
}

type BufferReader struct {
	readOffset         int64
	cursorOffset       int64
	config             *QueueBufferConfig
	runner             *foRunner
	sRunner            SplitterRunner
	splitter           *HekaFramingSplitter
	readFile           *os.File
	readId             uint
	cursorId           uint
	cursorCount        uint
	checkpointFilename string
	checkpointFile     *os.File
	queue              string
	queueSize          *BufferSize
}

type BufferSender interface {
	SendRecord(pack *PipelinePack) error
}

func NewBufferReader(queue string, config *QueueBufferConfig, queueSize *BufferSize,
	runner *foRunner, pConfig *PipelineConfig) (*BufferReader, error) {

	br := &BufferReader{
		queue:     queue,
		config:    config,
		queueSize: queueSize,
		runner:    runner,
	}

	pConfig.makersLock.RLock()
	splitterMakers := pConfig.makers["Splitter"]
	maker, ok := splitterMakers["HekaFramingSplitter"]
	if !ok {
		pConfig.makersLock.RUnlock()
		return nil, errors.New("no registered `HekaFramingSplitter`.")
	}
	splitterName := fmt.Sprintf("%s-buffer-splitter", runner.Name())
	sRunner, err := maker.MakeRunner(splitterName)
	pConfig.makersLock.RUnlock()
	if err != nil {
		return nil, fmt.Errorf("can't make SplitterRunner: %s", err.Error())
	}
	br.sRunner = sRunner.(SplitterRunner)

	br.checkpointFilename = filepath.Join(queue, "checkpoint.txt")

	if err = br.initReadFile(); err != nil {
		return nil, fmt.Errorf("can't access read location: %s", err)
	}

	br.cursorId, br.cursorOffset = br.readId, br.readOffset
	return br, nil
}

func (br *BufferReader) initReadFile() error {
	var err error
	if fileExists(br.checkpointFilename) {
		br.readId, br.readOffset, err = readCheckpoint(br.checkpointFilename)
		if err != nil {
			return fmt.Errorf("readCheckpoint %s", err)
		}
	} else {
		br.readOffset = 0
		br.readId = findBufferId(br.queue, false)
	}

	filename := getQueueFilename(br.queue, br.readId)
	if br.readId == 0 && br.readOffset == 0 && !fileExists(filename) {
		// New queue, no content, not an error.
		return nil
	}
	if !fileExists(filename) {
		br.readOffset = 0
		br.readFile, br.readId, err = br.getFileFromId(br.readId)
		if err != nil {
			br.readFile = nil
			br.readId = 0
		}
		return err
	}
	if br.readFile, err = os.Open(filename); err != nil {
		return err
	}
	if _, err = br.readFile.Seek(br.readOffset, 0); err != nil {
		br.readFile.Close()
		br.readFile = nil
	}
	return err
}

func (br *BufferReader) getFileFromId(id uint) (file *os.File, foundId uint,
	err error) {

	filename := getQueueFilename(br.queue, id)
	if fileExists(filename) {
		if file, err = os.Open(filename); err == nil {
			foundId = id
		}
		return file, foundId, err
	}

	// If we got this far there's no file matching our id, try to find the
	// next one above.
	ids := sortedBufferIds(br.queue)
	if len(ids) == 0 || ids[len(ids)-1] < id {
		// No log file for us, not an error.
		return nil, 0, nil
	}

	// Increment until we find an id greater than what was requested.
	for _, val := range ids {
		if val > id {
			foundId = val
			break
		}
	}
	filename = getQueueFilename(br.queue, foundId)
	file, err = os.Open(filename)
	return file, foundId, err
}

func (br *BufferReader) updateCursor(queueCursor string) error {
	id, offset, err := parseQueueCursor([]byte(queueCursor))
	if err != nil {
		return fmt.Errorf("can't parse queue cursor '%s': %s", queueCursor, err)
	}
	if id < br.cursorId {
		// TODO: Handle id wrapping?
		return QueueCursorPast
	}
	if id == br.cursorId {
		// Same file.
		if offset < br.cursorOffset {
			return QueueCursorPast
		}
		if offset == br.cursorOffset {
			// Same offset, no-op.
			return nil
		}
		br.cursorOffset = offset
		if br.cursorCount < br.config.CursorUpdateCount {
			br.cursorCount++
		} else {
			if err = br.writeCheckpoint(queueCursor); err != nil {
				return fmt.Errorf("can't write checkpoint file: %s", err)
			}
			br.cursorCount = 0
		}
		return nil
	}

	// If we got this far we've updated to a new index file, need to delete
	// any we've advanced past and decrement our queue size.
	oldId := br.cursorId
	br.cursorId = id
	br.cursorOffset = offset
	if err = br.writeCheckpoint(queueCursor); err != nil {
		return fmt.Errorf("can't write checkpoint file: %s", err)
	}
	br.cursorCount = 0
	for delId := oldId; delId < id; delId++ {
		filename := getQueueFilename(br.queue, delId)
		if !fileExists(filename) {
			// We don't care if the file is already gone.
			continue
		}
		fileInfo, err := os.Stat(filename)
		if err != nil {
			return fmt.Errorf("can't stat queue file %s: %s", filename, err)
		}
		if err = os.Remove(filename); err != nil {
			return fmt.Errorf("can't remove queue file %s: %s", filename, err)
		}
		br.queueSize.Add(^uint64(fileInfo.Size() - 1)) // Subtracts file size.
	}
	return nil
}

func (br *BufferReader) writeCheckpoint(queueCursor string) error {
	var err error
	if br.checkpointFile == nil {
		if br.checkpointFile, err = os.OpenFile(br.checkpointFilename,
			os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644); err != nil {
			return err
		}
	}
	br.checkpointFile.Seek(0, 0)
	n, err := br.checkpointFile.WriteString(queueCursor)
	if err != nil {
		return err
	}
	return br.checkpointFile.Truncate(int64(n))
}

func (br *BufferReader) runTimerEvent(tickerPlugin TickerPlugin) error {
	err := tickerPlugin.TimerEvent()
	if err != nil {
		br.runner.LogError(fmt.Errorf("running TimerEvent: %s", err.Error()))
		if _, ok := err.(PluginExitError); !ok {
			err = nil // Only return the error if it's fatal.
		}
	}
	return err
}

func (br *BufferReader) NewStreamOutput(sender MessageProcessor, packSupply chan *PipelinePack,
	tickerPlugin TickerPlugin, tickChan <-chan time.Time, stopChan chan bool) error {

	if tickChan != nil && tickerPlugin == nil {
		return errors.New("Must provide TickerPlugin if tickChan is not nil.")
	}

	defer func() {
		err := br.writeCheckpoint(fmt.Sprintf("%d %d", br.cursorId, br.cursorOffset))
		if err != nil {
			br.runner.LogError(fmt.Errorf("can't write buffer checkpoint: %s", err))
		}
		if br.checkpointFile != nil {
			br.checkpointFile.Close()
			br.checkpointFile = nil
		}
		if br.readFile != nil {
			br.readFile.Close()
			br.readFile = nil
		}
	}()

	rh, _ := NewRetryHelper(RetryOptions{
		MaxDelay:   "1s",
		Delay:      "10ms",
		MaxRetries: -1,
	})

	var (
		pack        *PipelinePack
		err         error
		resetNeeded bool
	)

	for {
		if err = br.initReadFile(); err != nil {
			return fmt.Errorf("can't initialize read file", err)
		}
		if br.readFile != nil {
			if resetNeeded {
				rh.Reset()
				resetNeeded = false
			}
			break
		}
		// No data yet.
		resetNeeded = true
		rh.Wait()
	}

	for {
		// We might need a new pack, but we want to reuse packs that haven't
		// actually been populated.
		if pack == nil {
			select {
			case <-stopChan:
				return nil
			case <-tickChan:
				if e := br.runTimerEvent(tickerPlugin); e != nil {
					return e
				}
			case pack = <-packSupply:
			}
		} else {
			select {
			case <-stopChan:
				return nil
			case <-tickChan:
				if e := br.runTimerEvent(tickerPlugin); e != nil {
					return e
				}
			default:
			}
		}
		if err = br.NextRecord(pack); err != nil {
			if err == QueueNeedData {
				continue
			}
			if err == QueueNoRecord {
				resetNeeded = true
				rh.Wait()
				continue
			}
			return fmt.Errorf("can't get record: %s", err)
		}

		if resetNeeded {
			rh.Reset()
			resetNeeded = false
		}

	sendLoop:
		for {
			err = sender.ProcessMessage(pack)
			if err != nil {
				switch err.(type) {
				case PluginExitError:
					atomic.AddInt64(&br.runner.dropMessageCount, 1)
					pack.recycle()
					return err
				case RetryMessageError:
					br.runner.LogError(fmt.Errorf("can't send record: %s", err))
					// Falls through to a retry wait below.
				default:
					atomic.AddInt64(&br.runner.dropMessageCount, 1)
					pack.recycle()
					break sendLoop
				}
			} else {
				atomic.AddInt64(&br.runner.processMessageCount, 1)
				pack.recycle()
				break sendLoop
			}
			select {
			case <-stopChan:
				atomic.AddInt64(&br.runner.dropMessageCount, 1)
				pack.recycle()
				return nil
			case <-tickChan:
				if e := br.runTimerEvent(tickerPlugin); e != nil {
					atomic.AddInt64(&br.runner.dropMessageCount, 1)
					pack.recycle()
					return e
				}
			default:
				resetNeeded = true
				rh.Wait()
			}
		}

		if resetNeeded {
			rh.Reset()
			resetNeeded = false
		}
		pack = nil // Signals that we need a new pack.
	}

}

func (br *BufferReader) StreamOutput(sender BufferSender,
	packSupply chan *PipelinePack, stopChan chan bool) error {

	defer func() {
		err := br.writeCheckpoint(fmt.Sprintf("%d %d", br.cursorId, br.cursorOffset))
		if err != nil {
			br.runner.LogError(fmt.Errorf("can't write buffer checkpoint: %s", err))
		}
		if br.checkpointFile != nil {
			br.checkpointFile.Close()
			br.checkpointFile = nil
		}
		if br.readFile != nil {
			br.readFile.Close()
			br.readFile = nil
		}
	}()

	rh, _ := NewRetryHelper(RetryOptions{
		MaxDelay:   "2s",
		Delay:      "10ms",
		MaxRetries: -1,
	})

	var (
		pack        *PipelinePack
		err         error
		resetNeeded bool
	)

	for {
		if err := br.initReadFile(); err != nil {
			return fmt.Errorf("can't initialize read file", err)
		}
		if br.readFile != nil {
			if resetNeeded {
				rh.Reset()
				resetNeeded = false
			}
			break
		}
		// No data yet.
		resetNeeded = true
		rh.Wait()
	}

	for {
		// We might need a new pack, but we want to reuse packs that haven't
		// actually been populated.
		if pack == nil {
			select {
			case <-stopChan:
				return nil
			case pack = <-packSupply:
			}
		} else {
			select {
			case <-stopChan:
				return nil
			default:
			}
		}
		if err = br.NextRecord(pack); err != nil {
			if err == QueueNeedData {
				continue
			}
			if err == QueueNoRecord {
				resetNeeded = true
				rh.Wait()
				continue
			}
			return fmt.Errorf("can't get record: %s", err)
		}

		if resetNeeded {
			rh.Reset()
			resetNeeded = false
		}
		for {
			if err = sender.SendRecord(pack); err == nil {
				if resetNeeded {
					rh.Reset()
					resetNeeded = false
				}
				break
			}
			select {
			case <-stopChan:
				pack.recycle()
				return nil
			default:
				resetNeeded = true
				rh.Wait()
			}
		}
		pack = nil // Signals that we need a new pack.
	}
}

func (br *BufferReader) NextRecord(pack *PipelinePack) error {
	if br.readFile == nil {
		err := br.initReadFile()
		if err != nil {
			return fmt.Errorf("can't open read file: %s", err)
		}
		if br.readFile == nil {
			return QueueNoRecord
		}
	}

	n, record, err := br.sRunner.GetRecordFromStream(br.readFile)
	if err != nil {
		if err == io.EOF {
			// Look to see if there's a newer file, advance to it if so.
			nextReadFile, nextFileId, err := br.getFileFromId(br.readId + 1)
			if err != nil {
				return fmt.Errorf("can't lookup subsequent file: %s", err)
			} else if nextReadFile == nil {
				// No next file, current file might still grow.
				return QueueNoRecord
			}
			// Newer file exists, bump the id and file and recurse.
			oldReadFile := br.readFile
			br.readFile = nextReadFile
			br.readId = nextFileId
			br.readOffset = 0
			if err = oldReadFile.Close(); err != nil {
				return fmt.Errorf("can't close readfile: %s", err)
			}
			return br.NextRecord(pack)
		} else {
			return fmt.Errorf("can't extract record: %s", err)
		}
	}

	br.readOffset += int64(n)
	recordLen := len(record)
	if recordLen == 0 {
		return QueueNeedData
	}
	if recordLen < 1 {
		return QueueInvalidRecord
	}
	headerLen := int(record[1]) + message.HEADER_FRAMING_SIZE
	if recordLen < headerLen {
		return QueueInvalidRecord
	}
	msgLen := len(record) - headerLen
	if cap(pack.MsgBytes) < msgLen {
		pack.MsgBytes = make([]byte, msgLen)
	} else {
		pack.MsgBytes = pack.MsgBytes[:msgLen]
	}
	copy(pack.MsgBytes, record[headerLen:])
	pack.TrustMsgBytes = true
	err = proto.Unmarshal(pack.MsgBytes, pack.Message)
	if err != nil {
		return fmt.Errorf("can't unmarshal record: %s", err)
	}
	pack.QueueCursor = fmt.Sprintf("%d %d", br.readId, br.readOffset)
	return nil
}

func parseQueueCursor(queueCursor []byte) (id uint, offset int64, err error) {
	idx := bytes.IndexByte(queueCursor, ' ')
	if idx == -1 {
		err = fmt.Errorf("invalid checkpoint format")
		return 0, 0, err
	}

	var un uint64
	if un, err = strconv.ParseUint(string(queueCursor[:idx]), 10, 32); err != nil {
		err = fmt.Errorf("invalid checkpoint id")
		return 0, 0, err
	}
	id = uint(un)

	if offset, err = strconv.ParseInt(string(queueCursor[idx+1:]), 10, 64); err != nil {
		err = fmt.Errorf("invalid checkpoint offset")
		return 0, 0, err
	}

	return id, offset, nil
}

func readCheckpoint(filename string) (id uint, offset int64, err error) {
	file, err := os.Open(filename)
	if err != nil {
		return 0, 0, err
	}
	defer file.Close()

	b := make([]byte, 64)
	n, err := file.Read(b)
	if err != nil {
		return 0, 0, err
	}
	return parseQueueCursor(b[:n])
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	if err == nil {
		return true
	}
	return false
}

func getQueueFilename(queue string, id uint) string {
	return filepath.Join(queue, fmt.Sprintf("%d.log", id))
}

func extractBufferId(filename string) (id uint, err error) {
	name := filepath.Base(filename)
	if len(name) < 5 {
		err = fmt.Errorf("invalid filename (too short)")
		return
	}
	i, err := strconv.Atoi(name[:len(name)-4])
	id = uint(i)
	return
}

func findBufferId(dir string, newest bool) uint {
	var current uint
	var first = true
	if matches, err := filepath.Glob(filepath.Join(dir, "*.log")); err == nil {
		for _, fn := range matches {
			id, err := extractBufferId(fn)
			if err != nil {
				continue
			}
			if first {
				current = id
				first = false
			} else {
				if newest {
					if id > current {
						current = id
					}
				} else {
					if id < current {
						current = id
					}
				}
			}
		}
	}
	return current
}

type uintSlice []uint

func (u uintSlice) Len() int {
	return len(u)
}

func (u uintSlice) Less(i, j int) bool {
	return u[i] < u[j]
}

func (u uintSlice) Swap(i, j int) {
	u[j], u[i] = u[i], u[j]
}

func sortedBufferIds(dir string) []uint {
	matches, err := filepath.Glob(filepath.Join(dir, "*.log"))
	if err != nil {
		return nil
	}
	ids := make([]uint, 0, len(matches))
	for _, fn := range matches {
		id, err := extractBufferId(fn)
		if err != nil {
			continue
		}
		ids = append(ids, id)
	}
	sortable := uintSlice(ids)
	sort.Sort(sortable)
	return []uint(sortable)
}

func getQueueBufferSize(dir string) (size uint64) {
	if matches, err := filepath.Glob(filepath.Join(dir, "*.log")); err == nil {
		for _, fn := range matches {
			file_info, err := os.Stat(fn)
			if err != nil {
				break
			}
			size += uint64(file_info.Size())
		}
	}
	return
}
