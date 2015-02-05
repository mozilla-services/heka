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
	ts "github.com/mozilla-services/heka/pipeline/testsupport"
	"github.com/rafrombrc/gomock/gomock"
	gs "github.com/rafrombrc/gospec/src/gospec"
	"io"
	"io/ioutil"
	"path/filepath"
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

func makeMockReader(data []byte) (d *MockDataReader) {
	d = new(MockDataReader)
	d.data = make([]byte, len(data))
	d.ptr = 0
	copy(d.data, data)
	return
}

func SplitterRunnerSpec(c gs.Context) {
	t := &ts.SimpleT{}
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	c.Specify("A SplitterRunner w/ HekaFramingSplitter", func() {
		splitter := &HekaFramingSplitter{}
		config := splitter.ConfigStruct().(*HekaFramingSplitterConfig)
		useMsgBytes := true
		srConfig := CommonSplitterConfig{
			UseMsgBytes: &useMsgBytes,
		}
		sr := NewSplitterRunner("HekaFramingSplitter", splitter, srConfig)

		err := splitter.Init(config)
		c.Assume(err, gs.IsNil)

		b, err := ioutil.ReadFile(filepath.Join(".", "testsupport", "multi.dat"))
		c.Assume(err, gs.IsNil)
		reader := makeMockReader(b)

		c.Specify("correctly handles data at EOF", func() {
			count := 0
			errCount := 0
			bytesRead := 0
			foundEOFCount := 0
			remainingDataLength := 0
			finalRecordLength := 0
			eofRecordLength := 0

			done := false
			for !done {
				n, record, err := sr.GetRecordFromStream(reader)
				if len(record) > 0 {
					count += 1
					finalRecordLength = len(record)
				}
				bytesRead += n
				if err != nil {
					if err == io.EOF {
						foundEOFCount = count
						eofRecordLength = len(record)

						rem := sr.GetRemainingData()
						remainingDataLength = len(rem)

						done = true
					} else {
						errCount++
						continue
					}

				}
			}

			c.Expect(done, gs.Equals, true)
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
				pack.Recycle()
			})

			for err == nil {
				err = sr.SplitStream(reader, nil)
			}
			c.Expect(err, gs.Equals, io.EOF)
		})
	})
}
