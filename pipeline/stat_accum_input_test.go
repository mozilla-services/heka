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

	c.Specify("A StatAccumInput using normal namespaces", func() {
		ith.MockHelper.EXPECT().PipelineConfig().Return(pConfig)
		ith.MockInputRunner.EXPECT().InChan().Return(ith.PackSupply)
		ith.MockInputRunner.EXPECT().Inject(ith.Pack)
		ith.MockInputRunner.EXPECT().Ticker()

		statAccumInput := StatAccumInput{}
		config := statAccumInput.ConfigStruct().(*StatAccumInputConfig)
		config.EmitInFields = true
		config.EmitInPayload = false
		err := statAccumInput.Init(config)
		c.Expect(err, gs.IsNil)

		var wg sync.WaitGroup
		prepareSendingStats := func() {
			wg.Add(1)
			go func() {
				err := statAccumInput.Run(ith.MockInputRunner, ith.MockHelper)
				wg.Done()
				c.Expect(err, gs.IsNil)
			}()
		}
		finalizeSendingStats := func() *message.Message {
			close(statAccumInput.statChan)
			wg.Wait()
			return ith.Pack.Message
		}

		sendTimer := func(key string, vals ...int) {
			for _, v := range vals {
				statAccumInput.statChan <- Stat{key, strconv.Itoa(v), "ms", float32(1)}
			}
		}
		sendCounter := func(key string, vals ...int) {
			for _, v := range vals {
				statAccumInput.statChan <- Stat{key, strconv.Itoa(v), "c", float32(1)}
			}
		}
		sendGauge := func(key string, vals ...int) {
			for _, v := range vals {
				statAccumInput.statChan <- Stat{key, strconv.Itoa(v), "g", float32(1)}
			}
		}

		validateValueAtKey := func(msg *message.Message, key string, value interface{}) {
			fieldValue, ok := msg.GetFieldValue(key)
			c.Expect(ok, gs.IsTrue)
			c.Expect(fieldValue, gs.Equals, value)
		}
		c.Specify("emits timer with correct prefixes", func() {
			prepareSendingStats()
			sendTimer("sample.timer", 10, 10, 20, 20)
			sendTimer("sample2.timer", 10, 20)
			msg := finalizeSendingStats()

			validateValueAtKey(msg, "sample.timer.count", int64(4))
			validateValueAtKey(msg, "sample.timer.mean", 15.0)
			validateValueAtKey(msg, "sample.timer.lower", 10.0)
			validateValueAtKey(msg, "sample2.timer.count", int64(2))
			validateValueAtKey(msg, "sample2.timer.mean", 15.0)
			validateValueAtKey(msg, "sample2.timer.lower", 10.0)

			validateValueAtKey(msg, "statsd.numStats", int64(2))
		})
		c.Specify("emits counters with correct prefixes", func() {
			prepareSendingStats()
			sendCounter("sample.cnt", 1, 2, 3, 4, 5)
			sendCounter("sample2.cnt", 159, 951)
			msg := finalizeSendingStats()

			validateValueAtKey(msg, "sample.cnt.count", int64(15))
			validateValueAtKey(msg, "sample.cnt.rate", 1.5)

			validateValueAtKey(msg, "sample2.cnt.count", int64(1110))
			validateValueAtKey(msg, "sample2.cnt.rate", 1110.0/float64(config.TickerInterval))
		})
		c.Specify("emits gauge with correct prefixes", func() {
			prepareSendingStats()
			sendGauge("sample.gauge", 1, 2)
			sendGauge("sample2.gauge", 1, 2, 3, 4, 5)
			msg := finalizeSendingStats()
			validateValueAtKey(msg, "sample.gauge", int64(2))
			validateValueAtKey(msg, "sample2.gauge", int64(5))
		})

		c.Specify("emits correct statsd.numStats count", func() {
			prepareSendingStats()
			sendGauge("sample.gauge", 1, 2)
			sendGauge("sample2.gauge", 1, 2)
			sendCounter("sample.cnt", 1, 2, 3, 4, 5)
			sendCounter("sample2.cnt", 159, 951)
			sendTimer("sample.timer", 10, 10, 20, 20)
			sendTimer("sample2.timer", 10, 20)
			msg := finalizeSendingStats()
			validateValueAtKey(msg, "statsd.numStats", int64(6))
		})
	})

	c.Specify("A StatAccumInput using Legacy namespaces", func() {
		statAccumInput := StatAccumInput{}
		config := statAccumInput.ConfigStruct().(*StatAccumInputConfig)
		config.LegacyNamespaces = true

		tickChan := make(chan time.Time)

		c.Specify("must emit data in payload and/or message fields", func() {
			config.EmitInPayload = false
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
				c.Expect(intTmp, gs.Equals, int64(30))

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
					switch i {
					case 0:
						c.Expect(line[0], gs.Equals, "stats."+statName)
						c.Expect(line[1], gs.Equals, "30.300000")
						timestamp = line[2]
					case 1:
						c.Expect(line[0], gs.Equals, "stats_counts."+statName)
						c.Expect(line[1], gs.Equals, strconv.Itoa(int(statVal)))
						c.Expect(line[2], gs.Equals, timestamp)
					case 2:
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
			ith.MockInputRunner.EXPECT().Ticker()

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

			c.Specify("emits data in payload by default", func() {
				err := statAccumInput.Init(config)
				c.Assume(err, gs.IsNil)

				startAndSwapTickChan()
				statAccumInput.statChan <- testStat
				close(statAccumInput.statChan)
				wg.Wait()

				validateMsgPayload(ith.Pack.Message)
			})

			c.Specify("emits data in fields when specified", func() {
				config.EmitInFields = true
				err := statAccumInput.Init(config)
				c.Assume(err, gs.IsNil)

				startAndSwapTickChan()
				statAccumInput.statChan <- testStat
				close(statAccumInput.statChan)
				wg.Wait()
				validateMsgFields(ith.Pack.Message)
				validateMsgPayload(ith.Pack.Message)
			})

			c.Specify("omits data in payload when specified", func() {
				config.EmitInPayload = false
				config.EmitInFields = true
				err := statAccumInput.Init(config)
				c.Assume(err, gs.IsNil)

				startAndSwapTickChan()
				statAccumInput.statChan <- testStat
				close(statAccumInput.statChan)
				wg.Wait()

				validateMsgFields(ith.Pack.Message)
				c.Expect(ith.Pack.Message.GetPayload(), gs.Equals, "")
			})

			c.Specify("honors time ticker to flush", func() {
				err := statAccumInput.Init(config)
				c.Assume(err, gs.IsNil)

				startAndSwapTickChan()
				statAccumInput.statChan <- testStat
				tickChan <- time.Now()
				validateMsgPayload(ith.Pack.Message)

				ith.PackSupply <- ith.Pack
				ith.MockInputRunner.EXPECT().InChan().Return(ith.PackSupply)
				ith.MockInputRunner.EXPECT().Inject(ith.Pack)
				close(statAccumInput.statChan)
				wg.Wait()
			})

			c.Specify("correctly processes timers", func() {
				sendTimer := func(vals ...int) {
					for _, v := range vals {
						statAccumInput.statChan <- Stat{"sample.timer", strconv.Itoa(int(v)), "ms", float32(1)}
					}
				}
				config.EmitInFields = true
				err := statAccumInput.Init(config)
				c.Assume(err, gs.IsNil)
				startAndSwapTickChan()

				sendTimer(220, 10, 20, 30, 40, 50, 60, 70, 80, 90, 100)

				close(statAccumInput.statChan)
				wg.Wait()

				msg := ith.Pack.Message

				getVal := func(token string) float64 {
					tmp, ok := msg.GetFieldValue("stats.timers.sample.timer." + token)
					c.Expect(ok, gs.IsTrue)
					val, ok := tmp.(float64)
					c.Expect(ok, gs.IsTrue)
					return val
				}

				c.Expect(getVal("upper"), gs.Equals, 220.0)
				c.Expect(getVal("lower"), gs.Equals, 10.0)
				c.Expect(getVal("mean"), gs.Equals, 70.0)
				c.Expect(getVal("upper_90"), gs.Equals, 100.0)
				c.Expect(getVal("mean_90"), gs.Equals, 55.0)
				tmp, ok := msg.GetFieldValue("stats.timers.sample.timer.count")
				c.Expect(ok, gs.IsTrue)
				intTmp, ok := tmp.(int64)
				c.Expect(ok, gs.IsTrue)
				c.Expect(intTmp, gs.Equals, int64(11))
			})
		})
	})
}
