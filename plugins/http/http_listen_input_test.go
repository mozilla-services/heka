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
	. "github.com/mozilla-services/heka/pipeline"
	pipeline_ts "github.com/mozilla-services/heka/pipeline/testsupport"
	"github.com/mozilla-services/heka/pipelinemock"
	plugins_ts "github.com/mozilla-services/heka/plugins/testsupport"
	"github.com/rafrombrc/gomock/gomock"
	gs "github.com/rafrombrc/gospec/src/gospec"
	"net/http"
	"net/http/httptest"
	"reflect"
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
	config.Decoder = "PayloadJsonDecoder"

	mockDecoderRunner := pipelinemock.NewMockDecoderRunner(ctrl)

	dRunnerInChan := make(chan *PipelinePack, 1)

	c.Specify("A HttpListenInput", func() {
		mockDecoderRunner.EXPECT().InChan().Return(dRunnerInChan)
		ith.MockInputRunner.EXPECT().InChan().Return(ith.PackSupply)
		ith.MockInputRunner.EXPECT().Name().Return("HttpListenInput").Times(2)
		ith.MockHelper.EXPECT().DecoderRunner("PayloadJsonDecoder", "HttpListenInput-PayloadJsonDecoder").Return(mockDecoderRunner, true)
		ith.MockHelper.EXPECT().PipelineConfig().Return(pConfig)

		startedChan := make(chan bool, 1)
		defer close(startedChan)
		ts := httptest.NewUnstartedServer(nil)

		httpListenInput.starterFunc = func(hli *HttpListenInput) error {
			ts.Start()
			startedChan <- true
			return nil
		}

		c.Specify("Adds query parameters to the message pack as fields", func() {
			err := httpListenInput.Init(config)
			ts.Config = httpListenInput.server
			c.Assume(err, gs.IsNil)

			startInput()
			ith.PackSupply <- ith.Pack
			<-startedChan
			resp, err := http.Get(ts.URL + "/?test=Hello%20World")
			c.Assume(err, gs.IsNil)
			resp.Body.Close()
			c.Assume(resp.StatusCode, gs.Equals, 200)

			pack := <-dRunnerInChan
			fieldValue, ok := pack.Message.GetFieldValue("test")
			c.Assume(ok, gs.IsTrue)
			c.Expect(fieldValue, gs.Equals, "Hello World")
		})

		c.Specify("Add custom headers", func() {
			config.Headers = http.Header{
				"One":  []string{"two", "three"},
				"Four": []string{"five", "six", "seven"},
			}
			err := httpListenInput.Init(config)
			ts.Config = httpListenInput.server
			c.Assume(err, gs.IsNil)

			startInput()
			ith.PackSupply <- ith.Pack
			<-startedChan
			resp, err := http.Get(ts.URL)
			c.Assume(err, gs.IsNil)
			resp.Body.Close()
			c.Assume(resp.StatusCode, gs.Equals, 200)
			<-dRunnerInChan

			// Verify headers are there
			eq := reflect.DeepEqual(resp.Header["One"], config.Headers["One"])
			c.Expect(eq, gs.IsTrue)
			eq = reflect.DeepEqual(resp.Header["Four"], config.Headers["Four"])
			c.Expect(eq, gs.IsTrue)

		})

		ts.Close()
		httpListenInput.Stop()
	})
}
