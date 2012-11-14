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
	"encoding/json"
	"errors"
	"github.com/rafrombrc/gospec/src/gospec"
	gs "github.com/rafrombrc/gospec/src/gospec"
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
		pipelineCalls := 0
		mockInput := NewMockInput(ctrl)
		inputRunner := InputRunner{"mock", mockInput, &second, false}

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

			var allUsed bool
			select {
			case allUsed = <-done:
			case <-time.After(second):
			}

			c.Expect(allUsed, gs.IsTrue)
			inputRunner.Stop()
			wg.Wait()
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

			var allUsed bool
			select {
			case allUsed = <-done:
			case <-time.After(second):
			}

			c.Expect(allUsed, gs.IsTrue)
			inputRunner.Stop()
			wg.Wait()
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
		udpInput := UdpInput{}
		udpInput.Init(&UdpInputConfig{addrStr})
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
}
