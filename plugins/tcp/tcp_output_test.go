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

package tcp

import (
	"code.google.com/p/gomock/gomock"
	. "github.com/mozilla-services/heka/pipeline"
	pipeline_ts "github.com/mozilla-services/heka/pipeline/testsupport"
	plugins_ts "github.com/mozilla-services/heka/plugins/testsupport"
	gs "github.com/rafrombrc/gospec/src/gospec"
	"io/ioutil"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"
)

func TcpOutputSpec(c gs.Context) {
	t := new(pipeline_ts.SimpleT)
	ctrl := gomock.NewController(t)

	tmpDir, tmpErr := ioutil.TempDir("", "tcp-tests-")
	os.MkdirAll(tmpDir, 0777)
	defer func() {
		ctrl.Finish()
		tmpErr = os.RemoveAll(tmpDir)
		c.Expect(tmpErr, gs.Equals, nil)
	}()
	pConfig := NewPipelineConfig(nil)

	c.Specify("TcpOutput Internals", func() {
		tcpOutput := new(TcpOutput)
		tcpOutput.connection = pipeline_ts.NewMockConn(ctrl)

		msg := pipeline_ts.GetTestMessage()
		pack := NewPipelinePack(pConfig.InputRecycleChan())
		pack.Message = msg
		pack.Decoded = true

		c.Specify("SetName", func() {
			tcpOutput.SetName("this-is a test")
			c.Expect(tcpOutput.name, gs.Equals, "this_is_a_test")
		})

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
			tcpOutput.checkpointFilename = filepath.Join(tmpDir, "cp.txt")
			err := tcpOutput.writeCheckpoint(43, 99999)
			c.Expect(err, gs.IsNil)
			c.Expect(fileExists(tcpOutput.checkpointFilename), gs.IsTrue)

			id, offset, err := readCheckpoint(tcpOutput.checkpointFilename)
			c.Expect(err, gs.IsNil)
			c.Expect(id, gs.Equals, uint(43))
			c.Expect(offset, gs.Equals, int64(99999))

			err = tcpOutput.writeCheckpoint(43, 1)
			c.Expect(err, gs.IsNil)
			id, offset, err = readCheckpoint(tcpOutput.checkpointFilename)
			c.Expect(err, gs.IsNil)
			c.Expect(id, gs.Equals, uint(43))
			c.Expect(offset, gs.Equals, int64(1))
			tcpOutput.checkpointFile.Close()
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

		c.Specify("writeToNextFile", func() {
			tcpOutput.checkpointFilename = filepath.Join(tmpDir, "cp.txt")
			tcpOutput.queue = tmpDir
			err := tcpOutput.writeToNextFile()
			c.Expect(err, gs.IsNil)
			c.Expect(fileExists(getQueueFilename(tcpOutput.queue, tcpOutput.writeId)), gs.IsTrue)
			tcpOutput.writeFile.WriteString("this is a test item")
			tcpOutput.writeFile.Close()
			tcpOutput.writeCheckpoint(tcpOutput.writeId, 10)
			tcpOutput.checkpointFile.Close()
			err = tcpOutput.readFromNextFile()
			buf := make([]byte, 4)
			n, err := tcpOutput.readFile.Read(buf)
			c.Expect(n, gs.Equals, 4)
			c.Expect("test", gs.Equals, string(buf))
			tcpOutput.writeFile.Close()
			tcpOutput.readFile.Close()
		})
	})

	c.Specify("TcpOutput", func() {
		origBaseDir := Globals().BaseDir
		Globals().BaseDir = tmpDir
		defer func() {
			Globals().BaseDir = origBaseDir
		}()

		tcpOutput := new(TcpOutput)
		tcpOutput.SetName("test")
		config := tcpOutput.ConfigStruct().(*TcpOutputConfig)
		tcpOutput.Init(config)

		tickChan := make(chan time.Time)
		oth := plugins_ts.NewOutputTestHelper(ctrl)
		oth.MockOutputRunner.EXPECT().Ticker().Return(tickChan)

		var wg sync.WaitGroup
		inChan := make(chan *PipelinePack, 1)

		msg := pipeline_ts.GetTestMessage()
		pack := NewPipelinePack(pConfig.InputRecycleChan())
		pack.Message = msg
		pack.Decoded = true

		c.Specify("writes out to the network", func() {
			inChanCall := oth.MockOutputRunner.EXPECT().InChan().AnyTimes()
			inChanCall.Return(inChan)

			collectData := func(ch chan string) {
				ln, err := net.Listen("tcp", "localhost:9125")
				if err != nil {
					ch <- err.Error()
					return
				}
				ch <- "ready"
				conn, err := ln.Accept()
				if err != nil {
					ch <- err.Error()
					return
				}
				b := make([]byte, 1000)
				n, _ := conn.Read(b)
				ch <- string(b[0:n])
				conn.Close()
				ln.Close()
			}
			ch := make(chan string, 1) // don't block on put
			go collectData(ch)
			result := <-ch // wait for server

			err := tcpOutput.Init(config)
			c.Assume(err, gs.IsNil)

			outStr := "Write me out to the network"
			pack.Message.SetPayload(outStr)
			go func() {
				wg.Add(1)
				c.Expect(err, gs.IsNil)
				err = tcpOutput.Run(oth.MockOutputRunner, oth.MockHelper)
				wg.Done()
			}()

			inChan <- pack

			output := false
			for x := 0; x < 5; x++ {
				if fileExists(tcpOutput.checkpointFilename) {
					output = true
					break
				}
				time.Sleep(time.Duration(500) * time.Millisecond)
			}
			c.Expect(output, gs.Equals, true)

			close(inChan)
			wg.Wait() // wait for close to finish, prevents intermittent test failures

			matchBytes := make([]byte, 0, 1000)

			newpack := NewPipelinePack(nil)
			newpack.Message = msg
			newpack.Decoded = true
			newpack.Message.SetPayload(outStr)
			err = ProtobufEncodeMessage(newpack, &matchBytes)
			c.Expect(err, gs.IsNil)

			result = <-ch
			c.Expect(result, gs.Equals, string(matchBytes))
			id, offset, err := readCheckpoint(tcpOutput.checkpointFilename)
			c.Expect(err, gs.IsNil)
			c.Expect(id, gs.Equals, uint(1))
			c.Expect(offset, gs.Equals, int64(115))
		})

		c.Specify("far end not initially listening", func() {
			inChanCall := oth.MockOutputRunner.EXPECT().InChan().AnyTimes()
			inChanCall.Return(inChan)
			oth.MockOutputRunner.EXPECT().LogError(gomock.Any()).AnyTimes()

			err := tcpOutput.Init(config)
			c.Assume(err, gs.IsNil)

			outStr := "Write me out to the network"
			pack.Message.SetPayload(outStr)
			go func() {
				wg.Add(1)
				c.Expect(err, gs.IsNil)
				err = tcpOutput.Run(oth.MockOutputRunner, oth.MockHelper)
				wg.Done()
			}()
			inChan <- pack

			output := false
			for x := 0; x < 5; x++ {
				if fileExists(getQueueFilename(tcpOutput.queue, tcpOutput.writeId)) {
					output = true
					time.Sleep(time.Duration(1) * time.Second) // give the send time to fail
					break
				}
				time.Sleep(time.Duration(500) * time.Millisecond)
			}
			c.Expect(output, gs.Equals, true)

			collectData := func(ch chan string) {
				ln, err := net.Listen("tcp", "localhost:9125")
				if err != nil {
					ch <- err.Error()
					return
				}
				conn, err := ln.Accept()
				if err != nil {
					ch <- err.Error()
					return
				}
				b := make([]byte, 1000)
				n, _ := conn.Read(b)
				ch <- string(b[0:n])
				conn.Close()
				ln.Close()
			}
			ch := make(chan string, 1) // don't block on put
			go collectData(ch)
			result := <-ch // wait for server

			close(inChan)
			wg.Wait() // wait for close to finish, prevents intermittent test failures

			matchBytes := make([]byte, 0, 1000)

			newpack := NewPipelinePack(nil)
			newpack.Message = msg
			newpack.Decoded = true
			newpack.Message.SetPayload(outStr)
			err = ProtobufEncodeMessage(newpack, &matchBytes)
			c.Expect(err, gs.IsNil)

			c.Expect(result, gs.Equals, string(matchBytes))
			id, offset, err := readCheckpoint(tcpOutput.checkpointFilename)
			c.Expect(err, gs.IsNil)
			c.Expect(id, gs.Equals, uint(1))
			c.Expect(offset, gs.Equals, int64(115))
		})
	})
}
