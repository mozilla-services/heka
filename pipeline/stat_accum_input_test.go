/***** BEGIN LICENSE BLOCK *****
# This Source Code Form is subject to the terms of the Mozilla Public
# License, v. 2.0. If a copy of the MPL was not distributed with this file,
# You can obtain one at http://mozilla.org/MPL/2.0/.
#
# The Initial Developer of the Original Code is the Mozilla Foundation.
# Portions created by the Initial Developer are Copyright (C) 2013-2015
# the Initial Developer. All Rights Reserved.
#
# Contributor(s):
#   Rob Miller (rmiller@mozilla.com)
#
# ***** END LICENSE BLOCK *****/

package pipeline

import (
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/mozilla-services/heka/message"
	ts "github.com/mozilla-services/heka/pipeline/testsupport"
	"github.com/rafrombrc/gomock/gomock"
	gs "github.com/rafrombrc/gospec/src/gospec"
)

type InputTestHelper struct {
	Msg             *message.Message
	Pack            *PipelinePack
	AddrStr         string
	ResolvedAddrStr string
	MockHelper      *MockPluginHelper
	MockInputRunner *MockInputRunner
	Decoder         DecoderRunner
	PackSupply      chan *PipelinePack
	DecodeChan      chan *PipelinePack
}

func StatAccumInputSpec(c gs.Context) {
	t := &ts.SimpleT{}
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	c.Specify("A StatAccumInput", func() {
		statAccumInput := StatAccumInput{}
		config := statAccumInput.ConfigStruct().(*StatAccumInputConfig)
		pConfig := NewPipelineConfig(nil)
		statAccumInput.pConfig = pConfig

		c.Specify("ticker interval is zero", func() {
			config.TickerInterval = 0
			err := statAccumInput.Init(config)
			c.Expect(err, gs.Not(gs.IsNil))
			expected := "TickerInterval must be greater than 0."
			c.Expect(err.Error(), gs.Equals, expected)
		})

		c.Specify("validates that data is emitted", func() {
			config.EmitInPayload = false
			err := statAccumInput.Init(config)
			c.Expect(err, gs.Not(gs.IsNil))
			expected := "One of either `EmitInPayload` or `EmitInFields` must be set to true."
			c.Expect(err.Error(), gs.Equals, expected)
		})

		c.Specify("that is started", func() {
			ith := new(InputTestHelper)
			ith.MockHelper = NewMockPluginHelper(ctrl)
			ith.MockInputRunner = NewMockInputRunner(ctrl)
			ith.Pack = NewPipelinePack(pConfig.inputRecycleChan)
			ith.PackSupply = make(chan *PipelinePack, 1)
			ith.PackSupply <- ith.Pack

			tickChan := make(chan time.Time)
			var inputStarted sync.WaitGroup

			runErrChan := make(chan error)
			startInput := func() {
				inputStarted.Add(1)

				// A call to Ticker() is the last step in the input's startup before
				// it's ready to process packs. Any tests that need to pause until
				// this happens can use `inputStarted.Wait()`.
				ith.MockInputRunner.EXPECT().Ticker().Do(func() {
					inputStarted.Done()
				}).Return(tickChan)

				go func() {
					err := statAccumInput.Run(ith.MockInputRunner, ith.MockHelper)
					runErrChan <- err
				}()
			}

			ith.MockInputRunner.EXPECT().InChan().Return(ith.PackSupply)
			ith.MockInputRunner.EXPECT().Name().Return("StatAccumInput").AnyTimes()

			injectCall := ith.MockInputRunner.EXPECT().Inject(ith.Pack)
			var injectCalled sync.WaitGroup
			injectCalled.Add(1)
			injectCall.Do(func(pack *PipelinePack) {
				injectCalled.Done()
			})

			c.Specify("using normal namespaces", func() {
				config.EmitInFields = true
				config.EmitInPayload = false
				err := statAccumInput.Init(config)
				c.Expect(err, gs.IsNil)

				finalizeSendingStats := func() (*message.Message, error) {
					close(statAccumInput.statChan)
					err := <-runErrChan
					return ith.Pack.Message, err
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
					startInput()
					sendTimer("sample.timer", 10, 10, 20, 20)
					sendTimer("sample2.timer", 10)
					msg, err := finalizeSendingStats()
					c.Assume(err, gs.Equals, nil)

					validateValueAtKey(msg, "stats.timers.sample.timer.count", int64(4))
					validateValueAtKey(msg, "stats.timers.sample.timer.count_ps", 0.4)
					validateValueAtKey(msg, "stats.timers.sample.timer.mean", 15.0)
					validateValueAtKey(msg, "stats.timers.sample.timer.lower", 10.0)
					validateValueAtKey(msg, "stats.timers.sample.timer.upper", 20.0)
					validateValueAtKey(msg, "stats.timers.sample.timer.sum", 60.0)
					validateValueAtKey(msg, "stats.timers.sample.timer.mean_90", 15.0)
					validateValueAtKey(msg, "stats.timers.sample.timer.upper_90", 20.0)
					validateValueAtKey(msg, "stats.timers.sample2.timer.count", int64(1))
					validateValueAtKey(msg, "stats.timers.sample2.timer.count_ps", 0.1)
					validateValueAtKey(msg, "stats.timers.sample2.timer.mean", 10.0)
					validateValueAtKey(msg, "stats.timers.sample2.timer.lower", 10.0)
					validateValueAtKey(msg, "stats.timers.sample2.timer.upper", 10.0)
					validateValueAtKey(msg, "stats.timers.sample2.timer.sum", 10.0)
					validateValueAtKey(msg, "stats.timers.sample2.timer.mean_90", 10.0)
					validateValueAtKey(msg, "stats.timers.sample2.timer.upper_90", 10.0)

					validateValueAtKey(msg, "stats.statsd.numStats", int64(2))
				})

				c.Specify("emits counters with correct prefixes", func() {
					startInput()
					sendCounter("sample.cnt", 1, 2, 3, 4, 5)
					sendCounter("sample2.cnt", 159, 951)
					msg, err := finalizeSendingStats()
					c.Assume(err, gs.IsNil)

					validateValueAtKey(msg, "stats.counters.sample.cnt.count", int64(15))
					validateValueAtKey(msg, "stats.counters.sample.cnt.rate", 1.5)

					validateValueAtKey(msg, "stats.counters.sample2.cnt.count", int64(1110))
					validateValueAtKey(msg, "stats.counters.sample2.cnt.rate", 1110.0/float64(config.TickerInterval))
				})

				c.Specify("emits gauge with correct prefixes", func() {
					startInput()
					sendGauge("sample.gauge", 1, 2)
					sendGauge("sample2.gauge", 1, 2, 3, 4, 5)
					msg, err := finalizeSendingStats()
					c.Assume(err, gs.IsNil)
					validateValueAtKey(msg, "stats.gauges.sample.gauge", float64(2))
					validateValueAtKey(msg, "stats.gauges.sample2.gauge", float64(5))
				})

				c.Specify("emits correct statsd.numStats count", func() {
					startInput()
					sendGauge("sample.gauge", 1, 2)
					sendGauge("sample2.gauge", 1, 2)
					sendCounter("sample.cnt", 1, 2, 3, 4, 5)
					sendCounter("sample2.cnt", 159, 951)
					sendTimer("sample.timer", 10, 10, 20, 20)
					sendTimer("sample2.timer", 10, 20)
					msg, err := finalizeSendingStats()
					c.Assume(err, gs.IsNil)
					validateValueAtKey(msg, "stats.statsd.numStats", int64(6))
				})

				c.Specify("emits proper idle stats", func() {
					startInput()
					sendGauge("sample.gauge", 1, 2)
					sendCounter("sample.cnt", 1, 2, 3, 4, 5)
					sendTimer("sample.timer", 10, 10, 20, 20)
					inputStarted.Wait()
					tickChan <- time.Now()

					injectCalled.Wait()
					ith.Pack.Recycle(nil)
					ith.PackSupply <- ith.Pack
					ith.MockInputRunner.EXPECT().Inject(ith.Pack)

					msg, err := finalizeSendingStats()
					c.Assume(err, gs.IsNil)
					validateValueAtKey(msg, "stats.gauges.sample.gauge", float64(2))
					validateValueAtKey(msg, "stats.counters.sample.cnt.count", int64(0))
					validateValueAtKey(msg, "stats.timers.sample.timer.count", int64(0))
					validateValueAtKey(msg, "stats.statsd.numStats", int64(3))
				})

				c.Specify("omits idle stats", func() {
					config.DeleteIdleStats = true
					err := statAccumInput.Init(config)
					c.Expect(err, gs.IsNil)

					startInput()
					sendGauge("sample.gauge", 1, 2)
					sendCounter("sample.cnt", 1, 2, 3, 4, 5)
					sendTimer("sample.timer", 10, 10, 20, 20)
					inputStarted.Wait() // Can't flush until the input has started.
					tickChan <- time.Now()
					injectCalled.Wait()

					sendTimer("sample2.timer", 10, 20)
					ith.Pack.Recycle(nil)
					ith.PackSupply <- ith.Pack
					ith.MockInputRunner.EXPECT().Inject(ith.Pack)
					msg, err := finalizeSendingStats()
					c.Assume(err, gs.IsNil)
					validateValueAtKey(msg, "stats.statsd.numStats", int64(1))
				})

			})

			c.Specify("using legacy namespaces", func() {
				config.LegacyNamespaces = true

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

				c.Specify("emits data in payload by default", func() {
					err := statAccumInput.Init(config)
					c.Assume(err, gs.IsNil)

					startInput()
					statAccumInput.statChan <- testStat
					close(statAccumInput.statChan)
					err = <-runErrChan
					c.Expect(err, gs.IsNil)

					validateMsgPayload(ith.Pack.Message)
				})

				c.Specify("emits data in fields when specified", func() {
					config.EmitInFields = true
					err := statAccumInput.Init(config)
					c.Assume(err, gs.IsNil)

					startInput()
					statAccumInput.statChan <- testStat
					close(statAccumInput.statChan)
					err = <-runErrChan
					c.Expect(err, gs.IsNil)
					validateMsgFields(ith.Pack.Message)
					validateMsgPayload(ith.Pack.Message)
				})

				c.Specify("omits data in payload when specified", func() {
					config.EmitInPayload = false
					config.EmitInFields = true
					err := statAccumInput.Init(config)
					c.Assume(err, gs.IsNil)

					startInput()
					statAccumInput.statChan <- testStat
					close(statAccumInput.statChan)
					err = <-runErrChan
					c.Expect(err, gs.IsNil)

					validateMsgFields(ith.Pack.Message)
					c.Expect(ith.Pack.Message.GetPayload(), gs.Equals, "")
				})

				c.Specify("honors time ticker to flush", func() {
					err := statAccumInput.Init(config)
					c.Assume(err, gs.IsNil)

					startInput()
					inputStarted.Wait()

					statAccumInput.statChan <- testStat
					// Sleep until the stat is processed.
					for len(statAccumInput.statChan) > 0 {
						time.Sleep(50)
					}
					tickChan <- time.Now()

					// Don't try to validate our message payload until Inject has
					// been called.
					injectCalled.Wait()
					validateMsgPayload(ith.Pack.Message)

					// Prep pack and EXPECTS for the close.
					ith.PackSupply <- ith.Pack
					ith.MockInputRunner.EXPECT().Inject(ith.Pack)

					close(statAccumInput.statChan)
					err = <-runErrChan
					c.Expect(err, gs.IsNil)
				})

				c.Specify("correctly processes timers", func() {
					sendTimer := func(vals ...int) {
						for _, v := range vals {
							statAccumInput.statChan <- Stat{"sample.timer", strconv.Itoa(int(v)),
								"ms", float32(1)}
						}
					}
					config.EmitInFields = true
					err := statAccumInput.Init(config)
					c.Assume(err, gs.IsNil)
					startInput()

					sendTimer(220, 10, 20, 30, 40, 50, 60, 70, 80, 90, 100)

					close(statAccumInput.statChan)
					err = <-runErrChan
					c.Expect(err, gs.IsNil)

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
	})
}
