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
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/bbangert/toml"
	"github.com/gogo/protobuf/proto"
	"github.com/mozilla-services/heka/client"
	"github.com/mozilla-services/heka/message"
	ts "github.com/mozilla-services/heka/pipeline/testsupport"
	"github.com/rafrombrc/gomock/gomock"
	gs "github.com/rafrombrc/gospec/src/gospec"
)

type FooOutput struct{}

func (f *FooOutput) Init(config interface{}) error {
	return nil
}

func (f *FooOutput) Run(or OutputRunner, h PluginHelper) error {
	return nil
}

func QueueBufferSpec(c gs.Context) {
	tmpDir, tmpErr := ioutil.TempDir("", "queuebuffer-tests")

	defer func() {
		tmpErr = os.RemoveAll(tmpDir)
		c.Expect(tmpErr, gs.Equals, nil)
	}()

	t := &ts.SimpleT{}
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	c.Specify("QueueBuffer Internals", func() {
		pConfig := NewPipelineConfig(nil)

		err := pConfig.RegisterDefault("HekaFramingSplitter")
		c.Assume(err, gs.IsNil)
		RegisterPlugin("FooOutput", func() interface{} {
			return &FooOutput{}
		})

		outputToml := `[FooOutput]
        message_matcher = "TRUE"
        `

		var configFile ConfigFile
		_, err = toml.Decode(outputToml, &configFile)
		c.Assume(err, gs.IsNil)
		section, ok := configFile["FooOutput"]
		c.Assume(ok, gs.IsTrue)
		maker, err := NewPluginMaker("FooOutput", pConfig, section)
		c.Assume(err, gs.IsNil)

		orTmp, err := maker.MakeRunner("FooOutput")
		c.Assume(err, gs.IsNil)
		or := orTmp.(*foRunner)

		// h := NewMockPluginHelper(ctrl)
		// h.EXPECT().PipelineConfig().Return(pConfig)

		qConfig := &QueueBufferConfig{
			FullAction:        "block",
			CursorUpdateCount: 1,
			MaxFileSize:       66000,
		}
		feeder, reader, err := NewBufferSet(tmpDir, "test", qConfig, or, pConfig)
		// bufferedOutput, err := NewBufferedOutput(tmpDir, "test", or, h, uint64(0))
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
			reader.checkpointFilename = filepath.Join(tmpDir, "cp.txt")
			err := reader.writeCheckpoint("43 99999")
			c.Expect(err, gs.IsNil)
			c.Expect(fileExists(reader.checkpointFilename), gs.IsTrue)

			id, offset, err := readCheckpoint(reader.checkpointFilename)
			c.Expect(err, gs.IsNil)
			c.Expect(id, gs.Equals, uint(43))
			c.Expect(offset, gs.Equals, int64(99999))

			err = reader.writeCheckpoint("43 1")
			c.Expect(err, gs.IsNil)
			id, offset, err = readCheckpoint(reader.checkpointFilename)
			c.Expect(err, gs.IsNil)
			c.Expect(id, gs.Equals, uint(43))
			c.Expect(offset, gs.Equals, int64(1))
			reader.checkpointFile.Close()
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
			feeder.queue = tmpDir
			err := feeder.RollQueue()
			c.Expect(err, gs.IsNil)
			c.Expect(fileExists(getQueueFilename(feeder.queue,
				feeder.writeId)), gs.IsTrue)
			feeder.writeFile.WriteString("this is a test item")
			feeder.writeFile.Close()

			reader.checkpointFilename = filepath.Join(tmpDir, "cp.txt")
			reader.queue = tmpDir
			reader.writeCheckpoint(fmt.Sprintf("%d 10", feeder.writeId))
			reader.checkpointFile.Close()
			err = reader.initReadFile()
			c.Assume(err, gs.IsNil)
			buf := make([]byte, 4)
			n, err := reader.readFile.Read(buf)
			c.Expect(n, gs.Equals, 4)
			c.Expect(string(buf), gs.Equals, "test")
			feeder.writeFile.Close()
			reader.readFile.Close()
		})

		c.Specify("QueueRecord", func() {
			reader.checkpointFilename = filepath.Join(tmpDir, "cp.txt")
			reader.queue = tmpDir
			newpack := NewPipelinePack(nil)
			newpack.Message = msg
			payload := "Write me out to the network"
			newpack.Message.SetPayload(payload)
			encoder := client.NewProtobufEncoder(nil)
			protoBytes, err := encoder.EncodeMessage(newpack.Message)
			newpack.MsgBytes = protoBytes
			expectedLen := 115

			c.Specify("adds framing", func() {
				err = feeder.RollQueue()
				c.Expect(err, gs.IsNil)
				err = feeder.QueueRecord(newpack)
				fName := getQueueFilename(feeder.queue, feeder.writeId)
				c.Expect(fileExists(fName), gs.IsTrue)
				c.Expect(err, gs.IsNil)
				feeder.writeFile.Close()

				f, err := os.Open(fName)
				c.Expect(err, gs.IsNil)

				n, record, err := reader.sRunner.GetRecordFromStream(f)
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
				feeder.Config.MaxBufferSize = uint64(200)
				c.Expect(feeder.queueSize.Get(), gs.Equals, uint64(0))

				err = feeder.RollQueue()
				c.Expect(err, gs.IsNil)

				err = feeder.QueueRecord(newpack)
				c.Expect(err, gs.IsNil)
				c.Expect(feeder.queueSize.Get(), gs.Equals, uint64(expectedLen))
			})

			c.Specify("when queue has limit and is full", func() {
				feeder.Config.MaxBufferSize = uint64(50)
				feeder.Config.MaxFileSize = uint64(50)

				c.Expect(feeder.queueSize.Get(), gs.Equals, uint64(0))
				err = feeder.RollQueue()
				c.Expect(err, gs.IsNil)
				queueFiles, err := ioutil.ReadDir(feeder.queue)
				c.Expect(err, gs.IsNil)
				numFiles := len(queueFiles)

				err = feeder.QueueRecord(newpack)
				c.Expect(err, gs.Equals, QueueIsFull)
				c.Expect(feeder.queueSize.Get(), gs.Equals, uint64(0))

				// Bump the max queue size so it will accept a record.
				feeder.Config.MaxBufferSize = uint64(120)
				err = feeder.QueueRecord(newpack)
				c.Expect(err, gs.IsNil)

				// Queue should have rolled.
				queueFiles, err = ioutil.ReadDir(feeder.queue)
				c.Expect(err, gs.IsNil)
				c.Expect(len(queueFiles), gs.Equals, numFiles+1)

				// Try to queue one last time, it should fail.
				err = feeder.QueueRecord(newpack)
				c.Expect(err, gs.Equals, QueueIsFull)

				// Ensure queue didn't roll twice.
				queueFiles, err = ioutil.ReadDir(feeder.queue)
				c.Expect(err, gs.IsNil)
				c.Expect(len(queueFiles), gs.Equals, numFiles+1)
			})

			c.Specify("rolls when queue file hits max size", func() {
				feeder.Config.MaxFileSize = uint64(300)
				c.Assume(feeder.writeFileSize, gs.Equals, uint64(0))

				// First two shouldn't trigger roll.
				err = feeder.QueueRecord(newpack)
				c.Expect(err, gs.IsNil)
				err = feeder.QueueRecord(newpack)
				c.Expect(err, gs.IsNil)
				c.Expect(feeder.writeFileSize, gs.Equals, uint64(expectedLen*2))
				queueFiles, err := ioutil.ReadDir(feeder.queue)
				c.Expect(err, gs.IsNil)
				c.Expect(len(queueFiles), gs.Equals, 1)

				// Third one should.
				err = feeder.QueueRecord(newpack)
				c.Expect(err, gs.IsNil)
				c.Expect(feeder.writeFileSize, gs.Equals, uint64(expectedLen))
				queueFiles, err = ioutil.ReadDir(feeder.queue)
				c.Expect(err, gs.IsNil)
				c.Expect(len(queueFiles), gs.Equals, 2)
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
