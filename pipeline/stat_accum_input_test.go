/***** BEGIN LICENSE BLOCK *****
# This Source Code Form is subject to the terms of the Mozilla Public
# License, v. 2.0. If a copy of the MPL was not distributed with this file,
# You can obtain one at http://mozilla.org/MPL/2.0/.
#
# The Initial Developer of the Original Code is the Mozilla Foundation.
# Portions created by the Initial Developer are Copyright (C) 2013
# the Initial Developer. All Rights Reserved.
#
# Contributor(s):
#   Rob Miller (rmiller@mozilla.com)
#
# ***** END LICENSE BLOCK *****/

package pipeline

import (
	"code.google.com/p/gomock/gomock"
	"github.com/mozilla-services/heka/message"
	ts "github.com/mozilla-services/heka/testsupport"
	gs "github.com/rafrombrc/gospec/src/gospec"
	"strconv"
	"strings"
	"sync"
	"time"
)

func StatAccumInputSpec(c gs.Context) {
	t := &ts.SimpleT{}
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	pConfig := NewPipelineConfig(nil)
	ith := new(InputTestHelper)
	ith.MockHelper = NewMockPluginHelper(ctrl)
	ith.MockInputRunner = NewMockInputRunner(ctrl)
	ith.Pack = NewPipelinePack(pConfig.inputRecycleChan)
	ith.PackSupply = make(chan *PipelinePack, 1)
	ith.PackSupply <- ith.Pack

	c.Specify("A StatAccumInput", func() {
		statAccumInput := StatAccumInput{}
		config := statAccumInput.ConfigStruct().(*StatAccumInputConfig)
		tickChan := make(chan time.Time)

		c.Specify("must emit data in payload and/or message fields", func() {
			config.EmitInFields = false
			err := statAccumInput.Init(config)
			c.Expect(err, gs.Not(gs.IsNil))
			expected := "One of either `EmitInPayload` or `EmitInFields` must be set to true."
			c.Expect(err.Error(), gs.Equals, expected)
		})

		c.Specify("that actually emits a message", func() {
			statName := "sample.stat"
			statVal := int64(303)
			testStat := Stat{statName, strconv.Itoa(int(statVal)), "c", float32(1)}

			validateMsgFields := func(msg *message.Message) {
				c.Expect(len(msg.Fields), gs.Equals, 4)

				// timestamp
				_, ok := msg.GetFieldValue("timestamp")
				c.Expect(ok, gs.IsTrue)

				var tmp interface{}
				var intTmp int64

				// stats.sample.stat
				tmp, ok = msg.GetFieldValue("stats." + statName)
				c.Expect(ok, gs.IsTrue)
				intTmp, ok = tmp.(int64)
				c.Expect(ok, gs.IsTrue)
				c.Expect(intTmp, gs.Equals, int64(0))

				// stats_counts.sample.stat
				tmp, ok = msg.GetFieldValue("stats_counts." + statName)
				c.Expect(ok, gs.IsTrue)
				intTmp, ok = tmp.(int64)
				c.Expect(ok, gs.IsTrue)
				c.Expect(intTmp, gs.Equals, statVal)

				// statsd.numStats
				tmp, ok = msg.GetFieldValue("statsd.numStats")
				c.Expect(ok, gs.IsTrue)
				intTmp, ok = tmp.(int64)
				c.Expect(ok, gs.IsTrue)
				c.Expect(intTmp, gs.Equals, int64(1))
			}

			validateMsgPayload := func(msg *message.Message) {
				lines := strings.Split(msg.GetPayload(), "\n")
				c.Expect(len(lines), gs.Equals, 4)
				c.Expect(lines[3], gs.Equals, "")

				var timestamp string
				for i := 0; i < 3; i++ {
					line := strings.Split(lines[i], " ")
					c.Expect(len(line), gs.Equals, 3)
					switch {
					case i == 0:
						c.Expect(line[0], gs.Equals, "stats."+statName)
						c.Expect(line[1], gs.Equals, "0.000030")
						timestamp = line[2]
					case i == 1:
						c.Expect(line[0], gs.Equals, "stats_counts."+statName)
						c.Expect(line[1], gs.Equals, strconv.Itoa(int(statVal)))
						c.Expect(line[2], gs.Equals, timestamp)
					case i == 2:
						c.Expect(line[0], gs.Equals, "statsd.numStats")
						c.Expect(line[1], gs.Equals, "1")
						c.Expect(line[2], gs.Equals, timestamp)
					}
				}
				expected := strings.Join(lines, "\n")
				c.Expect(msg.GetPayload(), gs.Equals, expected)

			}

			ith.MockHelper.EXPECT().PipelineConfig().Return(pConfig)
			ith.MockInputRunner.EXPECT().InChan().Return(ith.PackSupply)
			ith.MockInputRunner.EXPECT().Inject(ith.Pack)

			var wg sync.WaitGroup

			startAndSwapTickChan := func() {
				wg.Add(1)
				go func() {
					err := statAccumInput.Run(ith.MockInputRunner, ith.MockHelper)
					wg.Done()
					c.Expect(err, gs.IsNil)
				}()
				time.Sleep(50) // Kludgey wait for tickChan to be set so we can replace.
				statAccumInput.tickChan = tickChan
			}

			c.Specify("emits data in fields by default", func() {
				err := statAccumInput.Init(config)
				c.Assume(err, gs.IsNil)

				startAndSwapTickChan()
				statAccumInput.statChan <- testStat
				close(statAccumInput.statChan)
				wg.Wait()
				validateMsgFields(ith.Pack.Message)
				c.Expect(ith.Pack.Message.GetPayload(), gs.Equals, "")
			})

			c.Specify("emits data in payload when specified", func() {
				config.EmitInPayload = true
				err := statAccumInput.Init(config)
				c.Assume(err, gs.IsNil)

				startAndSwapTickChan()
				statAccumInput.statChan <- testStat
				close(statAccumInput.statChan)
				wg.Wait()

				validateMsgFields(ith.Pack.Message)
				validateMsgPayload(ith.Pack.Message)
			})

			c.Specify("omits data in fields when specified", func() {
				config.EmitInFields = false
				config.EmitInPayload = true
				err := statAccumInput.Init(config)
				c.Assume(err, gs.IsNil)

				startAndSwapTickChan()
				statAccumInput.statChan <- testStat
				close(statAccumInput.statChan)
				wg.Wait()

				validateMsgPayload(ith.Pack.Message)
				c.Expect(len(ith.Pack.Message.Fields), gs.Equals, 0)
			})

			c.Specify("honors time ticker to flush", func() {
				err := statAccumInput.Init(config)
				c.Assume(err, gs.IsNil)

				startAndSwapTickChan()
				statAccumInput.statChan <- testStat
				tickChan <- time.Now()
				validateMsgFields(ith.Pack.Message)

				ith.PackSupply <- ith.Pack
				ith.MockInputRunner.EXPECT().InChan().Return(ith.PackSupply)
				ith.MockInputRunner.EXPECT().Inject(ith.Pack)
				close(statAccumInput.statChan)
				wg.Wait()
			})
		})

	})
}
