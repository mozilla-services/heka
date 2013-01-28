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
#
# ***** END LICENSE BLOCK *****/
package pipeline

import (
	"code.google.com/p/gomock/gomock"
	"code.google.com/p/goprotobuf/proto"
	"encoding/json"
	"errors"
	"github.com/mozilla-services/heka/message"
	ts "github.com/mozilla-services/heka/testsupport"
	"github.com/rafrombrc/go-notify"
	gs "github.com/rafrombrc/gospec/src/gospec"
	"net"
	"sync"
	"time"
)

func InputRunnerSpec(c gs.Context) {
	t := &ts.SimpleT{}
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	c.Specify("An InputRunner", func() {
		second := time.Second

		poolSize := 5
		pipelineCalls := 0
		mockInput := NewMockInput(ctrl)
		inputRunner := InputRunner{"mock", mockInput, &second}
		defer notify.StopAll(STOP)

		recycleChan := make(chan *PipelinePack, poolSize+1)
		for i := 0; i < poolSize; i++ {
			recycleChan <- getTestPipelinePack()
		}

		var wg sync.WaitGroup
		comparePack := getTestPipelinePack()
		done := make(chan bool, 1)
		mu := sync.Mutex{}

		mockPipeline := func(pack *PipelinePack) {
			mu.Lock()
			pipelineCalls++
			mu.Unlock()
			if pipelineCalls == poolSize {
				done <- true
			}
		}

		c.Specify("will use all the pipelinePacks (in < 1 sec)", func() {
			readCall := mockInput.EXPECT().Read(comparePack, &second).Times(poolSize)
			readCall.Return(nil)

			inputRunner.Start(mockPipeline, recycleChan, &wg)
			wg.Add(1)
			defer notify.Post(STOP, nil)

			var allUsed bool
			select {
			case allUsed = <-done:
			case <-time.After(second):
			}

			c.Expect(allUsed, gs.IsTrue)
		})

		c.Specify("even if there are read errors", func() {
			readCall := mockInput.EXPECT().Read(comparePack, &second).Times(poolSize * 2)
			i := 0
			readCall.Do(func(pipelinePack *PipelinePack, timeout *time.Duration) {
				if i < poolSize {
					readCall.Return(errors.New("Test Error"))
				} else {
					readCall.Return(nil)
				}
				i++
			})

			inputRunner.Start(mockPipeline, recycleChan, &wg)
			wg.Add(1)
			defer notify.Post(STOP, nil)

			var allUsed bool
			select {
			case allUsed = <-done:
			case <-time.After(second):
			}

			c.Expect(allUsed, gs.IsTrue)
		})
	})
}

func InputsSpec(c gs.Context) {
	t := &ts.SimpleT{}
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	msg := getTestMessage()
	pipelinePack := getTestPipelinePack()

	// Specify localhost, but we're not really going to use the network
	addrStr := "localhost:55565"
	resolvedAddrStr := "127.0.0.1:55565"

	c.Specify("A UdpInput", func() {
		udpInput := UdpInput{}
		err := udpInput.Init(&UdpInputConfig{addrStr})
		c.Assume(err, gs.IsNil)
		realListener := (udpInput.Listener).(*net.UDPConn)
		c.Expect(realListener.LocalAddr().String(), gs.Equals, resolvedAddrStr)
		realListener.Close()

		// Replace the listener object w/ a mock listener
		mockListener := ts.NewMockConn(ctrl)
		udpInput.Listener = mockListener

		msgJson, _ := json.Marshal(msg)
		putMsgJsonInBytes := func(msgBytes []byte) {
			copy(msgBytes, msgJson)
		}

		c.Specify("reads a message from its listener", func() {
			mockListener.EXPECT().SetReadDeadline(gomock.Any())
			readCall := mockListener.EXPECT().Read(pipelinePack.MsgBytes)
			readCall.Return(len(msgJson), nil)
			readCall.Do(putMsgJsonInBytes)
			second := time.Second
			err := udpInput.Read(pipelinePack, &second)
			c.Expect(err, gs.IsNil)
			c.Expect(pipelinePack.Decoded, gs.IsFalse)
			c.Expect(string(pipelinePack.MsgBytes), gs.Equals, string(msgJson))
		})
	})

	c.Specify("A TcpInput", func() {
		tcpInput := TcpInput{}
		err := tcpInput.Init(&TcpInputConfig{addrStr})
		c.Assume(err, gs.IsNil)
		mockConnection := ts.NewMockConn(ctrl)

		/// @todo use the msg encoder
		mbytes, _ := proto.Marshal(msg)
		header := &message.Header{}
		header.SetMessageLength(uint32(len(mbytes)))
		hbytes, _ := proto.Marshal(header)
		buflen := 3 + len(hbytes) + len(mbytes)
		putPayloadInBytes := func(msgBytes []byte) {
			msgBytes[0] = RECORD_SEPARATOR
			msgBytes[1] = uint8(len(hbytes))
			copy(msgBytes[2:], hbytes)
			pos := 2 + len(hbytes)
			msgBytes[pos] = UNIT_SEPARATOR
			copy(msgBytes[pos+1:], mbytes)
		}

		c.Specify("reads a message from its connection", func() {
			buf := make([]byte, MAX_MESSAGE_SIZE+MAX_HEADER_SIZE)
			err = errors.New("connection closed")
			closeCall := mockConnection.EXPECT().Close()
			closeCall.Do(func() {})
			readCall := mockConnection.EXPECT().Read(buf)
			readCall.Return(buflen, err)
			readCall.Do(putPayloadInBytes)
			second := time.Second
			tcpInput.handleConnection(mockConnection)
			err = tcpInput.Read(pipelinePack, &second)
			c.Expect(err, gs.IsNil)
			c.Expect(pipelinePack.Decoded, gs.IsTrue)
			v, ok := pipelinePack.Message.GetFieldValue("foo")
			c.Expect(ok, gs.IsTrue)
			c.Expect(v, gs.Equals, "bar")
		})
	})
}
