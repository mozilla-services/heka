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
#   Victor Ng (vng@mozilla.com)
#   Rob Miller (rmiller@mozilla.com)
#
# ***** END LICENSE BLOCK *****/

package process

import (
	"code.google.com/p/gomock/gomock"
	"fmt"
	. "github.com/mozilla-services/heka/pipeline"
	pipeline_ts "github.com/mozilla-services/heka/pipeline/testsupport"
	"github.com/mozilla-services/heka/pipelinemock"
	plugins_ts "github.com/mozilla-services/heka/plugins/testsupport"
	gs "github.com/rafrombrc/gospec/src/gospec"
	"runtime"
	"time"
)

func ProcessInputSpec(c gs.Context) {
	t := &pipeline_ts.SimpleT{}
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	config := NewPipelineConfig(nil)
	ith := new(plugins_ts.InputTestHelper)
	ith.Msg = pipeline_ts.GetTestMessage()
	ith.Pack = NewPipelinePack(config.InputRecycleChan())

	// Specify localhost, but we're not really going to use the network
	ith.AddrStr = "localhost:55565"
	ith.ResolvedAddrStr = "127.0.0.1:55565"

	// set up mock helper, decoder set, and packSupply channel
	ith.MockHelper = pipelinemock.NewMockPluginHelper(ctrl)
	ith.MockInputRunner = pipelinemock.NewMockInputRunner(ctrl)
	ith.Decoder = pipelinemock.NewMockDecoderRunner(ctrl)
	ith.PackSupply = make(chan *PipelinePack, 1)
	ith.DecodeChan = make(chan *PipelinePack)

	c.Specify("A ProcessInput", func() {
		pInput := ProcessInput{}

		ith.MockInputRunner.EXPECT().InChan().Return(ith.PackSupply).AnyTimes()
		ith.MockInputRunner.EXPECT().Name().Return("logger").AnyTimes()

		enccall := ith.MockHelper.EXPECT().DecoderRunner("RegexpDecoder", "logger-RegexpDecoder").AnyTimes()
		enccall.Return(ith.Decoder, true)

		mockDecoderRunner := ith.Decoder.(*pipelinemock.MockDecoderRunner)
		mockDecoderRunner.EXPECT().InChan().Return(ith.DecodeChan).AnyTimes()

		config := pInput.ConfigStruct().(*ProcessInputConfig)
		config.Command = make(map[string]cmdConfig)

		pConfig := NewPipelineConfig(nil)
		ith.MockHelper.EXPECT().PipelineConfig().Return(pConfig)

		tickChan := make(chan time.Time)
		ith.MockInputRunner.EXPECT().Ticker().Return(tickChan)

		c.Specify("reads a message from ProcessInput", func() {

			pInput.SetName("SimpleTest")
			config.Decoder = "RegexpDecoder"
			config.ParserType = "token"
			config.Delimiter = "|"

			// Note that no working directory is explicitly specified
			config.Command["0"] = cmdConfig{Bin: PROCESSINPUT_TEST1_CMD, Args: PROCESSINPUT_TEST1_CMD_ARGS}
			err := pInput.Init(config)
			c.Assume(err, gs.IsNil)

			go func() {
				pInput.Run(ith.MockInputRunner, ith.MockHelper)
			}()
			tickChan <- time.Now()

			expected_payloads := PROCESSINPUT_TEST1_OUTPUT
			actual_payloads := []string{}

			for x := 0; x < 4; x++ {
				ith.PackSupply <- ith.Pack
				packRef := <-ith.DecodeChan
				c.Expect(ith.Pack, gs.Equals, packRef)
				actual_payloads = append(actual_payloads, *packRef.Message.Payload)
				fPInputName := *packRef.Message.FindFirstField("ProcessInputName")
				c.Expect(fPInputName.ValueString[0], gs.Equals, "SimpleTest.stdout")
				// Free up the scheduler
				runtime.Gosched()
			}

			for x := 0; x < 4; x++ {
				c.Expect(expected_payloads[x], gs.Equals, actual_payloads[x])
			}

			pInput.Stop()
		})

		c.Specify("handles bad arguments", func() {

			pInput.SetName("BadArgs")
			config.ParseStdout = false
			config.ParseStderr = true
			config.Decoder = "RegexpDecoder"
			config.ParserType = "token"
			config.Delimiter = "|"

			// Note that no working directory is explicitly specified
			config.Command["0"] = cmdConfig{Bin: STDERR_CMD, Args: STDERR_CMD_ARGS}

			err := pInput.Init(config)
			c.Assume(err, gs.IsNil)

			expected_err := fmt.Errorf("BadArgs CommandChain::Wait() error: [Subcommand returned an error: [exit status 1]]")
			ith.MockInputRunner.EXPECT().LogError(expected_err)

			go func() {
				pInput.Run(ith.MockInputRunner, ith.MockHelper)
			}()
			tickChan <- time.Now()

			ith.PackSupply <- ith.Pack
			<-ith.DecodeChan
			runtime.Gosched()

			pInput.Stop()
		})

		c.Specify("can pipe multiple commands together", func() {

			pInput.SetName("PipedCmd")
			config.Decoder = "RegexpDecoder"
			config.ParserType = "token"
			// Overload the delimiter
			config.Delimiter = " "

			// Note that no working directory is explicitly specified
			config.Command["0"] = cmdConfig{Bin: PROCESSINPUT_PIPE_CMD1, Args: PROCESSINPUT_PIPE_CMD1_ARGS}
			config.Command["1"] = cmdConfig{Bin: PROCESSINPUT_PIPE_CMD2, Args: PROCESSINPUT_PIPE_CMD2_ARGS}
			err := pInput.Init(config)
			c.Assume(err, gs.IsNil)

			go func() {
				pInput.Run(ith.MockInputRunner, ith.MockHelper)
			}()
			tickChan <- time.Now()

			expected_payloads := PROCESSINPUT_PIPE_OUTPUT
			actual_payloads := []string{}

			for x := 0; x < len(PROCESSINPUT_PIPE_OUTPUT); x++ {
				ith.PackSupply <- ith.Pack
				packRef := <-ith.DecodeChan
				c.Expect(ith.Pack, gs.Equals, packRef)
				actual_payloads = append(actual_payloads, *packRef.Message.Payload)
				fPInputName := *packRef.Message.FindFirstField("ProcessInputName")
				c.Expect(fPInputName.ValueString[0], gs.Equals, "PipedCmd.stdout")
				// Free up the scheduler
				runtime.Gosched()
			}

			for x := 0; x < len(PROCESSINPUT_PIPE_OUTPUT); x++ {
				c.Expect(fmt.Sprintf("[%d] [%s] [%x]",
					len(actual_payloads[x]),
					actual_payloads[x],
					actual_payloads[x]),
					gs.Equals,
					fmt.Sprintf("[%d] [%s] [%x]",
						len(expected_payloads[x]),
						expected_payloads[x],
						expected_payloads[x]))
			}

			pInput.Stop()
		})

	})
}
