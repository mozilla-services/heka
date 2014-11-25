/***** BEGIN LICENSE BLOCK *****
# This Source Code Form is subject to the terms of the Mozilla Public
# License, v. 2.0. If a copy of the MPL was not distributed with this file,
# You can obtain one at http://mozilla.org/MPL/2.0/.
#
# The Initial Developer of the Original Code is the Mozilla Foundation.
# Portions created by the Initial Developer are Copyright (C) 2012-2014
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
	"github.com/mozilla-services/heka/message"
	ts "github.com/mozilla-services/heka/pipeline/testsupport"
	"github.com/rafrombrc/gomock/gomock"
	gs "github.com/rafrombrc/gospec/src/gospec"
	"sync"
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
	}
	pConfig := NewPipelineConfig(globals)
	mockHelper := NewMockPluginHelper(ctrl)
	input := &StoppingInput{}
	maker := &pluginMaker{}
	maker.configStruct = make(map[string]interface{})

	c.Specify("Runner restarts a plugin on the first time only", func() {
		var commonInput CommonInputConfig
		commonInput.Retries = RetryOptions{
			MaxDelay:   "1us",
			Delay:      "1us",
			MaxJitter:  "1us",
			MaxRetries: 1,
		}

		runner := NewInputRunner("stopping", input, commonInput, false)
		runner.(*iRunner).maker = maker
		var wg sync.WaitGroup
		cfgCall := mockHelper.EXPECT().PipelineConfig().Times(2)
		cfgCall.Return(pConfig)
		wg.Add(1)
		runner.Start(mockHelper, &wg)
		wg.Wait()
		c.Expect(stopinputTimes, gs.Equals, 2)
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

type _payloadEncoder struct{}

func (enc *_payloadEncoder) Encode(pack *PipelinePack) (output []byte, err error) {
	return []byte(pack.Message.GetPayload()), nil
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
		maker.configStruct = make(map[string]interface{})
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
				ok := DecodeHeader(result[2:i+1], header)
				c.Expect(ok, gs.IsTrue)
				c.Expect(header.GetMessageLength(), gs.Equals, uint32(len(payload)))
			})
		})
	})
}
