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
	"bytes"
	"code.google.com/p/gomock/gomock"
	"code.google.com/p/goprotobuf/proto"
	"encoding/json"
	"fmt"
	ts "github.com/mozilla-services/heka/testsupport"
	gs "github.com/rafrombrc/gospec/src/gospec"
	"io/ioutil"
	"net"
	"os"
	"sync"
	"time"
)

func OutputsSpec(c gs.Context) {
	t := new(ts.SimpleT)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	c.Specify("A FileWriter", func() {
		fileOutput := new(FileOutput)

		tmpFileName := fmt.Sprintf("fileoutput-test-%d", time.Now().UnixNano())
		tmpFilePath := fmt.Sprint(os.TempDir(), string(os.PathSeparator),
			tmpFileName)
		config := fileOutput.ConfigStruct().(*FileOutputConfig)
		config.Path = tmpFilePath

		msg := getTestMessage()
		pack := getTestPipelinePack()
		pack.Message = msg
		pack.Decoded = true

		toString := func(outData interface{}) string {
			return string(*(outData.(*[]byte)))
		}

		c.Specify("correctly formats text output", func() {
			err := fileOutput.Init(config)
			defer os.Remove(tmpFilePath)
			c.Assume(err, gs.IsNil)
			outData := make([]byte, 0, 20)

			c.Specify("by default", func() {
				fileOutput.handleMessage(pack, &outData)
				c.Expect(toString(&outData), gs.Equals, *msg.Payload+"\n")
			})

			c.Specify("w/ a prepended timestamp when specified", func() {
				fileOutput.prefix_ts = true
				fileOutput.handleMessage(pack, &outData)
				// Test will fail if date flips btn handleMessage call and
				// todayStr calculation... should be extremely rare.
				todayStr := time.Now().Format("[2006/Jan/02:")
				strContents := toString(&outData)
				payload := *msg.Payload
				c.Expect(strContents, ts.StringContains, payload)
				c.Expect(strContents, ts.StringStartsWith, todayStr)
			})
		})

		c.Specify("correctly formats JSON output", func() {
			config.Format = "json"
			err := fileOutput.Init(config)
			defer os.Remove(tmpFilePath)
			c.Assume(err, gs.IsNil)
			outData := make([]byte, 0, 200)

			c.Specify("when specified", func() {
				fileOutput.handleMessage(pack, &outData)
				msgJson, err := json.Marshal(pack.Message)
				c.Assume(err, gs.IsNil)
				c.Expect(toString(&outData), gs.Equals, string(msgJson)+"\n")
			})

			c.Specify("and with a timestamp", func() {
				fileOutput.prefix_ts = true
				fileOutput.handleMessage(pack, &outData)
				// Test will fail if date flips btn handleMessage call and
				// todayStr calculation... should be extremely rare.
				todayStr := time.Now().Format("[2006/Jan/02:")
				strContents := toString(&outData)
				msgJson, err := json.Marshal(pack.Message)
				c.Assume(err, gs.IsNil)
				c.Expect(strContents, ts.StringContains, string(msgJson)+"\n")
				c.Expect(strContents, ts.StringStartsWith, todayStr)
			})
		})

		c.Specify("correctly formats protocol buffer stream output", func() {
			config.Format = "protobufstream"
			err := fileOutput.Init(config)
			defer os.Remove(tmpFilePath)
			c.Assume(err, gs.IsNil)
			outData := make([]byte, 0, 200)

			c.Specify("when specified and timestamp ignored", func() {
				fileOutput.prefix_ts = true
				err := fileOutput.handleMessage(pack, &outData)
				c.Expect(err, gs.IsNil)
				b := []byte{30, 2, 8, uint8(proto.Size(pack.Message)), 31, 10, 16} // sanity check the header and the start of the protocol buffer
				c.Expect(bytes.Equal(b, outData[:len(b)]), gs.IsTrue)
			})
		})

		c.Specify("processes incoming messages", func() {
			err := fileOutput.Init(config)
			defer os.Remove(tmpFilePath)
			c.Assume(err, gs.IsNil)
			// Don't block on recycle.
			pack.Config.RecycleChan = make(chan *PipelinePack, 10)
			// Save for comparison.
			payload := fmt.Sprintf("%s\n", pack.Message.GetPayload())

			go fileOutput.receiver()
			fileOutput.inChan <- pack
			close(fileOutput.inChan)

			outBatch := <-fileOutput.batchChan
			c.Expect(string(outBatch), gs.Equals, payload)
		})

		c.Specify("commits to a file", func() {
			outStr := "Write me out to the log file"
			outBytes := []byte(outStr)
			fileOutput.wg = new(sync.WaitGroup)
			fileOutput.wg.Add(1)

			c.Specify("with default settings", func() {
				err := fileOutput.Init(config)
				defer os.Remove(tmpFilePath)
				c.Assume(err, gs.IsNil)

				// Start committer loop
				go fileOutput.committer()

				// Feed and close the batchChan
				go func() {
					fileOutput.batchChan <- outBytes
					_ = <-fileOutput.backChan // clear backChan to prevent blocking
					close(fileOutput.batchChan)
				}()

				// Wait for the file close operation to happen.
				for ; err == nil; _, err = fileOutput.file.Stat() {
				}

				tmpFile, err := os.Open(tmpFilePath)
				defer tmpFile.Close()
				c.Assume(err, gs.IsNil)
				contents, err := ioutil.ReadAll(tmpFile)
				c.Assume(err, gs.IsNil)
				c.Expect(string(contents), gs.Equals, outStr)
			})

			c.Specify("with different Perm settings", func() {
				config.Perm = 0600
				err := fileOutput.Init(config)
				defer os.Remove(tmpFilePath)
				c.Assume(err, gs.IsNil)

				go fileOutput.committer()
				go func() {
					fileOutput.batchChan <- outBytes
					_ = <-fileOutput.backChan
					close(fileOutput.batchChan)
				}()

				tmpFile, err := os.Open(tmpFilePath)
				defer tmpFile.Close()
				c.Assume(err, gs.IsNil)
				fileInfo, err := tmpFile.Stat()
				c.Assume(err, gs.IsNil)
				fileMode := fileInfo.Mode()
				// 7 consecutive dashes implies no perms for group or other
				c.Expect(fileMode.String(), ts.StringContains, "-------")
			})
		})
	})

	c.Specify("A TcpWriter", func() {
		tcpWriter := new(TcpWriter)
		config := tcpWriter.ConfigStruct().(*TcpWriterConfig)

		msg := getTestMessage()
		pipelinePack := getTestPipelinePack()
		pipelinePack.Message = msg
		pipelinePack.Decoded = true

		stopAndDelete := func() {
			tcpWriter.Event(STOP)
		}

		c.Specify("makes a pointer to a byte slice", func() {
			outData := tcpWriter.MakeOutData()
			_, ok := outData.(*[]byte)
			c.Expect(ok, gs.IsTrue)
		})

		c.Specify("zeroes a byte slice", func() {
			outBytes := make([]byte, 0, 100)
			str := "This is a test"
			outBytes = append(outBytes, []byte(str)...)
			c.Expect(len(outBytes), gs.Equals, len(str))
			tcpWriter.ZeroOutData(&outBytes)
			c.Expect(len(outBytes), gs.Equals, 0)
		})

		c.Specify("correctly formats protocol buffer stream output", func() {
			outData := tcpWriter.MakeOutData()

			c.Specify("default test message", func() {
				err := tcpWriter.PrepOutData(pipelinePack, outData, nil)
				c.Expect(err, gs.IsNil)
				b := []byte{30, 2, 8, uint8(proto.Size(pipelinePack.Message)), 31, 10, 16} // sanity check the header and the start of the protocol buffer
				c.Expect(bytes.Equal(b, (*outData.(*[]byte))[:len(b)]), gs.IsTrue)
			})
		})

		c.Specify("writes out to the network", func() {
			outStr := "Write me out to the network"
			collectData := func(ch chan string) {
				ln, err := net.Listen("tcp", "localhost:9125")
				if err != nil {
					ch <- err.Error()
				}
				ch <- "ready"
				conn, err := ln.Accept()
				if err != nil {
					ch <- err.Error()
				}
				b := make([]byte, 40)
				n, _ := conn.Read(b)
				ch <- string(b[0:n])
			}
			ch := make(chan string)
			go collectData(ch)
			result := <-ch // wait for server

			err := tcpWriter.Init(config)
			c.Assume(err, gs.IsNil)
			outData := tcpWriter.MakeOutData()
			outBytes := outData.(*[]byte)
			*outBytes = append(*outBytes, []byte(outStr)...)

			defer stopAndDelete()
			c.Assume(err, gs.IsNil)
			err = tcpWriter.Write(outData)
			c.Expect(err, gs.IsNil)
			result = <-ch
			c.Expect(result, gs.Equals, outStr)
		})
	})
}
