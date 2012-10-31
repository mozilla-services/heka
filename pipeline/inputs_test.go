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
	"bytes"
	"code.google.com/p/gomock/gomock"
	"encoding/gob"
	"encoding/json"
	gs "github.com/orfjackal/gospec/src/gospec"
	"github.com/orfjackal/gospec/src/gospec"
	mocks "heka/pipeline/mocks"
	"net"
	"sync"
	"time"
)

func InputRunnerSpec(c gospec.Context) {
	t := &SimpleT{}
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	c.Specify("An InputRunner", func() {
		second := time.Second

		poolSize := 5
		pipelineCalls := int32(0)
		recycleChan := make(chan *PipelinePack, poolSize+1)
		var wg sync.WaitGroup

		for i := 0; i < poolSize; i++ {
			recycleChan <- getTestPipelinePack()
		}

		//msg := getTestMessage()
		//msgJson, _ := json.Marshal(msg)

		c.Specify("will use all the pipelinePacks (in < 1 sec)", func() {
			mockInput := NewMockInput(ctrl)
			inputRunner := InputRunner{mockInput, &second, false}
			readCall := mockInput.EXPECT().Read(getTestPipelinePack(),
				&second).Times(poolSize)
			readCall.Return(nil)

			done := make(chan bool, 1)
			mockPipeline := func(pipelinePack *PipelinePack) {
				pipelineCalls++
				if int(pipelineCalls) == poolSize {
					done <- true
				}
			}

			inputRunner.Start(mockPipeline, recycleChan, &wg)
			defer inputRunner.Stop()

			var allUsed bool
			select {
			case allUsed = <-done:
			case <-time.After(second):
			}

			c.Expect(allUsed, gs.IsTrue)
		})
	})
}

func InputsSpec(c gospec.Context) {
	t := &SimpleT{}
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	msg := getTestMessage()
	pipelinePack := getTestPipelinePack()

	// Specify localhost, but we're not really going to use the network
	addrStr := "localhost:5565"
	resolvedAddrStr := "127.0.0.1:5565"

	c.Specify("A UdpInput", func() {
		udpInput := NewUdpInput(addrStr, nil)
		realListener := (udpInput.Listener).(*net.UDPConn)
		c.Expect(realListener.LocalAddr().String(), gs.Equals, resolvedAddrStr)
		realListener.Close()

		// Replace the listener object w/ a mock listener
		mockListener := mocks.NewMockConn(ctrl)
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

	c.Specify("A UdpGobInput", func() {
		udpGobInput := NewUdpGobInput(addrStr, nil)
		realListener := (udpGobInput.Listener).(*net.UDPConn)
		c.Expect(realListener.LocalAddr().String(), gs.Equals, resolvedAddrStr)
		realListener.Close()

		// Replace the listener object w/ a mock listener
		mockListener := mocks.NewMockConn(ctrl)
		udpGobInput.Listener = mockListener
		udpGobInput.Decoder = gob.NewDecoder(mockListener)

		encodeBuffer := new(bytes.Buffer)
		gobEncoder := gob.NewEncoder(encodeBuffer)
		gobEncoder.Encode(msg)
		msgGob := make([]byte, 300)
		n, err := encodeBuffer.Read(msgGob)
		msgGob = msgGob[:n]
		c.Assume(err, gs.IsNil)

		putMsgGobInBytes := func(msgBytes []byte) {
			copy(msgBytes, msgGob)
		}

		c.Specify("successfully decodes a message from its listener", func() {
			mockListener.EXPECT().SetReadDeadline(gomock.Any())
			readCall := mockListener.EXPECT().Read(gomock.Any())
			readCall.Return(n, nil)
			readCall.Do(putMsgGobInBytes)
			second := time.Second
			err := udpGobInput.Read(pipelinePack, &second)
			c.Expect(err, gs.IsNil)
			c.Expect(pipelinePack.Decoded, gs.IsTrue)
			c.Expect(pipelinePack.Message, gs.Equals, msg)
		})
	})
}
