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
#   Victor Ng (vng@mozilla.com)
#   Mike Trinkala (trink@mozilla.com)
#
# ***** END LICENSE BLOCK *****/

package pipeline

import (
	"bytes"
	"errors"
	"sync"

	"github.com/gogo/protobuf/proto"
	"github.com/mozilla-services/heka/message"
	ts "github.com/mozilla-services/heka/pipeline/testsupport"
	"github.com/rafrombrc/gomock/gomock"
	gs "github.com/rafrombrc/gospec/src/gospec"
)

var stopinputTimes int

type StoppingInput struct{}

func (s *StoppingInput) Init(config interface{}) (err error) {
	if stopinputTimes > 1 {
		err = errors.New("Stopped enough, done")
	}
	return
}

func (s *StoppingInput) Run(ir InputRunner, h PluginHelper) (err error) {
	err = errors.New("Unclean Exit")
	return
}

func (s *StoppingInput) CleanupForRestart() {
	stopinputTimes += 1
}

func (s *StoppingInput) Stop() {
	return
}

func InputRunnerSpec(c gs.Context) {
	t := &ts.SimpleT{}
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	globals := &GlobalConfigStruct{
		PluginChanSize: 5,
		PoolSize:       1,
	}
	pConfig := NewPipelineConfig(globals)
	pConfig.RegisterDefault("NullSplitter")
	mockHelper := NewMockPluginHelper(ctrl)
	decoder := &_fooDecoder{}

	decoderMaker := &pluginMaker{
		name:     "FooDecoder",
		category: "Decoder",
		pConfig:  pConfig,
	}
	decoderMaker.constructor = func() interface{} {
		return decoder
	}
	decoderMaker.prepConfig = func() (interface{}, error) {
		return make(map[string]interface{}), nil
	}
	pConfig.DecoderMakers["FooDecoder"] = decoderMaker

	c.Specify("An InputRunner", func() {

		var commonInput CommonInputConfig
		commonInput.Retries = RetryOptions{
			MaxDelay:   "1us",
			Delay:      "1us",
			MaxJitter:  "1us",
			MaxRetries: 1,
		}

		var wg sync.WaitGroup

		startRunner := func(runner InputRunner) {
			pConfig.InputRunners[runner.Name()] = runner
			wg.Add(1)
			err := runner.Start(mockHelper, &wg)
			if err != nil {
				wg.Done()
			}
			c.Assume(err, gs.IsNil)
		}

		c.Specify("honors MaxRetries", func() {
			mockHelper.EXPECT().PipelineConfig().Return(pConfig)
			input := &StoppingInput{}
			maker := &pluginMaker{}
			maker.prepConfig = func() (interface{}, error) {
				return make(map[string]interface{}), nil
			}
			runner := NewInputRunner("stopping", input, commonInput).(*iRunner)
			runner.maker = maker
			startRunner(runner)
			wg.Wait()
			c.Expect(stopinputTimes, gs.Equals, 2)
		})

		c.Specify("delivers messages correctly", func() {
			input := &StatAccumInput{
				pConfig: pConfig,
			}
			iConfig := &StatAccumInputConfig{
				EmitInPayload:  true,
				TickerInterval: 10,
			}
			err := input.Init(iConfig)
			c.Assume(err, gs.IsNil)
			pack := NewPipelinePack(pConfig.inputRecycleChan)
			pack.Message = ts.GetTestMessage()
			c.Assume(pack.TrustMsgBytes, gs.IsFalse)
			msgEncoding, err := proto.Marshal(pack.Message)
			c.Assume(err, gs.IsNil)

			c.Specify("if splitters should be visible in reports", func() {
				runner := NewInputRunner("splitinput", input, commonInput).(*iRunner)
				runner.pConfig = pConfig
				c.Expect(len(pConfig.allSplitters), gs.Equals, 0)

				sr := runner.NewSplitterRunner("split")

				c.Expect(len(pConfig.allSplitters), gs.Equals, 1)
				c.Expect(pConfig.allSplitters[0], gs.Equals, sr)
				c.Expect(pConfig.allSplitters[0].Name(), gs.Equals, "splitinput-NullSplitter-split")
			})

			c.Specify("when decoding is synchronous", func() {
				mockHelper.EXPECT().PipelineConfig().Return(pConfig)

				syncDecode := true
				commonInput.SyncDecode = &syncDecode
				commonInput.Decoder = "FooDecoder"
				runner := NewInputRunner("syncdec", input, commonInput).(*iRunner)
				runner.pConfig = pConfig
				d := runner.NewDeliverer("").(*deliverer)
				runner.deliver = d.deliver

				startRunner(runner)

				runner.Deliver(pack)
				recd := <-pConfig.router.inChan

				c.Expect(recd, gs.Equals, pack)
				c.Expect(pack.Message.GetPayload(), gs.Equals, "FOO")

				msgEncoding, err = proto.Marshal(pack.Message)
				c.Expect(err, gs.IsNil)
				c.Expect(pack.TrustMsgBytes, gs.IsTrue)
				c.Expect(bytes.Equal(msgEncoding, pack.MsgBytes), gs.IsTrue)

				// Checking if decoder was added to global list of synchronous decoders
				c.Expect(len(pConfig.allSyncDecoders), gs.Equals, 1)
				c.Expect(pConfig.allSyncDecoders[0].decoder, gs.Equals, d.decoder)
				c.Expect(pConfig.allSyncDecoders[0].name, gs.Equals, "syncdec-FooDecoder")

				d.Done()

				// Tests if decoder was removed safely
				c.Expect(len(pConfig.allSyncDecoders), gs.Equals, 0)

				pack.Recycle(nil)
				input.Stop()
				wg.Wait()
			})

			c.Specify("when there's no decoder", func() {
				mockHelper.EXPECT().PipelineConfig().Return(pConfig)
				runner := NewInputRunner("accum", input, commonInput).(*iRunner)
				runner.pConfig = pConfig
				startRunner(runner)
				runner.Deliver(pack)
				// It was delivered to the router.
				recd := <-pConfig.router.inChan
				c.Expect(recd, gs.Equals, pack)
				// And MsgBytes was populated w/ protobuf encoding.
				c.Expect(pack.TrustMsgBytes, gs.IsTrue)
				c.Expect(bytes.Equal(msgEncoding, pack.MsgBytes), gs.IsTrue)

				pack.Recycle(nil) // Put it back so input.Flush() can use it again.
				input.Stop()
				wg.Wait()
			})

			c.Specify("when using a decoder runner", func() {
				mockHelper.EXPECT().PipelineConfig().Return(pConfig)
				commonInput.Decoder = "FooDecoder"
				runner := NewInputRunner("accum", input, commonInput).(*iRunner)
				runner.pConfig = pConfig
				d := runner.NewDeliverer("").(*deliverer)
				runner.deliver = d.deliver
				startRunner(runner)
				go runner.Deliver(pack)

				dWg := new(sync.WaitGroup)
				dWg.Add(1)
				d.dRunner.Start(pConfig, dWg)

				// Make sure it was passed all the way through to the router.
				recd := <-pConfig.router.inChan
				c.Expect(recd, gs.Equals, pack)
				c.Expect(pack.Message.GetPayload(), gs.Equals, "FOO")

				// Check that DecoderRunner stored protobuf encoding on MsgBytes.
				msgEncoding, err = proto.Marshal(pack.Message)
				c.Expect(err, gs.IsNil)
				c.Expect(pack.TrustMsgBytes, gs.IsTrue)
				c.Expect(bytes.Equal(msgEncoding, pack.MsgBytes), gs.IsTrue)
				close(d.dRunner.InChan())

				pack.Recycle(nil)
				input.Stop()
				wg.Wait()
			})

			c.Specify("when using a decoder", func() {
				mockHelper.EXPECT().PipelineConfig().Return(pConfig)
				b := true
				commonInput.SyncDecode = &b
				commonInput.SendDecodeFailures = &b
				commonInput.Decoder = "FooDecoder"
				runner := NewInputRunner("accum", input, commonInput).(*iRunner)
				runner.pConfig = pConfig
				d := runner.NewDeliverer("").(*deliverer)
				runner.deliver = d.deliver
				startRunner(runner)

				c.Specify("and the decode succeeds", func() {
					runner.Deliver(pack)
					recd := <-pConfig.router.inChan // It was delivered to the router.
					c.Expect(recd, gs.Equals, pack)
					c.Expect(pack.Message.GetPayload(), gs.Equals, "FOO") // And decoded.

					// Make sure deliverFunc stored protobuf encoding on MsgBytes
					msgEncoding, err = proto.Marshal(pack.Message)
					c.Expect(err, gs.IsNil)
					c.Expect(pack.TrustMsgBytes, gs.IsTrue)
					c.Expect(bytes.Equal(msgEncoding, pack.MsgBytes), gs.IsTrue)

					pack.Recycle(nil)
					input.Stop()
					wg.Wait()
				})

				c.Specify("and the decode fails", func() {
					decoder.fail = true
					runner.Deliver(pack)
					recd := <-pConfig.router.inChan // It was delivered to the router.
					c.Expect(recd, gs.Equals, pack)
					c.Expect(pack.Message.GetPayload(), gs.Not(gs.Equals), "FOO") // No decode.
					f := pack.Message.FindFirstField("decode_failure")
					c.Expect(f, gs.Not(gs.IsNil))
					c.Expect(f.GetValue().(bool), gs.IsTrue)
					pack.Recycle(nil)
					input.Stop()
					wg.Wait()
				})

				c.Specify("unless sendDecodeFailure is false", func() {
					decoder.fail = true
					runner.sendDecodeFailures = false
					runner.Deliver(pack)
					var (
						recd *PipelinePack
						ok   bool
					)
					select {
					case recd = <-pConfig.router.inChan:
					default:
						ok = true
					}
					c.Expect(recd, gs.IsNil)
					c.Expect(ok, gs.IsTrue)
					c.Expect(pack.Message.GetPayload(), gs.Equals, "") // Pack was recycled.
					input.Stop()
					wg.Wait()
				})
			})
		})
	})
}

func FilterRunnerSpec(c gs.Context) {
	c.Specify("A filterrunner", func() {
		pConfig := NewPipelineConfig(nil)
		filter := &CounterFilter{}
		commonFO := CommonFOConfig{
			Matcher: "Type == 'bogus'",
		}
		chanSize := 10
		fRunner, err := NewFORunner("counterFilter", filter, commonFO, "CounterFilter",
			chanSize)
		fRunner.h = pConfig
		c.Assume(err, gs.IsNil)

		pack := NewPipelinePack(pConfig.injectRecycleChan)
		pConfig.injectRecycleChan <- pack
		pack.Message = ts.GetTestMessage()
		c.Assume(pack.TrustMsgBytes, gs.IsFalse)
		msgEncoding, err := proto.Marshal(pack.Message)
		c.Assume(err, gs.IsNil)

		c.Specify("puts protobuf encoding into MsgBytes before delivery", func() {
			result := fRunner.Inject(pack)
			c.Expect(result, gs.IsTrue)
			recd := <-pConfig.router.inChan
			c.Expect(recd, gs.Equals, pack)
			c.Expect(recd.TrustMsgBytes, gs.IsTrue)
			c.Expect(bytes.Equal(msgEncoding, recd.MsgBytes), gs.IsTrue)
		})
	})
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

type _fooDecoder struct {
	fail bool
}

func (d *_fooDecoder) Init(config interface{}) error {
	return nil
}

func (d *_fooDecoder) Decode(pack *PipelinePack) (packs []*PipelinePack, err error) {
	if d.fail {
		return nil, errors.New("DECODE ERROR")
	}
	pack.Message.SetPayload("FOO")
	return []*PipelinePack{pack}, nil
}

type _payloadEncoder struct{}

func (enc *_payloadEncoder) Encode(pack *PipelinePack) (output []byte, err error) {
	return []byte(pack.Message.GetPayload()), nil
}

type _ignoreEncoder struct{}

func (enc *_ignoreEncoder) Encode(pack *PipelinePack) (output []byte, err error) {
	return nil, nil
}

var (
	stopresumeHolder   []string      = make([]string, 0, 10)
	_pack              *PipelinePack = new(PipelinePack)
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
		or.RetainPack(_pack)
	} else if stopresumerunTimes == 1 {
		inChan := or.InChan()
		pk := <-inChan
		if pk == _pack {
			stopresumeHolder = append(stopresumeHolder, "success")
		}
	} else if stopresumerunTimes > 1 {
		inChan := or.InChan()
		_, ok := <-inChan
		if !ok {
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

func OutputRunnerSpec(c gs.Context) {
	t := new(ts.SimpleT)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockHelper := NewMockPluginHelper(ctrl)

	c.Specify("A runner", func() {
		stopoutputTimes = 0
		pConfig := NewPipelineConfig(nil)
		output := &StoppingOutput{}
		commonFO := CommonFOConfig{
			Matcher: "TRUE",
		}
		chanSize := 10
		maker := &pluginMaker{}
		maker.prepConfig = func() (interface{}, error) {
			return make(map[string]interface{}), nil
		}
		pConfig.makers["Output"]["stoppingOutput"] = maker

		c.Specify("restarts a plugin on the first time only", func() {
			commonFO.Retries = RetryOptions{
				MaxDelay:   "1us",
				Delay:      "1us",
				MaxJitter:  "1us",
				MaxRetries: 1,
			}
			oRunner, err := NewFORunner("stoppingOutput", output, commonFO, "StoppingOutput",
				chanSize)
			c.Assume(err, gs.IsNil)
			oRunner.maker = maker

			close(oRunner.inChan) // signal the shutdown
			mockHelper.EXPECT().PipelineConfig().Return(pConfig)
			var wg sync.WaitGroup
			wg.Add(1)
			oRunner.Start(mockHelper, &wg) // no panic => success
			wg.Wait()
			c.Expect(stopoutputTimes, gs.Equals, 2)
		})

		c.Specify("restarts plugin and resumes feeding it", func() {
			output := &StopResumeOutput{}
			commonFO.Retries = RetryOptions{
				MaxDelay:   "1us",
				Delay:      "1us",
				MaxJitter:  "1us",
				MaxRetries: 4,
			}
			oRunner, err := NewFORunner("stoppingOutput", output, commonFO,
				"StoppingResumeOutput", chanSize)
			c.Assume(err, gs.IsNil)
			oRunner.maker = maker

			close(oRunner.inChan) // signal the shutdown
			mockHelper.EXPECT().PipelineConfig().Return(pConfig)
			var wg sync.WaitGroup
			wg.Add(1)
			oRunner.Start(mockHelper, &wg) // no panic => success
			wg.Wait()
			c.Expect(stopresumerunTimes, gs.Equals, 3)
			c.Expect(len(stopresumeHolder), gs.Equals, 2)
			c.Expect(stopresumeHolder[1], gs.Equals, "woot")
			c.Expect(oRunner.retainPack, gs.IsNil)
		})

		c.Specify("can exit without causing shutdown", func() {
			commonFO.Retries = RetryOptions{MaxRetries: 0}
			oRunner, err := NewFORunner("stoppingOutput", output, commonFO, "StoppingOutput",
				chanSize)
			c.Assume(err, gs.IsNil)
			oRunner.canExit = true
			oRunner.maker = maker

			// This pack is for the sending of the terminated message
			pack := NewPipelinePack(pConfig.injectRecycleChan)
			pConfig.injectRecycleChan <- pack

			// Feed in a pack to the input so we can verify its been recycled
			// after stopping (no leaks)
			pack = NewPipelinePack(pConfig.inputRecycleChan)
			oRunner.inChan <- pack

			// This is code to emulate the router removing the Output, and
			// closing up the outputs inChan channel
			go func() {
				<-pConfig.Router().RemoveOutputMatcher()
				// We don't close the matcher inChan because its not
				// instantiated in the tests
				close(oRunner.inChan)
			}()

			mockHelper.EXPECT().PipelineConfig().Return(pConfig)
			var wg sync.WaitGroup
			wg.Add(1)
			oRunner.Start(mockHelper, &wg)
			wg.Wait()
			c.Expect(stopoutputTimes, gs.Equals, 1)
			p := <-pConfig.router.inChan
			c.Expect(p.Message.GetType(), gs.Equals, "heka.terminated")
			c.Expect(p.Message.GetLogger(), gs.Equals, "hekad")
			plugin, _ := p.Message.GetFieldValue("plugin")
			c.Expect(plugin, gs.Equals, "stoppingOutput")
			// This should be 1 because the inChan should be flushed
			// and packs should not be leaked
			c.Expect(len(pConfig.inputRecycleChan), gs.Equals, 1)
		})

		c.Specify("encodes a message", func() {
			oRunner, err := NewFORunner("stoppingOutput", output, commonFO, "StoppingOutput",
				chanSize)
			c.Assume(err, gs.IsNil)
			oRunner.encoder = new(_payloadEncoder)
			oRunner.maker = maker
			_pack.Message = ts.GetTestMessage()
			payload := "Test Payload"

			c.Specify("without framing", func() {
				result, err := oRunner.Encode(_pack)
				c.Expect(err, gs.IsNil)
				c.Expect(string(result), gs.Equals, payload)
			})

			c.Specify("with framing", func() {
				oRunner.SetUseFraming(true)
				result, err := oRunner.Encode(_pack)
				c.Expect(err, gs.IsNil)

				i := bytes.IndexByte(result, message.UNIT_SEPARATOR)
				c.Expect(i > 3, gs.IsTrue, -1)
				c.Expect(string(result[i+1:]), gs.Equals, payload)
				header := new(message.Header)
				ok, err := message.DecodeHeader(result[2:i+1], header)
				c.Expect(ok, gs.IsTrue)
				c.Expect(err, gs.IsNil)
				c.Expect(header.GetMessageLength(), gs.Equals, uint32(len(payload)))
			})

			c.Specify("with framing, ignore message", func() {
				oRunner.SetUseFraming(true)
				oRunner.encoder = new(_ignoreEncoder)
				result, err := oRunner.Encode(_pack)
				c.Expect(err, gs.IsNil)
				c.Expect(result == nil, gs.IsTrue)
			})
		})
	})
}
