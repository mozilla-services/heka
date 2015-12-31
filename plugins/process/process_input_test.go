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
#   Victor Ng (vng@mozilla.com)
#   Rob Miller (rmiller@mozilla.com)
#
# ***** END LICENSE BLOCK *****/

package process

import (
	"io"
	"io/ioutil"
	"time"

	. "github.com/mozilla-services/heka/pipeline"
	pipeline_ts "github.com/mozilla-services/heka/pipeline/testsupport"
	"github.com/mozilla-services/heka/pipelinemock"
	plugins_ts "github.com/mozilla-services/heka/plugins/testsupport"
	"github.com/rafrombrc/gomock/gomock"
	gs "github.com/rafrombrc/gospec/src/gospec"
)

func ProcessInputSpec(c gs.Context) {
	t := &pipeline_ts.SimpleT{}
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	pConfig := NewPipelineConfig(nil)
	ith := new(plugins_ts.InputTestHelper)
	ith.Msg = pipeline_ts.GetTestMessage()

	ith.MockHelper = pipelinemock.NewMockPluginHelper(ctrl)
	ith.MockInputRunner = pipelinemock.NewMockInputRunner(ctrl)
	ith.MockDeliverer = pipelinemock.NewMockDeliverer(ctrl)
	ith.MockSplitterRunner = pipelinemock.NewMockSplitterRunner(ctrl)
	ith.Pack = NewPipelinePack(pConfig.InputRecycleChan())

	c.Specify("A ProcessInput", func() {
		pInput := ProcessInput{}

		config := pInput.ConfigStruct().(*ProcessInputConfig)
		config.Command = make(map[string]cmdConfig)

		ith.MockHelper.EXPECT().Hostname().Return(pConfig.Hostname())

		tickChan := make(chan time.Time)
		ith.MockInputRunner.EXPECT().Ticker().Return(tickChan)

		errChan := make(chan error)

		ith.MockSplitterRunner.EXPECT().UseMsgBytes().Return(false)

		decChan := make(chan func(*PipelinePack), 1)
		setDecCall := ith.MockSplitterRunner.EXPECT().SetPackDecorator(gomock.Any())
		setDecCall.Do(func(dec func(*PipelinePack)) {
			decChan <- dec
		})

		bytesChan := make(chan []byte, 1)
		splitCall := ith.MockSplitterRunner.EXPECT().SplitStreamNullSplitterToEOF(gomock.Any(),
			ith.MockDeliverer).Return(nil)
		splitCall.Do(func(r io.Reader, del Deliverer) {
			bytes, err := ioutil.ReadAll(r)
			c.Assume(err, gs.IsNil)
			bytesChan <- bytes
		})

		ith.MockDeliverer.EXPECT().Done()

		c.Specify("using stdout", func() {
			ith.MockInputRunner.EXPECT().NewDeliverer("stdout").Return(ith.MockDeliverer)
			ith.MockInputRunner.EXPECT().NewSplitterRunner("stdout").Return(
				ith.MockSplitterRunner)
			ith.MockSplitterRunner.EXPECT().Done()

			c.Specify("reads a message from ProcessInput", func() {
				pInput.SetName("SimpleTest")

				// Note that no working directory is explicitly specified.
				config.Command["0"] = cmdConfig{
					Bin:  PROCESSINPUT_TEST1_CMD,
					Args: PROCESSINPUT_TEST1_CMD_ARGS,
				}
				err := pInput.Init(config)
				c.Assume(err, gs.IsNil)

				go func() {
					errChan <- pInput.Run(ith.MockInputRunner, ith.MockHelper)
				}()
				tickChan <- time.Now()

				actual := <-bytesChan
				c.Expect(string(actual), gs.Equals, PROCESSINPUT_TEST1_OUTPUT+"\n")

				dec := <-decChan

				dec(ith.Pack)
				fPInputName := ith.Pack.Message.FindFirstField("ProcessInputName")
				c.Expect(fPInputName.ValueString[0], gs.Equals, "SimpleTest.stdout")

				fPInputName = ith.Pack.Message.FindFirstField("ExitStatus")
				c.Expect(fPInputName.ValueInteger[0], gs.Equals, int64(0))

				pInput.Stop()
				err = <-errChan
				c.Expect(err, gs.IsNil)
			})

			c.Specify("can pipe multiple commands together", func() {
				pInput.SetName("PipedCmd")

				// Note that no working directory is explicitly specified.
				config.Command["0"] = cmdConfig{
					Bin:  PROCESSINPUT_PIPE_CMD1,
					Args: PROCESSINPUT_PIPE_CMD1_ARGS,
				}
				config.Command["1"] = cmdConfig{
					Bin:  PROCESSINPUT_PIPE_CMD2,
					Args: PROCESSINPUT_PIPE_CMD2_ARGS,
				}
				err := pInput.Init(config)
				c.Assume(err, gs.IsNil)

				go func() {
					errChan <- pInput.Run(ith.MockInputRunner, ith.MockHelper)
				}()
				tickChan <- time.Now()

				actual := <-bytesChan
				c.Expect(string(actual), gs.Equals, PROCESSINPUT_PIPE_OUTPUT+"\n")

				dec := <-decChan

				dec(ith.Pack)
				fPInputName := ith.Pack.Message.FindFirstField("ProcessInputName")
				c.Expect(fPInputName.ValueString[0], gs.Equals, "PipedCmd.stdout")

				fPInputName = ith.Pack.Message.FindFirstField("ExitStatus")
				c.Expect(fPInputName.ValueInteger[0], gs.Equals, int64(0))

				pInput.Stop()
				err = <-errChan
				c.Expect(err, gs.IsNil)
			})
		})

		c.Specify("using stderr", func() {
			ith.MockInputRunner.EXPECT().NewDeliverer("stderr").Return(ith.MockDeliverer)
			ith.MockInputRunner.EXPECT().NewSplitterRunner("stderr").Return(
				ith.MockSplitterRunner)
			ith.MockSplitterRunner.EXPECT().Done()

			c.Specify("handles bad arguments", func() {
				pInput.SetName("BadArgs")
				config.ParseStdout = false
				config.ParseStderr = true

				// Note that no working directory is explicitly specified.
				config.Command["0"] = cmdConfig{Bin: STDERR_CMD, Args: STDERR_CMD_ARGS}

				err := pInput.Init(config)
				c.Assume(err, gs.IsNil)

				go func() {
					errChan <- pInput.Run(ith.MockInputRunner, ith.MockHelper)
				}()
				tickChan <- time.Now()

				// Error message differs by platform, but we at least wait
				// until we get it.
				<-bytesChan

				pInput.Stop()
				err = <-errChan
				c.Expect(err, gs.IsNil)
			})
		})
	})
}
