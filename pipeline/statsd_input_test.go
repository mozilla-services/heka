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
	// "fmt"
	ts "github.com/mozilla-services/heka/testsupport"
	gs "github.com/rafrombrc/gospec/src/gospec"
	// "sync"
	// "time"
)

func StatsdInputSpec(c gs.Context) {
	t := &ts.SimpleT{}
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	pConfig := NewPipelineConfig(nil)
	ith := new(InputTestHelper)
	ith.Msg = getTestMessage()
	ith.Pack = NewPipelinePack(pConfig.inputRecycleChan)
	ith.PackSupply = make(chan *PipelinePack, 1)

	// Specify localhost, but we're not really going to use the network
	ith.AddrStr = "localhost:55565"
	ith.ResolvedAddrStr = "127.0.0.1:55565"

	// set up mock helper, decoder set, and packSupply channel
	ith.MockHelper = NewMockPluginHelper(ctrl)
	ith.MockInputRunner = NewMockInputRunner(ctrl)

	/*
		The following test code is commented out b/c it's very hard to get it
		working correctly due to timing and synchronization related issues btn
		the StatsdInput and the StatMonitor. Since we already have a ticket
		open for decoupling these two components (https://github.com/mozilla-
		services/heka/issues/119) I'm going to commit this code commented out
		for now, and will immediately start work on decoupling StatMonitor
		from StatsdInput, revisiting these tests when that is complete.

		c.Specify("A StatsdInput", func() {
			statsdInput := StatsdInput{}
			config := statsdInput.ConfigStruct().(*StatsdInputConfig)
			ith.MockInputRunner.EXPECT().InChan().Return(ith.PackSupply)
			ith.PackSupply <- ith.Pack
			ith.MockHelper.EXPECT().PipelineConfig().Times(2).Return(pConfig)
			ith.MockInputRunner.EXPECT().Inject(ith.Pack)
			ith.MockInputRunner.EXPECT().Name().Return("TestingStatsdInput")

			var wg sync.WaitGroup


			c.Specify("without a UDP listener", func() {
				err := statsdInput.Init(config)
				c.Assume(err, gs.IsNil)
				c.Expect(statsdInput.listener, gs.IsNil)

				c.Specify("doesn't return until we close the stopChan", func() {
					wg.Add(1)

					go func() {
						defaulted := false
						err = statsdInput.Run(ith.MockInputRunner, ith.MockHelper)
						select {
						case _, ok := <-statsdInput.stopChan:
							c.Expect(ok, gs.IsFalse)
						default:
							defaulted = true
						}
						c.Expect(defaulted, gs.IsFalse)
						c.Expect(err, gs.IsNil)
						wg.Done()
					}()

					// Explicitly yield the scheduler for a bit to give the
					// goroutine we just created a chance to run.
					time.Sleep(1000)
					statsdInput.Stop()
					wg.Wait()
				})
			})

			c.Specify("with a UDP listener", func() {
				config.Address = ith.AddrStr
				err := statsdInput.Init(config)
				c.Assume(err, gs.IsNil)
					realListener := statsdInput.listener
				c.Expect(realListener.LocalAddr().String(), gs.Equals, ith.ResolvedAddrStr)
				realListener.Close()
					mockListener := ts.NewMockConn(ctrl)
				statsdInput.listener = mockListener
				mockListener.EXPECT().Close()
				mockListener.EXPECT().SetReadDeadline(gomock.Any())
					c.Specify("generates a StatPacket", func() {
					statName := "sample.count"
					statVal := 303
					msg := fmt.Sprintf("%s:%d|c\n", statName, statVal)
					readCall := mockListener.EXPECT().Read(make([]byte, 512))
					readCall.Return(len(msg), nil)
					readCall.Do(func(msgBytes []byte) {
						copy(msgBytes, []byte(msg))
						statsdInput.Stop()
					})
						var wg sync.WaitGroup
					wg.Add(1)
					go func() {
						err = statsdInput.Run(ith.MockInputRunner, ith.MockHelper)
						c.Expect(err, gs.IsNil)
						wg.Done()
					}()
					wg.Wait()
					time.Sleep(50000)
						fieldName := fmt.Sprintf("stats_counts.%s", statName)
					fieldVal, ok := ith.Pack.Message.GetFieldValue(fieldName)
					c.Expect(ok, gs.IsTrue)
					c.Expect(fieldVal.(int64), gs.Equals, int64(statVal))
				})
			})
		})
	*/
}
