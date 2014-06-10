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
#   Rob Miller (rmiller@mozilla.com)
#   Mike Trinkala (trink@mozilla.com)
#
# ***** END LICENSE BLOCK *****/

package pipeline

import (
	pipeline_ts "github.com/mozilla-services/heka/pipeline/testsupport"
	gs "github.com/rafrombrc/gospec/src/gospec"
	"io/ioutil"
	"os"
	"path/filepath"
)

func BufferedOutputSpec(c gs.Context) {
	tmpDir, tmpErr := ioutil.TempDir("", "bufferedout-tests-")
	os.MkdirAll(tmpDir, 0777)
	defer func() {
		tmpErr = os.RemoveAll(tmpDir)
		c.Expect(tmpErr, gs.Equals, nil)
	}()
	c.Specify("BufferedOutput Internals", func() {
		encoder := new(ProtobufEncoder)
		bufferedOutput, err := NewBufferedOutput(tmpDir, "test", encoder)
		c.Expect(err, gs.IsNil)
		msg := pipeline_ts.GetTestMessage()

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
			_, err := os.OpenFile(filepath.Join(tmpDir, "4.log"), os.O_CREATE, 0644)
			c.Expect(err, gs.IsNil)
			_, err = os.OpenFile(filepath.Join(tmpDir, "5.log"), os.O_CREATE, 0644)
			c.Expect(err, gs.IsNil)
			_, err = os.OpenFile(filepath.Join(tmpDir, "6a.log"), os.O_CREATE, 0644)
			c.Expect(err, gs.IsNil)
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
			c.Expect(fileExists(getQueueFilename(bufferedOutput.queue, bufferedOutput.writeId)), gs.IsTrue)
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
			c.Expect(fileExists(getQueueFilename(bufferedOutput.queue, bufferedOutput.writeId)), gs.IsTrue)
			newpack := NewPipelinePack(nil)
			newpack.Message = msg
			newpack.Decoded = true
			newpack.Message.SetPayload("Write me out to the network")
			qerr := bufferedOutput.QueueRecord(pack)
			c.Expect(qerr, gs.IsNil)
			bufferedOutput.writeFile.Close()
			bufferedOutput.writeCheckpoint(bufferedOutput.writeId, 10)
			id, offset, err := readCheckpoint(bufferedOutput.checkpointFilename)
			c.Expect(err, gs.IsNil)
			c.Expect(id, gs.Equals, uint(1))
			c.Expect(offset, gs.Equals, int64(115))
		})
	})

}
