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
	"github.com/mozilla-services/heka/client"
	"github.com/mozilla-services/heka/message"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"sync/atomic"
	"time"
)

type BufferedOutput struct {
	sentMessageCount   int64
	readOffset         int64
	sRunner            SplitterRunner
	or                 OutputRunner
	writeFile          *os.File
	writeId            uint
	readFile           *os.File
	readId             uint
	checkpointFilename string
	checkpointFile     *os.File
	queue              string
	name               string
	outBytes           []byte
	maxQueueSize       uint64
	queueSize          uint64
}

type BufferedOutputSender interface {
	SendRecord(record []byte) (err error)
}

var QueueIsFull = errors.New("Queue is full")

func NewBufferedOutput(queueDir, queueName string, or OutputRunner, h PluginHelper,
	maxQueueSize uint64) (*BufferedOutput, error) {

	b := &BufferedOutput{
		or:           or,
		maxQueueSize: maxQueueSize,
	}
	pConfig := h.PipelineConfig()

	pConfig.makersLock.RLock()
	splitterMakers := pConfig.makers["Splitter"]
	maker, ok := splitterMakers["HekaFramingSplitter"]
	if !ok {
		pConfig.makersLock.RUnlock()
		return nil, errors.New("no registered `HekaFramingSplitter`.")
	}
	splitterName := fmt.Sprintf("%s-buffer-splitter", or.Name())
	runner, err := maker.MakeRunner(splitterName)
	pConfig.makersLock.RUnlock()
	if err != nil {
		return nil, fmt.Errorf("can't make SplitterRunner: %s", err.Error())
	}
	b.sRunner = runner.(SplitterRunner)

	globals := pConfig.Globals
	b.queue = globals.PrependBaseDir(filepath.Join(queueDir, queueName))
	b.checkpointFilename = filepath.Join(b.queue, "checkpoint.txt")
	b.outBytes = make([]byte, 0, 1000) // encoding will reallocate the buffer as necessary

	if !fileExists(b.queue) {
		if err = os.MkdirAll(b.queue, 0766); err != nil {
			return nil, fmt.Errorf("can't make queue directory: %s", err.Error())
		}
	}
	b.writeId = findBufferId(b.queue, true)
	b.queueSize = getQueueBufferSize(b.queue)
	return b, nil
}

func (b *BufferedOutput) QueueBytes(msgBytes []byte) (err error) {
	var msgSize int

	if b.maxQueueSize > 0 && (b.queueSize+uint64(len(msgBytes))) > b.maxQueueSize {
		err = QueueIsFull
		return
	}

	// If framing isn't already in place then we need to add it.
	if b.or.UsesFraming() {
		b.outBytes = msgBytes
	} else {
		if err = client.CreateHekaStream(msgBytes, &b.outBytes, nil); err != nil {
			return
		}
	}

	if msgSize, err = b.writeFile.Write(b.outBytes); err != nil {
		return fmt.Errorf("writing to %s: %s", getQueueFilename(b.queue, b.writeId), err)
	}
	atomic.AddUint64(&b.queueSize, uint64(msgSize))
	return nil
}

func (b *BufferedOutput) QueueRecord(pack *PipelinePack) (err error) {
	var msgBytes []byte

	if msgBytes, err = b.or.Encode(pack); msgBytes == nil || err != nil {
		return
	}
	return b.QueueBytes(msgBytes)
}

func (b *BufferedOutput) writeCheckpoint(id uint, offset int64) (err error) {
	if b.checkpointFile == nil {
		if b.checkpointFile, err = os.OpenFile(b.checkpointFilename,
			os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644); err != nil {
			return
		}
	}
	b.checkpointFile.Seek(0, 0)
	n, err := b.checkpointFile.WriteString(fmt.Sprintf("%d %d", id, offset))
	if err != nil {
		return
	}
	err = b.checkpointFile.Truncate(int64(n))
	return
}

func (b *BufferedOutput) RollQueue() (err error) {
	if b.writeFile != nil {
		b.writeFile.Close()
		b.writeFile = nil
	}
	b.writeId++
	b.writeFile, err = os.OpenFile(getQueueFilename(b.queue, b.writeId),
		os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0644)
	return err
}

func (b *BufferedOutput) readFromNextFile() (err error) {
	if b.readFile != nil {
		b.readFile.Close()
		b.readFile = nil
	}

	b.readOffset = 0
	if fileExists(b.checkpointFilename) {
		if b.readId, b.readOffset, err = readCheckpoint(b.checkpointFilename); err != nil {
			return fmt.Errorf("readCheckpoint %s", err)
		}
	} else {
		b.readId = findBufferId(b.queue, false)
	}
	if b.readFile, err = os.Open(getQueueFilename(b.queue, b.readId)); err != nil {
		return
	}
	_, err = b.readFile.Seek(b.readOffset, 0)
	return
}

func (b *BufferedOutput) Start(sender BufferedOutputSender, outputError,
	outputExit chan error, stopChan chan bool) {

	if err := b.RollQueue(); err != nil {
		outputExit <- err
		return
	}

	go b.streamOutput(sender, outputError, outputExit, stopChan)
}

func (b *BufferedOutput) streamOutput(sender BufferedOutputSender, outputError,
	outputExit chan error, stopChan chan bool) {

	var (
		err    error
		n      int
		record []byte
	)

	defer func() {
		if b.checkpointFile != nil {
			b.checkpointFile.Close()
			b.checkpointFile = nil
		}
		if b.readFile != nil {
			b.readFile.Close()
			b.readFile = nil
		}
		if b.writeFile != nil {
			b.writeFile.Close()
			b.writeFile = nil
		}
	}()

	if err = b.readFromNextFile(); err != nil {
		outputExit <- err
		return
	}

	rh, _ := NewRetryHelper(RetryOptions{
		MaxDelay:   "10s",
		Delay:      "1s",
		MaxRetries: -1,
	})

	for true {
		select {
		case <-stopChan:
			outputExit <- nil
			return
		default: // carry on
		}
		n, record, err = b.sRunner.GetRecordFromStream(b.readFile)
		if err != nil {
			if err == io.EOF {
				nextReadId := b.readId + 1
				filename := getQueueFilename(b.queue, nextReadId)

				if fileExists(filename) {
					readFileInfo, err := b.readFile.Stat()
					b.readFile.Close()
					b.readFile = nil

					if err != nil {
						break
					}

					if err = os.Remove(getQueueFilename(b.queue, b.readId)); err != nil {
						break
					} else {
						atomic.AddUint64(&b.queueSize, ^uint64(readFileInfo.Size()-1))
					}

					if err = b.writeCheckpoint(nextReadId, 0); err != nil {
						break
					}

					if err = b.readFromNextFile(); err != nil {
						break
					}
				} else {
					time.Sleep(time.Duration(500) * time.Millisecond)
				}
			} else {
				break
			}
		} else {
			if len(record) > 0 {
				// Remove the framing if we put it there.
				if !b.or.UsesFraming() {
					headerLen := int(record[1]) + message.HEADER_FRAMING_SIZE
					record = record[headerLen:]
				}
				rh.Reset()
				for true {
					err = sender.SendRecord(record)
					if err == nil {
						atomic.AddInt64(&b.sentMessageCount, 1)
						break
					}
					select {
					case <-stopChan:
						outputExit <- nil
						return
					default:
						outputError <- err
						rh.Wait() // this will delay Heka shutdown up to MaxDelay
					}
				}
			} else {
				runtime.Gosched()
			}
		}
		if n > 0 {
			b.readOffset += int64(n) // offset can advance without finding a valid record
			if err = b.writeCheckpoint(b.readId, b.readOffset); err != nil {
				break
			}
		}
	}

	outputExit <- err
	return
}

func (b *BufferedOutput) ReportMsg(msg *message.Message) error {

	message.NewInt64Field(msg, "SentMessageCount", atomic.LoadInt64(&b.sentMessageCount), "count")
	return nil
}

func readCheckpoint(filename string) (id uint, offset int64, err error) {
	file, err := os.Open(filename)
	if err != nil {
		return
	}
	defer file.Close()

	b := make([]byte, 64)
	n, err := file.Read(b)
	if err != nil {
		return
	}
	idx := bytes.IndexByte(b, ' ')
	if idx == -1 {
		err = fmt.Errorf("invalid checkpoint format")
		return
	}

	var un uint64
	if un, err = strconv.ParseUint(string(b[:idx]), 10, 32); err != nil {
		err = fmt.Errorf("invalid checkpoint id")
		return
	}
	id = uint(un)

	if offset, err = strconv.ParseInt(string(b[idx+1:n]), 10, 64); err != nil {
		err = fmt.Errorf("invalid checkpoint offset")
		return
	}

	return
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
