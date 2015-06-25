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
#   Mike Trinkala (trink@mozilla.com)
#   Mark Reid (mreid@mozilla.com)
#   Rob Miller (rmiller@mozilla.com)
#
# ***** END LICENSE BLOCK *****/

package pipeline

import (
	"bytes"
	"io"
	"io/ioutil"
	"path/filepath"

	"github.com/mozilla-services/heka/message"
	ts "github.com/mozilla-services/heka/pipeline/testsupport"
	"github.com/rafrombrc/gomock/gomock"
	gs "github.com/rafrombrc/gospec/src/gospec"
)

// Dummy reader that will return some data along with the EOF error.
type MockDataReader struct {
	data []byte
	ptr  int
}

func (d *MockDataReader) Read(p []byte) (n int, err error) {
	var start = d.ptr
	d.ptr += len(p)
	if d.ptr >= len(d.data) {
		d.ptr = len(d.data)
		copy(p, d.data[start:])
		return (d.ptr - start), io.EOF
	}
	copy(p, d.data[start:d.ptr])
	return (d.ptr - start), nil
}

func (d *MockDataReader) Append(p []byte) {
	newData := make([]byte, len(d.data)+len(p))
	copy(newData, d.data)
	copy(newData[len(d.data):], p)
	d.data = newData
}

func makeMockReader(data []byte) (d *MockDataReader) {
	d = new(MockDataReader)
	d.data = make([]byte, len(data))
	d.ptr = 0
	copy(d.data, data)
	return
}

func readRecordsFromStream(sr *sRunner, reader io.Reader, getRemaining bool) (count int,
	errCount int, bytesRead int, foundEOFCount int, remainingDataLength int,
	finalRecordLength int, eofRecordLength int) {
	done := false
	for !done {
		n, record, err := (*sr).GetRecordFromStream(reader)
		if len(record) > 0 {
			count += 1
			finalRecordLength = len(record)
		}
		bytesRead += n
		if err != nil {
			if err == io.EOF {
				foundEOFCount = count
				eofRecordLength = len(record)

				if getRemaining {
					rem := (*sr).GetRemainingData()
					remainingDataLength = len(rem)
				}
				done = true
			} else {
				errCount++
				continue
			}
		}
	}
	return
}

func SplitterRunnerSpec(c gs.Context) {
	t := &ts.SimpleT{}
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	srConfig := CommonSplitterConfig{}

	c.Specify("A SplitterRunner w/ HekaFramingSplitter", func() {
		splitter := &HekaFramingSplitter{}
		config := splitter.ConfigStruct().(*HekaFramingSplitterConfig)
		useMsgBytes := true
		srConfig.UseMsgBytes = &useMsgBytes
		sr := NewSplitterRunner("HekaFramingSplitter", splitter, srConfig)
		splitter.SetSplitterRunner(sr)

		err := splitter.Init(config)
		c.Assume(err, gs.IsNil)

		b, err := ioutil.ReadFile(filepath.Join(".", "testsupport", "multi.dat"))
		c.Assume(err, gs.IsNil)
		reader := makeMockReader(b)

		c.Specify("correctly handles data at EOF", func() {
			count, errCount, bytesRead, foundEOFCount,
				remainingDataLength, finalRecordLength,
				eofRecordLength := readRecordsFromStream(sr, reader, true)

			c.Expect(errCount, gs.Equals, 0)
			c.Expect(count, gs.Equals, 50)
			c.Expect(foundEOFCount, gs.Equals, 50)
			c.Expect(remainingDataLength, gs.Equals, 0)
			c.Expect(finalRecordLength, gs.Equals, 215)
			c.Expect(eofRecordLength, gs.Equals, 0)
			c.Expect(bytesRead, gs.Equals, len(b))
		})

		c.Specify("correctly splits & unframes a protobuf stream", func() {
			ir := NewMockInputRunner(ctrl)
			sr.SetInputRunner(ir)
			recycleChan := make(chan *PipelinePack, 1)
			pack := NewPipelinePack(recycleChan)
			recycleChan <- pack
			numRecs := 50
			ir.EXPECT().InChan().Times(numRecs).Return(recycleChan)
			delCall := ir.EXPECT().Deliver(pack).Times(numRecs)
			delCall.Do(func(pack *PipelinePack) {
				pack.Recycle(nil)
			})

			for err == nil {
				err = sr.SplitStream(reader, nil)
			}
			c.Expect(err, gs.Equals, io.EOF)
		})

		c.Specify("correctly handles appends after EOF", func() {
			half := len(b) / 2
			reader := makeMockReader(b[:half])
			totalBytesRead := 0

			count, errCount, bytesRead, foundEOFCount, _, finalRecordLength,
				eofRecordLength := readRecordsFromStream(sr, reader, false)
			totalBytesRead += bytesRead

			c.Expect(errCount, gs.Equals, 0)
			c.Expect(count, gs.Equals, 25)
			c.Expect(foundEOFCount, gs.Equals, 25)
			c.Expect(finalRecordLength, gs.Equals, 215)
			c.Expect(eofRecordLength, gs.Equals, 0)
			c.Expect(bytesRead <= half, gs.IsTrue)

			reader.Append(b[half:])

			count, errCount, bytesRead, foundEOFCount,
				remainingDataLength, finalRecordLength,
				eofRecordLength := readRecordsFromStream(sr, reader, true)
			totalBytesRead += bytesRead
			c.Expect(errCount, gs.Equals, 0)
			c.Expect(count, gs.Equals, 25)
			c.Expect(foundEOFCount, gs.Equals, 25)
			c.Expect(remainingDataLength, gs.Equals, 0)
			c.Expect(finalRecordLength, gs.Equals, 215)
			c.Expect(eofRecordLength, gs.Equals, 0)

			c.Expect(totalBytesRead, gs.Equals, len(b))
		})

		c.Specify("reuse on another stream without GetRemainingData", func() {
			// Test the case where we reuse the same SplitterRunner on
			// two different readers, and we do not call GetRemainingData before
			// using the second reader.
			half := len(b) / 2
			reader1 := makeMockReader(b[:half])

			count, errCount, bytesRead, foundEOFCount, _, finalRecordLength,
				eofRecordLength := readRecordsFromStream(sr, reader1, false)

			c.Expect(errCount, gs.Equals, 0)
			c.Expect(count, gs.Equals, 25)
			c.Expect(foundEOFCount, gs.Equals, 25)
			c.Expect(finalRecordLength, gs.Equals, 215)
			c.Expect(eofRecordLength, gs.Equals, 0)

			leftovers := half - bytesRead
			c.Expect(leftovers > 0, gs.IsTrue)

			reader2 := makeMockReader(b)

			// Don't call GetRemainingData before using sr on a new stream
			count, errCount, bytesRead, foundEOFCount, remainingDataLength, finalRecordLength,
				eofRecordLength := readRecordsFromStream(sr, reader2, true)

			c.Expect(errCount, gs.Equals, 0)
			c.Expect(count, gs.Equals, 50)
			c.Expect(foundEOFCount, gs.Equals, 50)
			c.Expect(remainingDataLength, gs.Equals, 0)
			c.Expect(finalRecordLength, gs.Equals, 215)
			c.Expect(eofRecordLength, gs.Equals, 0)
			// sr misreports the "remaining data" piece from reader1 as being
			// read from reader2
			c.Expect(bytesRead, gs.Equals, len(b)+leftovers)
		})

		c.Specify("reuse on another stream with reset", func() {
			// Test the case where we reuse the same SplitterRunner on
			// two different readers, but we call GetRemainingData before using
			// the second reader.
			half := len(b) / 2
			reader1 := makeMockReader(b[:half])

			count, errCount, bytesRead, foundEOFCount, _, finalRecordLength,
				eofRecordLength := readRecordsFromStream(sr, reader1, false)

			c.Expect(errCount, gs.Equals, 0)
			c.Expect(count, gs.Equals, 25)
			c.Expect(foundEOFCount, gs.Equals, 25)
			c.Expect(finalRecordLength, gs.Equals, 215)
			c.Expect(eofRecordLength, gs.Equals, 0)

			leftovers := half - bytesRead
			c.Expect(leftovers > 0, gs.IsTrue)

			reader2 := makeMockReader(b)

			// Call GetRemainingData before using sr on a new stream
			sr.GetRemainingData()
			count, errCount, bytesRead, foundEOFCount, remainingDataLength, finalRecordLength,
				eofRecordLength := readRecordsFromStream(sr, reader2, true)

			c.Expect(errCount, gs.Equals, 0)
			c.Expect(count, gs.Equals, 50)
			c.Expect(foundEOFCount, gs.Equals, 50)
			c.Expect(remainingDataLength, gs.Equals, 0)
			c.Expect(finalRecordLength, gs.Equals, 215)
			c.Expect(eofRecordLength, gs.Equals, 0)
			// Now we see the correct number of bytes being read.
			c.Expect(bytesRead, gs.Equals, len(b))
		})
	})

	c.Specify("A SplitterRunner w/ TokenSplitter", func() {
		splitter := &TokenSplitter{}
		config := splitter.ConfigStruct().(*TokenSplitterConfig)

		c.Specify("sets readPos to 0 when read returns ErrShortBuffer", func() {
			config.Delimiter = "\t"
			err := splitter.Init(config)
			c.Assume(err, gs.IsNil)

			sr := NewSplitterRunner("TokenSplitter", splitter, srConfig)

			b := make([]byte, message.MAX_RECORD_SIZE+1)
			reader := bytes.NewReader(b)

			var n int
			var record []byte
			for err == nil {
				n, record, err = sr.GetRecordFromStream(reader)
			}
			c.Expect(n, gs.Equals, int(message.MAX_RECORD_SIZE))
			c.Expect(len(record), gs.Equals, 0)
			c.Expect(err, gs.Equals, io.ErrShortBuffer)
			c.Expect(sr.readPos, gs.Equals, 0)
			c.Expect(sr.scanPos, gs.Equals, 0)
		})

		c.Specify("checks if splitter honors 'deliver_incomplete_final' setting", func() {

			config.Count = 4
			numRecs := 10
			err := splitter.Init(config)
			c.Assume(err, gs.IsNil)

			packSupply := make(chan *PipelinePack, 1)
			pack := NewPipelinePack(packSupply)
			packSupply <- pack
			ir := NewMockInputRunner(ctrl)
			// ir.EXPECT().InChan().Return(packSupply).Times(numRecs)
			// ir.EXPECT().Name().Return("foo").Times(numRecs)
			ir.EXPECT().InChan().Return(packSupply).AnyTimes()
			ir.EXPECT().Name().Return("foo").AnyTimes()

			incompleteFinal := true
			srConfig.IncompleteFinal = &incompleteFinal
			sr := NewSplitterRunner("TokenSplitter", splitter, srConfig)
			sr.ir = ir

			rExpected := []byte("test1\ntest12\ntest123\npartial\n")
			buf := bytes.Repeat(rExpected, numRecs)
			buf = buf[:len(buf)-1] // 40 lines separated by 39 newlines

			reader := bytes.NewReader(buf)
			mockDel := NewMockDeliverer(ctrl)
			delCall := mockDel.EXPECT().Deliver(gomock.Any()).AnyTimes()
			i := 0
			delCall.Do(func(pack *PipelinePack) {
				i++
				if i < numRecs {
					c.Expect(pack.Message.GetPayload(), gs.Equals, string(rExpected))
				} else {
					c.Expect(pack.Message.GetPayload(), gs.Equals,
						string(rExpected[:len(rExpected)-1]))
				}
				pack.Recycle(nil)
			})
			c.Specify("via SplitStream", func() {
				for err == nil {
					err = sr.SplitStream(reader, mockDel)
				}
				c.Expect(err, gs.Equals, io.EOF)
				c.Expect(i, gs.Equals, numRecs)
			})
			c.Specify("via SplitBytes", func() {
				seekPos, err := sr.SplitBytes(buf, mockDel)
				c.Assume(err, gs.IsNil)
				c.Expect(seekPos, gs.Equals, len(buf))
				c.Expect(i, gs.Equals, numRecs)
			})
		})

	})
}
