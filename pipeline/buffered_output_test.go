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
	"code.google.com/p/gogoprotobuf/proto"
	"github.com/mozilla-services/heka/client"
	"github.com/mozilla-services/heka/message"
	ts "github.com/mozilla-services/heka/pipeline/testsupport"
	"github.com/rafrombrc/gomock/gomock"
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
		pConfig := NewPipelineConfig(nil)
		or := NewMockOutputRunner(ctrl)
		h := NewMockPluginHelper(ctrl)
		h.EXPECT().PipelineConfig().Return(pConfig)
		or.EXPECT().Name().Return("FooOutput")

		err := pConfig.RegisterDefault("HekaFramingSplitter")
		c.Assume(err, gs.IsNil)
		bufferedOutput, err := NewBufferedOutput(tmpDir, "test", or, h, uint64(0))
		c.Assume(err, gs.IsNil)
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

				n, record, err := bufferedOutput.sRunner.GetRecordFromStream(f)
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

				n, record, err := bufferedOutput.sRunner.GetRecordFromStream(f)
				f.Close()
				c.Expect(n, gs.Equals, expectedLen)
				c.Expect(err, gs.IsNil)
				headerLen := int(record[1]) + message.HEADER_FRAMING_SIZE
				record = record[headerLen:]
				outMsg := new(message.Message)
				proto.Unmarshal(record, outMsg)
				c.Expect(outMsg.GetPayload(), gs.Equals, payload)
			})

			c.Specify("when queue has limit", func() {
				or.EXPECT().Encode(newpack).Return(protoBytes, err)
				or.EXPECT().UsesFraming().Return(false)

				bufferedOutput.maxQueueSize = uint64(200)
				c.Expect(bufferedOutput.queueSize, gs.Equals, uint64(0))

				err = bufferedOutput.RollQueue()
				c.Expect(err, gs.IsNil)

				err = bufferedOutput.QueueRecord(newpack)
				c.Expect(err, gs.IsNil)
				c.Expect(bufferedOutput.queueSize, gs.Equals, uint64(115))
			})

			c.Specify("when queue has limit and is full", func() {
				or.EXPECT().Encode(newpack).Return(protoBytes, err).Times(4)
				bufferedOutput.maxQueueSize = uint64(50)

				c.Expect(bufferedOutput.queueSize, gs.Equals, uint64(0))
				err = bufferedOutput.RollQueue()
				c.Expect(err, gs.IsNil)
				queueFiles, err := ioutil.ReadDir(bufferedOutput.queue)
				c.Expect(err, gs.IsNil)
				numFiles := len(queueFiles)

				err = bufferedOutput.QueueRecord(newpack)
				c.Expect(err, gs.Equals, QueueIsFull)
				c.Expect(bufferedOutput.queueSize, gs.Equals, uint64(0))

				// Queue should have rolled again.
				queueFiles, err = ioutil.ReadDir(bufferedOutput.queue)
				c.Expect(err, gs.IsNil)
				c.Expect(len(queueFiles), gs.Equals, numFiles+1)

				// Try again.
				err = bufferedOutput.QueueRecord(newpack)
				c.Expect(err, gs.Equals, QueueIsFull)
				c.Expect(bufferedOutput.queueSize, gs.Equals, uint64(0))

				// Ensure queue didn't roll twice.
				queueFiles, err = ioutil.ReadDir(bufferedOutput.queue)
				c.Expect(err, gs.IsNil)
				c.Expect(len(queueFiles), gs.Equals, numFiles+1)

				or.EXPECT().UsesFraming().Return(false)
				// Bump the max queue size so it will accept a record.
				bufferedOutput.maxQueueSize = uint64(120)
				err = bufferedOutput.QueueRecord(newpack)
				c.Expect(err, gs.IsNil)

				// Try to queue one last time, it should fail and trigger
				// another queue roll.
				err = bufferedOutput.QueueRecord(newpack)
				c.Expect(err, gs.Equals, QueueIsFull)
				queueFiles, err = ioutil.ReadDir(bufferedOutput.queue)
				c.Expect(err, gs.IsNil)
				c.Expect(len(queueFiles), gs.Equals, numFiles+2)
			})

		})

		c.Specify("getQueueBufferSize", func() {
			c.Expect(getQueueBufferSize(tmpDir), gs.Equals, uint64(0))

			fd, _ := os.Create(filepath.Join(tmpDir, "4.log"))
			fd.WriteString("0123456789")
			fd.Close()

			fd, _ = os.Create(filepath.Join(tmpDir, "5.log"))
			fd.WriteString("0123456789")
			fd.Close()

			// Only size of *.log files should be taken in calculations.
			fd, _ = os.Create(filepath.Join(tmpDir, "random_file"))
			fd.WriteString("0123456789")
			fd.Close()

			c.Expect(getQueueBufferSize(tmpDir), gs.Equals, uint64(20))
		})
	})
}
