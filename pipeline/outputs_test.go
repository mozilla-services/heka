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
    "path"
	"encoding/json"
	"errors"
	"fmt"
	ts "github.com/mozilla-services/heka/testsupport"
	gs "github.com/rafrombrc/gospec/src/gospec"
	"io/ioutil"
	"net"
	"os"
	"sync"
	"time"
)

type OutputTestHelper struct {
	MockHelper       *MockPluginHelper
	MockOutputRunner *MockOutputRunner
}

func NewOutputTestHelper(ctrl *gomock.Controller) (oth *OutputTestHelper) {
	oth = new(OutputTestHelper)
	oth.MockHelper = NewMockPluginHelper(ctrl)
	oth.MockOutputRunner = NewMockOutputRunner(ctrl)
	return
}

type PanicOutput struct{}

func (p *PanicOutput) Init(config interface{}) (err error) {
	panic("PANICOUTPUT")
}

func (p *PanicOutput) Run(or OutputRunner, h PluginHelper) (err error) {
	panic("PANICOUTPUT")
}

var stopoutputTimes int

type StoppingOutput struct{}

func (s *StoppingOutput) Init(config interface{}) (err error) {
	if stopoutputTimes > 1 {
		err = errors.New("exiting now")
	}
	return
}

func (s *StoppingOutput) Run(or OutputRunner, h PluginHelper) (err error) {
	return
}

func (s *StoppingOutput) CleanupForRestart() {
	stopoutputTimes += 1
}

func (s *StoppingOutput) Stop() {
	return
}

var (
	stopresumeHolder   []string      = make([]string, 0, 10)
	pack               *PipelinePack = new(PipelinePack)
	stopresumerunTimes int
)

type StopResumeOutput struct{}

func (s *StopResumeOutput) Init(config interface{}) (err error) {
	if stopresumerunTimes > 2 {
		err = errors.New("Aborting")
	}
	return
}

func (s *StopResumeOutput) Run(or OutputRunner, h PluginHelper) (err error) {
	if stopresumerunTimes == 0 {
		or.RetainPack(pack)
	} else if stopresumerunTimes == 1 {
		inChan := or.InChan()
		pk := <-inChan
		if pk == pack {
			stopresumeHolder = append(stopresumeHolder, "success")
		}
	} else if stopresumerunTimes > 1 {
		inChan := or.InChan()
		select {
		case <-inChan:
			stopresumeHolder = append(stopresumeHolder, "oye")
		default:
			stopresumeHolder = append(stopresumeHolder, "woot")
		}
	}
	stopresumerunTimes += 1
	return
}

func (s *StopResumeOutput) CleanupForRestart() {
}

func (s *StopResumeOutput) Stop() {
	return
}

func OutputsSpec(c gs.Context) {
	t := new(ts.SimpleT)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	oth := NewOutputTestHelper(ctrl)
	var wg sync.WaitGroup
	inChan := make(chan *PipelinePack, 1)
	pConfig := NewPipelineConfig(nil)

	c.Specify("A FileOutput", func() {
		fileOutput := new(FileOutput)

		tmpFileName := fmt.Sprintf("fileoutput-test-%d", time.Now().UnixNano())
		tmpFilePath := fmt.Sprint(os.TempDir(), string(os.PathSeparator),
			tmpFileName)
		config := fileOutput.ConfigStruct().(*FileOutputConfig)
		config.Path = tmpFilePath

		msg := getTestMessage()
		pack := NewPipelinePack(pConfig.inputRecycleChan)
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
			// Save for comparison.
			payload := fmt.Sprintf("%s\n", pack.Message.GetPayload())

			oth.MockOutputRunner.EXPECT().InChan().Return(inChan)
			wg.Add(1)
			go fileOutput.receiver(oth.MockOutputRunner, &wg)
			inChan <- pack
			close(inChan)
			outBatch := <-fileOutput.batchChan
			wg.Wait()
			c.Expect(string(outBatch), gs.Equals, payload)
		})

		c.Specify("Init halts if basedirectory is not writable", func() {
            tmpdir := path.Join(os.TempDir(), "tmpdir")
			err := os.MkdirAll(tmpdir, 0400)
			c.Assume(err, gs.IsNil)
			config.Path = tmpdir
			err = fileOutput.Init(config)
			c.Assume(err, gs.Not(gs.IsNil))
		})

		c.Specify("commits to a file", func() {
			outStr := "Write me out to the log file"
			outBytes := []byte(outStr)

			c.Specify("with default settings", func() {
				err := fileOutput.Init(config)
				defer os.Remove(tmpFilePath)
				c.Assume(err, gs.IsNil)

				// Start committer loop
				wg.Add(1)
				go fileOutput.committer(oth.MockOutputRunner, &wg)

				// Feed and close the batchChan
				go func() {
					fileOutput.batchChan <- outBytes
					_ = <-fileOutput.backChan // clear backChan to prevent blocking
					close(fileOutput.batchChan)
				}()

				wg.Wait()
				// Wait for the file close operation to happen.
				//for ; err == nil; _, err = fileOutput.file.Stat() {
				//}

				tmpFile, err := os.Open(tmpFilePath)
				defer tmpFile.Close()
				c.Assume(err, gs.IsNil)
				contents, err := ioutil.ReadAll(tmpFile)
				c.Assume(err, gs.IsNil)
				c.Expect(string(contents), gs.Equals, outStr)
			})

			c.Specify("with different Perm settings", func() {
				config.Perm = "600"
				err := fileOutput.Init(config)
				defer os.Remove(tmpFilePath)
				c.Assume(err, gs.IsNil)

				// Start committer loop
				wg.Add(1)
				go fileOutput.committer(oth.MockOutputRunner, &wg)

				// Feed and close the batchChan
				go func() {
					fileOutput.batchChan <- outBytes
					_ = <-fileOutput.backChan // clear backChan to prevent blocking
					close(fileOutput.batchChan)
				}()

				wg.Wait()
				// Wait for the file close operation to happen.
				//for ; err == nil; _, err = fileOutput.file.Stat() {
				//}

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

	c.Specify("A TcpOutput", func() {
		tcpOutput := new(TcpOutput)
		config := tcpOutput.ConfigStruct().(*TcpOutputConfig)
		tcpOutput.connection = ts.NewMockConn(ctrl)

		msg := getTestMessage()
		pack := NewPipelinePack(pConfig.inputRecycleChan)
		pack.Message = msg
		pack.Decoded = true

		c.Specify("correctly formats protocol buffer stream output", func() {
			outBytes := make([]byte, 0, 200)
			err := createProtobufStream(pack, &outBytes)
			c.Expect(err, gs.IsNil)

			b := []byte{30, 2, 8, uint8(proto.Size(pack.Message)), 31, 10, 16} // sanity check the header and the start of the protocol buffer
			c.Expect(bytes.Equal(b, (outBytes)[:len(b)]), gs.IsTrue)
		})

		c.Specify("writes out to the network", func() {
			inChanCall := oth.MockOutputRunner.EXPECT().InChan().AnyTimes()
			inChanCall.Return(inChan)

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
				b := make([]byte, 1000)
				n, _ := conn.Read(b)
				ch <- string(b[0:n])
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
				tcpOutput.Run(oth.MockOutputRunner, oth.MockHelper)
				wg.Done()
			}()
			inChan <- pack

			close(inChan)
			wg.Wait() // wait for close to finish, prevents intermittent test failures

			matchBytes := make([]byte, 0, 1000)
			err = createProtobufStream(pack, &matchBytes)
			c.Expect(err, gs.IsNil)

			result = <-ch
			c.Expect(result, gs.Equals, string(matchBytes))
		})
	})

	c.Specify("Runner recovers from panic in output's `Run()` method", func() {
		output := new(PanicOutput)
		oRunner := NewFORunner("panic", output, nil)
		var wg sync.WaitGroup
		wg.Add(1)
		oRunner.Start(oth.MockHelper, &wg) // no panic => success
		wg.Wait()
	})

	c.Specify("Runner restarts a plugin on the first time only", func() {
		pc := new(PipelineConfig)
		var pluginGlobals PluginGlobals
		pluginGlobals.Retries = RetryOptions{
			MaxDelay:   "1us",
			Delay:      "1us",
			MaxJitter:  "1us",
			MaxRetries: 1,
		}
		pw := &PluginWrapper{
			name:          "stoppingOutput",
			configCreator: func() interface{} { return nil },
			pluginCreator: func() interface{} { return new(StoppingOutput) },
		}
		output := new(StoppingOutput)
		pc.outputWrappers = make(map[string]*PluginWrapper)
		pc.outputWrappers["stoppingOutput"] = pw
		oRunner := NewFORunner("stoppingOutput", output, &pluginGlobals)
		var wg sync.WaitGroup
		cfgCall := oth.MockHelper.EXPECT().PipelineConfig()
		cfgCall.Return(pc)
		wg.Add(1)
		oRunner.Start(oth.MockHelper, &wg) // no panic => success
		wg.Wait()
		c.Expect(stopoutputTimes, gs.Equals, 2)
	})

	c.Specify("Runner restarts plugin and resumes feeding it", func() {
		pc := new(PipelineConfig)
		var pluginGlobals PluginGlobals
		pluginGlobals.Retries = RetryOptions{
			MaxDelay:   "1us",
			Delay:      "1us",
			MaxJitter:  "1us",
			MaxRetries: 4,
		}
		pw := &PluginWrapper{
			name:          "stoppingresumeOutput",
			configCreator: func() interface{} { return nil },
			pluginCreator: func() interface{} { return new(StopResumeOutput) },
		}
		output := new(StopResumeOutput)
		pc.outputWrappers = make(map[string]*PluginWrapper)
		pc.outputWrappers["stoppingresumeOutput"] = pw
		oRunner := NewFORunner("stoppingresumeOutput", output, &pluginGlobals)
		var wg sync.WaitGroup
		cfgCall := oth.MockHelper.EXPECT().PipelineConfig()
		cfgCall.Return(pc)
		wg.Add(1)
		oRunner.Start(oth.MockHelper, &wg) // no panic => success
		wg.Wait()
		c.Expect(stopresumerunTimes, gs.Equals, 3)
		c.Expect(len(stopresumeHolder), gs.Equals, 2)
		c.Expect(stopresumeHolder[1], gs.Equals, "woot")
		c.Expect(oRunner.retainPack, gs.IsNil)
	})
}
