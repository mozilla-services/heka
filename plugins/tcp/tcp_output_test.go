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

package tcp

import (
	"io/ioutil"
	"net"
	"os"
	"sync/atomic"
	"time"

	"github.com/gogo/protobuf/proto"
	. "github.com/mozilla-services/heka/pipeline"
	pipeline_ts "github.com/mozilla-services/heka/pipeline/testsupport"
	"github.com/mozilla-services/heka/plugins"
	plugins_ts "github.com/mozilla-services/heka/plugins/testsupport"
	"github.com/rafrombrc/gomock/gomock"
	gs "github.com/rafrombrc/gospec/src/gospec"
)

func TcpOutputSpec(c gs.Context) {
	t := new(pipeline_ts.SimpleT)
	ctrl := gomock.NewController(t)

	tmpDir, tmpErr := ioutil.TempDir("", "tcp-tests")
	defer func() {
		ctrl.Finish()
		tmpErr = os.RemoveAll(tmpDir)
		c.Expect(tmpErr, gs.Equals, nil)
	}()
	globals := DefaultGlobals()
	globals.BaseDir = tmpDir

	pConfig := NewPipelineConfig(globals)
	pConfig.RegisterDefault("HekaFramingSplitter")

	c.Specify("TcpOutput", func() {
		tcpOutput := new(TcpOutput)
		tcpOutput.SetName("test")
		config := tcpOutput.ConfigStruct().(*TcpOutputConfig)
		tcpOutput.Init(config)

		tickChan := make(chan time.Time)
		oth := plugins_ts.NewOutputTestHelper(ctrl)
		oth.MockOutputRunner.EXPECT().Ticker().Return(tickChan).AnyTimes()
		encoder := new(ProtobufEncoder)
		encoder.SetPipelineConfig(pConfig)
		encoder.Init(nil)

		msg := pipeline_ts.GetTestMessage()
		pack := NewPipelinePack(pConfig.InputRecycleChan())
		pack.Message = msg
		pack.QueueCursor = "queuecursor"

		outStr := "Write me out to the network"
		newpack := NewPipelinePack(nil)
		newpack.Message = msg
		newpack.Message.SetPayload(outStr)
		matchBytes, err := proto.Marshal(newpack.Message)
		c.Assume(err, gs.IsNil)
		pack.MsgBytes = matchBytes
		newpack.MsgBytes = matchBytes

		oth.MockHelper.EXPECT().PipelineConfig().Return(pConfig)

		c.Specify("doesn't use framing w/o ProtobufEncoder", func() {
			encoder := new(plugins.PayloadEncoder)
			oth.MockOutputRunner.EXPECT().Encoder().Return(encoder)
			err := tcpOutput.Init(config)
			c.Assume(err, gs.IsNil)
			err = tcpOutput.Prepare(oth.MockOutputRunner, oth.MockHelper)
			c.Expect(err, gs.IsNil)
			// We should fail if SetUseFraming is called since we didn't
			// EXPECT it.
		})

		c.Specify("doesn't use framing if config says not to", func() {
			useFraming := false
			config.UseFraming = &useFraming
			err := tcpOutput.Init(config)
			c.Assume(err, gs.IsNil)
			err = tcpOutput.Prepare(oth.MockOutputRunner, oth.MockHelper)
			c.Expect(err, gs.IsNil)
			// We should fail if SetUseFraming is called since we didn't
			// EXPECT it.
		})

		c.Specify("writes out to the network", func() {
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

			oth.MockOutputRunner.EXPECT().Encoder().Return(encoder)
			oth.MockOutputRunner.EXPECT().SetUseFraming(true)
			err = tcpOutput.Prepare(oth.MockOutputRunner, oth.MockHelper)
			c.Assume(err, gs.IsNil)

			oth.MockOutputRunner.EXPECT().Encode(pack).Return(encoder.Encode(pack))
			oth.MockOutputRunner.EXPECT().UpdateCursor(pack.QueueCursor)

			pack.Message.SetPayload(outStr)

			msgcount := atomic.LoadInt64(&tcpOutput.processMessageCount)
			c.Expect(msgcount, gs.Equals, int64(0))

			err = tcpOutput.ProcessMessage(pack)
			c.Expect(err, gs.IsNil)
			result = <-ch

			msgcount = atomic.LoadInt64(&tcpOutput.processMessageCount)
			c.Expect(msgcount, gs.Equals, int64(1))
			c.Expect(result, gs.Equals, string(matchBytes))

			tcpOutput.CleanUp()
		})

		c.Specify("far end not initially listening", func() {
			oth.MockOutputRunner.EXPECT().LogError(gomock.Any()).AnyTimes()

			err := tcpOutput.Init(config)
			c.Assume(err, gs.IsNil)

			oth.MockOutputRunner.EXPECT().Encoder().Return(encoder)
			oth.MockOutputRunner.EXPECT().SetUseFraming(true)
			err = tcpOutput.Prepare(oth.MockOutputRunner, oth.MockHelper)
			c.Assume(err, gs.IsNil)

			pack.Message.SetPayload(outStr)

			msgcount := atomic.LoadInt64(&tcpOutput.processMessageCount)
			c.Expect(msgcount, gs.Equals, int64(0))

			err = tcpOutput.ProcessMessage(pack)
			_, ok := err.(RetryMessageError)
			c.Expect(ok, gs.IsTrue)
			msgcount = atomic.LoadInt64(&tcpOutput.processMessageCount)
			c.Expect(msgcount, gs.Equals, int64(0))

			// After the message is queued start the collector. However, we
			// don't have a way guarantee a send attempt has already been made
			// and that we are actually exercising the retry code.
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
			result := <-ch

			oth.MockOutputRunner.EXPECT().Encode(pack).Return(encoder.Encode(pack))
			oth.MockOutputRunner.EXPECT().UpdateCursor(pack.QueueCursor)

			err = tcpOutput.ProcessMessage(pack)
			c.Expect(err, gs.IsNil)

			result = <-ch
			c.Expect(result, gs.Equals, string(matchBytes))
			msgcount = atomic.LoadInt64(&tcpOutput.processMessageCount)
			c.Expect(msgcount, gs.Equals, int64(1))

			tcpOutput.CleanUp()
		})

		// c.Specify("Overload queue drops messages", func() {
		// 	config.QueueFullAction = "drop"
		// 	config.QueueMaxBufferSize = uint64(1)
		// 	use_framing := false
		// 	config.UseFraming = &use_framing
		// 	oth.MockOutputRunner.EXPECT().Encode(pack).Return(encoder.Encode(pack))
		// 	oth.MockOutputRunner.EXPECT().LogError(QueueIsFull)

		// 	err := tcpOutput.Init(config)
		// 	c.Expect(err, gs.IsNil)

		// 	startOutput()

		// 	inChan <- pack

		// 	dropcount := atomic.LoadInt64(&tcpOutput.dropMessageCount)

		// 	for x := 0; x < 5 && dropcount == 0; x++ {
		// 		dropcount = atomic.LoadInt64(&tcpOutput.dropMessageCount)
		// 		time.Sleep(time.Duration(100) * time.Millisecond)
		// 	}

		// 	c.Expect(dropcount, gs.Equals, int64(1))

		// 	close(inChan)
		// })

		// c.Specify("Overload queue shutdowns Heka", func() {
		// 	config.QueueFullAction = "shutdown"
		// 	config.QueueMaxBufferSize = uint64(1)
		// 	use_framing := false
		// 	config.UseFraming = &use_framing

		// 	oth.MockOutputRunner.EXPECT().Encode(pack).Return(encoder.Encode(pack))
		// 	oth.MockOutputRunner.EXPECT().LogError(QueueIsFull)

		// 	sigChan := globals.SigChan()

		// 	err := tcpOutput.Init(config)
		// 	c.Expect(err, gs.IsNil)

		// 	startOutput()

		// 	inChan <- pack
		// 	shutdownSignal := <-sigChan
		// 	c.Expect(shutdownSignal, gs.Equals, syscall.SIGINT)

		// 	close(inChan)
		// })

		// c.Specify("Overload queue blocks processing until packet is sent", func() {
		// 	config.QueueFullAction = "block"
		// 	config.QueueMaxBufferSize = uint64(1)
		// 	use_framing := false
		// 	config.UseFraming = &use_framing

		// 	oth.MockOutputRunner.EXPECT().Encode(pack).Return(encoder.Encode(pack)).AnyTimes()
		// 	oth.MockOutputRunner.EXPECT().LogError(QueueIsFull)

		// 	err := tcpOutput.Init(config)
		// 	c.Expect(err, gs.IsNil)

		// 	startOutput()

		// 	inChan <- pack

		// 	msgcount := atomic.LoadInt64(&tcpOutput.dropMessageCount)

		// 	for x := 0; x < 5 && msgcount == 0; x++ {
		// 		msgcount = atomic.LoadInt64(&tcpOutput.dropMessageCount)
		// 		time.Sleep(time.Duration(100) * time.Millisecond)
		// 	}

		// 	c.Expect(atomic.LoadInt64(&tcpOutput.dropMessageCount), gs.Equals, int64(0))
		// 	c.Expect(atomic.LoadInt64(&tcpOutput.processMessageCount), gs.Equals, int64(0))

		// 	close(inChan)
		// })

	})
}
