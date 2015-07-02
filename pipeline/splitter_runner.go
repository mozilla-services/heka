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
	"errors"
	"fmt"
	"io"
	"time"

	"code.google.com/p/go-uuid/uuid"
	"github.com/mozilla-services/heka/message"
)

type WantsSplitterRunner interface {
	SetSplitterRunner(sr SplitterRunner)
}

type SplitterRunner interface {
	PluginRunner
	SetInputRunner(ir InputRunner)
	Splitter() Splitter
	SplitBytes(data []byte, del Deliverer) (int, error)
	SplitStream(r io.Reader, del Deliverer) error
	GetRemainingData() (record []byte)
	GetRecordFromStream(r io.Reader) (int, []byte, error)
	DeliverRecord(record []byte, del Deliverer)
	KeepTruncated() bool
	UseMsgBytes() bool
	IncompleteFinal() bool
	SetPackDecorator(decorator func(*PipelinePack))
	Done()
}

type sRunner struct {
	pRunnerBase
	splitter        Splitter
	buf             []byte
	readPos         int
	scanPos         int
	needData        bool
	keepTruncated   bool
	useMsgBytes     bool
	reachedEOF      bool
	incompleteFinal bool
	unframer        UnframingSplitter
	ir              InputRunner
	packDecorator   func(*PipelinePack)
}

func NewSplitterRunner(name string, splitter Splitter,
	config CommonSplitterConfig) *sRunner {

	bufSize := config.BufferSize
	if bufSize == 0 {
		bufSize = 8 * 1024
	}
	buf := make([]byte, bufSize)
	sr := &sRunner{
		pRunnerBase: pRunnerBase{
			name:   name,
			plugin: splitter.(Plugin),
		},
		splitter: splitter,
		buf:      buf,
		needData: true,
	}
	sr.name = name
	if config.KeepTruncated != nil {
		sr.keepTruncated = *config.KeepTruncated
	}
	if config.UseMsgBytes != nil {
		sr.useMsgBytes = *config.UseMsgBytes
	}
	if config.IncompleteFinal != nil {
		sr.incompleteFinal = *config.IncompleteFinal
	}
	// Cache our unframer so we don't need to do type coersion for every
	// message. Ignoring the ok is safe here, it just means sr.unframer might
	// be nil, which we test for later.
	sr.unframer, _ = splitter.(UnframingSplitter)
	if wantsSplitterRunner, ok := splitter.(WantsSplitterRunner); ok {
		wantsSplitterRunner.SetSplitterRunner(sr)
	}
	return sr
}

func (sr *sRunner) LogError(err error) {
	LogError.Printf("Splitter '%s' error: %s", sr.name, err)
}

func (sr *sRunner) LogMessage(msg string) {
	LogInfo.Printf("Splitter '%s': %s", sr.name, msg)
}

func (sr *sRunner) KeepTruncated() bool {
	return sr.keepTruncated
}

func (sr *sRunner) UseMsgBytes() bool {
	return sr.useMsgBytes
}

func (sr *sRunner) IncompleteFinal() bool {
	return sr.incompleteFinal
}

func (sr *sRunner) SetInputRunner(ir InputRunner) {
	sr.ir = ir
}

func (sr *sRunner) SetPackDecorator(decorator func(*PipelinePack)) {
	sr.packDecorator = decorator
}

func (sr *sRunner) Splitter() Splitter {
	return sr.splitter
}

func (sr *sRunner) Done() {
	pConfig := sr.h.PipelineConfig()
	pConfig.allSplittersLock.Lock()
	for i, otherSr := range pConfig.allSplitters {
		if sr == otherSr {
			pConfig.allSplitters = append(pConfig.allSplitters[:i], pConfig.allSplitters[i+1:]...)
			break
		}
	}
	pConfig.allSplittersLock.Unlock()
}

func (sr *sRunner) GetRemainingData() (record []byte) {
	if sr.readPos-sr.scanPos > 0 {
		record = sr.buf[sr.scanPos:sr.readPos]
	}
	sr.scanPos = 0
	sr.readPos = 0
	return record
}

func (sr *sRunner) setMinimumBufferSize(size int) {
	if cap(sr.buf) < size {
		newSlice := make([]byte, size)
		copy(newSlice, sr.buf)
		sr.buf = newSlice
	}
}

func (sr *sRunner) read(r io.Reader) (n int, err error) {
	bufCap := cap(sr.buf)
	if bufCap-sr.readPos <= bufCap/2 {
		if sr.scanPos == 0 { // Line won't fit in the current buffer.
			newSize := bufCap * 2
			if newSize > int(message.MAX_RECORD_SIZE) {
				if bufCap == int(message.MAX_RECORD_SIZE) {
					if sr.readPos == bufCap {
						sr.scanPos = 0
						sr.readPos = 0
						return bufCap, io.ErrShortBuffer
					} else {
						newSize = 0 // Don't allocate more, just read into what's left.
					}
				} else {
					newSize = int(message.MAX_RECORD_SIZE)
				}
			}
			if newSize > 0 {
				sr.setMinimumBufferSize(newSize)
			}
		} else {
			// Reclaim the space at the beginning of the buffer.
			copy(sr.buf, sr.buf[sr.scanPos:sr.readPos])
			sr.readPos, sr.scanPos = sr.readPos-sr.scanPos, 0
		}
	}
	n, err = r.Read(sr.buf[sr.readPos:])
	return n, err
}

func (sr *sRunner) GetRecordFromStream(r io.Reader) (bytesRead int, record []byte, err error) {
	if sr.needData && !sr.reachedEOF {
		bytesRead, err = sr.read(r)

		// We could still have one or more records at the end of the stream.
		// Hang on to the EOF error until all the records have been used up.
		if err == io.EOF {
			if bytesRead == 0 {
				// If we didn't read any bytes, we don't need to look for more
				// records, we can return the EOF.
				return bytesRead, record, err
			}
			// We did read some bytes, so clear the EOF for now
			sr.reachedEOF = true
			err = nil
		}

		if err == io.ErrShortBuffer && sr.keepTruncated {
			// Return truncated message.
			record = sr.buf
		}

		if err != nil {
			return bytesRead, record, err
		}
	}

	sr.readPos += bytesRead
	bytesRead, record = sr.splitter.FindRecord(sr.buf[sr.scanPos:sr.readPos])
	sr.scanPos += bytesRead
	if len(record) == 0 {
		// If the record is empty and we've reached EOF, we will not find any
		// more full records in the stream. There may still be some bytes left
		// over, which can be fetched with GetRemainingData(). Now is the time
		// to return the EOF error.
		if sr.reachedEOF {
			err = io.EOF
			// Reset reachedEOF so that if any new data is appended to the file,
			// we can continue reading where we left off. Note that if you want
			// to reuse this SplitterRunner on a different stream, you should
			// call GetRemainingData() to clear any remaining data out of the
			// buffer.
			sr.reachedEOF = false
		} else {
			// If we haven't yet reached EOF, then we need to read more data.
			sr.needData = true
		}
	} else {
		if sr.readPos == sr.scanPos {
			sr.readPos = 0
			sr.scanPos = 0
			sr.needData = true
		} else {
			sr.needData = false
		}
	}
	return bytesRead, record, err
}

func (sr *sRunner) DeliverRecord(record []byte, del Deliverer) {
	unframed := record
	pack := <-sr.ir.InChan()
	if sr.unframer != nil {
		unframed = sr.unframer.UnframeRecord(record, pack)
		if unframed == nil {
			pack.Recycle()
			return
		}
	}
	if sr.useMsgBytes {
		// Put the blob in the pack and let the decoder sort it out.
		messageLen := len(unframed)
		if messageLen > cap(pack.MsgBytes) {
			pack.MsgBytes = make([]byte, messageLen)
		}
		pack.MsgBytes = pack.MsgBytes[:messageLen]
		copy(pack.MsgBytes, unframed)
	} else {
		// Put the record data in the payload.
		pack.Message.SetUuid(uuid.NewRandom())
		pack.Message.SetTimestamp(time.Now().UnixNano())
		pack.Message.SetLogger(sr.ir.Name())
		pack.Message.SetPayload(string(unframed))
	}
	// Give the input one last chance to mutate the pack.
	if sr.packDecorator != nil {
		sr.packDecorator(pack)
		pack.TrustMsgBytes = false
	}
	if del == nil {
		sr.ir.Deliver(pack)
	} else {
		del.Deliver(pack)
	}
}

func (sr *sRunner) SplitBytes(data []byte, del Deliverer) (int, error) {
	var (
		n      int
		record []byte
	)
	seekPos := 0
	dataLen := len(data)
	for true {
		n, record = sr.Splitter().FindRecord(data[seekPos:])
		recordLen := uint32(len(record))
		if recordLen == 0 {
			// Checks if there is remaining unsplitted data
			remainingDataSize := uint32(len(data[seekPos+n:]))
			if sr.incompleteFinal && remainingDataSize > 0 {
				record = data[seekPos+n:]
				recordLen = remainingDataSize
				n += int(remainingDataSize)
			} else {
				if seekPos == 0 {
					return 0, errors.New("no records")
				}
				// Exit w/ no error.
				break
			}
		}
		seekPos += n
		if recordLen > message.MAX_RECORD_SIZE {
			if sr.keepTruncated {
				record = record[:message.MAX_RECORD_SIZE]
			} else {
				record = record[:0]
				recordLen = 0
			}
		}
		if recordLen > 0 {
			sr.DeliverRecord(record, del)
		}
		if seekPos >= dataLen {
			break
		}
	}
	return seekPos, nil
}

func (sr *sRunner) SplitStream(r io.Reader, del Deliverer) error {
	var (
		record []byte
		err    error
	)
	for true {
		_, record, err = sr.GetRecordFromStream(r)
		if err != nil {
			if err == io.ErrShortBuffer {
				sr.ir.LogError(fmt.Errorf("record exceeded MAX_RECORD_SIZE %d",
					message.MAX_RECORD_SIZE))
				err = nil
			}
		}
		if len(record) == 0 {
			if err == nil && sr.needData && !sr.reachedEOF {
				continue
			}
			if sr.incompleteFinal && err == io.EOF {
				record = sr.GetRemainingData()
				if len(record) > 0 {
					sr.DeliverRecord(record, del)
				}
			}
			break
		}
		sr.DeliverRecord(record, del)
	}
	return err
}
