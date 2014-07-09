/***** BEGIN LICENSE BLOCK *****
# This Source Code Form is subject to the terms of the Mozilla Public
# License, v. 2.0. If a copy of the MPL was not distributed with this file,
# You can obtain one at http://mozilla.org/MPL/2.0/.
#
# The Initial Developer of the Original Code is the Mozilla Foundation.
# Portions created by the Initial Developer are Copyright (C) 2012-2014
# the Initial Developer. All Rights Reserved.
#
# Contributor(s):
#   Rob Miller (rmiller@mozilla.com)
#   Mike Trinkala (trink@mozilla.com)
#
# ***** END LICENSE BLOCK *****/

package pipeline

import (
	"code.google.com/p/gomock/gomock"
	"code.google.com/p/goprotobuf/proto"
	"github.com/mozilla-services/heka/client"
	"github.com/mozilla-services/heka/message"
	ts "github.com/mozilla-services/heka/pipeline/testsupport"
	gs "github.com/rafrombrc/gospec/src/gospec"
	"io/ioutil"
	"os"
	"path/filepath"
)

func BufferedOutputSpec(c gs.Context) {
	tmpDir, tmpErr := ioutil.TempDir("", "bufferedout-tests")

	defer func() {
		tmpErr = os.RemoveAll(tmpDir)
		c.Expect(tmpErr, gs.Equals, nil)
	}()

	t := &ts.SimpleT{}
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	c.Specify("BufferedOutput Internals", func() {
		encoder := new(ProtobufEncoder)
		encoder.sample = false
		encoder.sampleDenominator = 1000
		or := NewMockOutputRunner(ctrl)

		bufferedOutput, err := NewBufferedOutput(tmpDir, "test", or)
		c.Expect(err, gs.IsNil)
		msg := ts.GetTestMessage()

		c.Specify("fileExists", func() {
			c.Expect(fileExists(tmpDir), gs.IsTrue)
			c.Expect(fileExists(filepath.Join(tmpDir, "test.log")), gs.IsFalse)
		})

		c.Specify("extractBufferId", func() {
			id, err := extractBufferId("555.log")
			c.Expect(err, gs.IsNil)
			c.Expect(id, gs.Equals, uint(555))
			id, err = extractBufferId("")
			c.Expect(err, gs.Not(gs.IsNil))
			id, err = extractBufferId("a.log")
			c.Expect(err, gs.Not(gs.IsNil))
		})

		c.Specify("findBufferId", func() {
			c.Expect(findBufferId(tmpDir, true), gs.Equals, uint(0))
			c.Expect(findBufferId(tmpDir, false), gs.Equals, uint(0))
			fd, err := os.Create(filepath.Join(tmpDir, "4.log"))
			c.Expect(err, gs.IsNil)
			fd.Close()
			fd, err = os.Create(filepath.Join(tmpDir, "5.log"))
			c.Expect(err, gs.IsNil)
			fd.Close()
			fd, err = os.Create(filepath.Join(tmpDir, "6a.log"))
			c.Expect(err, gs.IsNil)
			fd.Close()
			c.Expect(findBufferId(tmpDir, false), gs.Equals, uint(4))
			c.Expect(findBufferId(tmpDir, true), gs.Equals, uint(5))
		})

		c.Specify("writeCheckpoint", func() {
			bufferedOutput.checkpointFilename = filepath.Join(tmpDir, "cp.txt")
			err := bufferedOutput.writeCheckpoint(43, 99999)
			c.Expect(err, gs.IsNil)
			c.Expect(fileExists(bufferedOutput.checkpointFilename), gs.IsTrue)

			id, offset, err := readCheckpoint(bufferedOutput.checkpointFilename)
			c.Expect(err, gs.IsNil)
			c.Expect(id, gs.Equals, uint(43))
			c.Expect(offset, gs.Equals, int64(99999))

			err = bufferedOutput.writeCheckpoint(43, 1)
			c.Expect(err, gs.IsNil)
			id, offset, err = readCheckpoint(bufferedOutput.checkpointFilename)
			c.Expect(err, gs.IsNil)
			c.Expect(id, gs.Equals, uint(43))
			c.Expect(offset, gs.Equals, int64(1))
			bufferedOutput.checkpointFile.Close()
		})

		c.Specify("readCheckpoint", func() {
			cp := filepath.Join(tmpDir, "cp.txt")
			file, err := os.OpenFile(cp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
			c.Expect(err, gs.IsNil)
			id, offset, err := readCheckpoint(cp)
			c.Expect(err, gs.Not(gs.IsNil))

			file.WriteString("22")
			id, offset, err = readCheckpoint(cp)
			c.Expect(err.Error(), gs.Equals, "invalid checkpoint format")

			file.Seek(0, 0)
			file.WriteString("aa 22")
			id, offset, err = readCheckpoint(cp)
			c.Expect(err.Error(), gs.Equals, "invalid checkpoint id")

			file.Seek(0, 0)
			file.WriteString("43 aa")
			id, offset, err = readCheckpoint(cp)
			c.Expect(err.Error(), gs.Equals, "invalid checkpoint offset")

			file.Seek(0, 0)
			file.WriteString("43 22")
			file.Close()

			id, offset, err = readCheckpoint(cp)
			c.Expect(err, gs.IsNil)
			c.Expect(id, gs.Equals, uint(43))
			c.Expect(offset, gs.Equals, int64(22))
		})

		c.Specify("RollQueue", func() {
			bufferedOutput.checkpointFilename = filepath.Join(tmpDir, "cp.txt")
			bufferedOutput.queue = tmpDir
			err := bufferedOutput.RollQueue()
			c.Expect(err, gs.IsNil)
			c.Expect(fileExists(getQueueFilename(bufferedOutput.queue,
				bufferedOutput.writeId)), gs.IsTrue)
			bufferedOutput.writeFile.WriteString("this is a test item")
			bufferedOutput.writeFile.Close()
			bufferedOutput.writeCheckpoint(bufferedOutput.writeId, 10)
			bufferedOutput.checkpointFile.Close()
			err = bufferedOutput.readFromNextFile()
			buf := make([]byte, 4)
			n, err := bufferedOutput.readFile.Read(buf)
			c.Expect(n, gs.Equals, 4)
			c.Expect("test", gs.Equals, string(buf))
			bufferedOutput.writeFile.Close()
			bufferedOutput.readFile.Close()
		})

		c.Specify("QueueRecord", func() {
			bufferedOutput.checkpointFilename = filepath.Join(tmpDir, "cp.txt")
			bufferedOutput.queue = tmpDir
			newpack := NewPipelinePack(nil)
			newpack.Message = msg
			newpack.Decoded = true
			payload := "Write me out to the network"
			newpack.Message.SetPayload(payload)
			protoBytes, err := encoder.Encode(newpack)
			expectedLen := 115

			c.Specify("adds framing when necessary", func() {
				or.EXPECT().Encode(newpack).Return(protoBytes, err)
				or.EXPECT().UsesFraming().Return(false)
				err = bufferedOutput.RollQueue()
				c.Expect(err, gs.IsNil)
				err = bufferedOutput.QueueRecord(newpack)
				fName := getQueueFilename(bufferedOutput.queue, bufferedOutput.writeId)
				c.Expect(fileExists(fName), gs.IsTrue)
				c.Expect(err, gs.IsNil)
				bufferedOutput.writeFile.Close()

				f, err := os.Open(fName)
				c.Expect(err, gs.IsNil)

				n, record, err := bufferedOutput.parser.Parse(f)
				f.Close()
				c.Expect(n, gs.Equals, expectedLen)
				c.Expect(err, gs.IsNil)
				headerLen := int(record[1]) + message.HEADER_FRAMING_SIZE
				record = record[headerLen:]
				outMsg := new(message.Message)
				proto.Unmarshal(record, outMsg)
				c.Expect(outMsg.GetPayload(), gs.Equals, payload)
			})

			c.Specify("doesn't add framing if it's already there", func() {
				var framed []byte
				client.CreateHekaStream(protoBytes, &framed, nil)
				or.EXPECT().Encode(newpack).Return(framed, err)
				or.EXPECT().UsesFraming().Return(true)
				err = bufferedOutput.RollQueue()
				c.Expect(err, gs.IsNil)
				err = bufferedOutput.QueueRecord(newpack)
				fName := getQueueFilename(bufferedOutput.queue, bufferedOutput.writeId)
				c.Expect(fileExists(fName), gs.IsTrue)
				c.Expect(err, gs.IsNil)
				bufferedOutput.writeFile.Close()

				f, err := os.Open(fName)
				c.Expect(err, gs.IsNil)

				n, record, err := bufferedOutput.parser.Parse(f)
				f.Close()
				c.Expect(n, gs.Equals, expectedLen)
				c.Expect(err, gs.IsNil)
				headerLen := int(record[1]) + message.HEADER_FRAMING_SIZE
				record = record[headerLen:]
				outMsg := new(message.Message)
				proto.Unmarshal(record, outMsg)
				c.Expect(outMsg.GetPayload(), gs.Equals, payload)
			})

		})
	})
}
