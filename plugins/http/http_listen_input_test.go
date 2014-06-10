/***** BEGIN LICENSE BLOCK *****
# This Source Code Form is subject to the terms of the Mozilla Public
# License, v. 2.0. If a copy of the MPL was not distributed with this file,
# You can obtain one at http://mozilla.org/MPL/2.0/.
#
# The Initial Developer of the Original Code is the Mozilla Foundation.
# Portions created by the Initial Developer are Copyright (C) 2014
# the Initial Developer. All Rights Reserved.
#
# ***** END LICENSE BLOCK *****/

package http

import (
	"code.google.com/p/gomock/gomock"
	. "github.com/mozilla-services/heka/pipeline"
	pipeline_ts "github.com/mozilla-services/heka/pipeline/testsupport"
	"github.com/mozilla-services/heka/pipelinemock"
	plugins_ts "github.com/mozilla-services/heka/plugins/testsupport"
	gs "github.com/rafrombrc/gospec/src/gospec"
	"net/http"
)

func HttpListenInputSpec(c gs.Context) {
	t := &pipeline_ts.SimpleT{}
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	pConfig := NewPipelineConfig(nil)

	httpListenInput := HttpListenInput{}
	ith := new(plugins_ts.InputTestHelper)
	ith.MockHelper = pipelinemock.NewMockPluginHelper(ctrl)
	ith.MockInputRunner = pipelinemock.NewMockInputRunner(ctrl)

	startInput := func() {
		go func() {
			httpListenInput.Run(ith.MockInputRunner, ith.MockHelper)
		}()
	}

	ith.Pack = NewPipelinePack(pConfig.InputRecycleChan())
	ith.PackSupply = make(chan *PipelinePack, 1)

	config := httpListenInput.ConfigStruct().(*HttpListenInputConfig)
	config.Address = "127.0.0.1:8325"
	config.Decoder = "PayloadJsonDecoder"

	ith.MockHelper.EXPECT().PipelineConfig().Return(pConfig)

	mockDecoderRunner := pipelinemock.NewMockDecoderRunner(ctrl)

	dRunnerInChan := make(chan *PipelinePack, 1)
	mockDecoderRunner.EXPECT().InChan().Return(dRunnerInChan).AnyTimes()

	ith.MockInputRunner.EXPECT().InChan().Return(ith.PackSupply).AnyTimes()
	ith.MockInputRunner.EXPECT().Name().Return("HttpListenInput").AnyTimes()
	ith.MockHelper.EXPECT().DecoderRunner("PayloadJsonDecoder", "HttpListenInput-PayloadJsonDecoder").Return(mockDecoderRunner, true)

	err := httpListenInput.Init(config)
	c.Assume(err, gs.IsNil)
	ith.MockInputRunner.EXPECT().LogMessage(gomock.Any())
	startInput()

	c.Specify("A HttpListenInput", func() {
		c.Specify("Adds query parameters to the message pack as fields", func() {
			ith.PackSupply <- ith.Pack
			resp, err := http.Get("http://127.0.0.1:8325/?test=Hello%20World")
			c.Assume(err, gs.IsNil)
			resp.Body.Close()
			c.Assume(resp.StatusCode, gs.Equals, 200)

			pack := <-dRunnerInChan
			fieldValue, ok := pack.Message.GetFieldValue("test")
			c.Assume(ok, gs.IsTrue)
			c.Expect(fieldValue, gs.Equals, "Hello World")
		})
		httpListenInput.Stop()
	})
}
