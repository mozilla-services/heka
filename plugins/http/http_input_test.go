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
#   Victor Ng (vng@mozilla.com)
#   Rob Miller (rmiller@mozilla.com)
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
	"time"
)

func HttpInputSpec(c gs.Context) {
	t := &pipeline_ts.SimpleT{}
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	pConfig := NewPipelineConfig(nil)

	json_post := `{"uuid": "xxBI3zyeXU+spG8Uiveumw==", "timestamp": 1372966886023588, "hostname": "Victors-MacBook-Air.local", "pid": 40183, "fields": [{"representation": "", "value_type": "STRING", "name": "cef_meta.syslog_priority", "value_string": [""]}, {"representation": "", "value_type": "STRING", "name": "cef_meta.syslog_ident", "value_string": [""]}, {"representation": "", "value_type": "STRING", "name": "cef_meta.syslog_facility", "value_string": [""]}, {"representation": "", "value_type": "STRING", "name": "cef_meta.syslog_options", "value_string": [""]}], "logger": "", "env_version": "0.8", "type": "cef", "payload": "Jul 04 15:41:26 Victors-MacBook-Air.local CEF:0|mozilla|weave|3|xx\\\\|x|xx\\\\|x|5|cs1Label=requestClientApplication cs1=MySuperBrowser requestMethod=GET request=/ src=127.0.0.1 dest=127.0.0.1 suser=none", "severity": 6}'`

	c.Specify("A HttpInput", func() {

		httpInput := HttpInput{}
		ith := new(plugins_ts.InputTestHelper)
		ith.MockHelper = pipelinemock.NewMockPluginHelper(ctrl)
		ith.MockInputRunner = pipelinemock.NewMockInputRunner(ctrl)

		runOutputChan := make(chan string, 1)
		startInput := func() {
			go func() {
				err := httpInput.Run(ith.MockInputRunner, ith.MockHelper)
				var runOutput string
				if err != nil {
					runOutput = err.Error()
				}
				runOutputChan <- runOutput
			}()
		}

		ith.Pack = NewPipelinePack(pConfig.InputRecycleChan())
		ith.PackSupply = make(chan *PipelinePack, 1)
		ith.PackSupply <- ith.Pack

		config := httpInput.ConfigStruct().(*HttpInputConfig)

		c.Specify("short circuits packs into the router", func() {
			config.Url = "http://localhost:9876/"
			tickChan := make(chan time.Time)

			ith.MockInputRunner.EXPECT().LogMessage(gomock.Any()).Times(2)

			ith.MockHelper.EXPECT().PipelineConfig().Return(pConfig)
			ith.MockInputRunner.EXPECT().InChan().Return(ith.PackSupply)
			ith.MockInputRunner.EXPECT().Ticker().Return(tickChan)

			err := httpInput.Init(config)
			c.Assume(err, gs.IsNil)
			startInput()

			tickChan <- time.Now()

			// We need for the pipeline to finish up
			time.Sleep(50 * time.Millisecond)
		})

		c.Specify("with a decoder", func() {

			decoderName := "TestDecoder"
			config.DecoderName = decoderName

			c.Specify("honors time ticker to flush", func() {
				// Spin up a http server
				server, err := plugins_ts.NewOneHttpServer(json_post, "localhost", 9876)
				c.Expect(err, gs.IsNil)
				go server.Start("/")
				time.Sleep(10 * time.Millisecond)

				config.Url = "http://localhost:9876/"
				tickChan := make(chan time.Time)

				ith.MockInputRunner.EXPECT().LogMessage(gomock.Any()).Times(2)

				ith.MockHelper.EXPECT().PipelineConfig().Return(pConfig)
				ith.MockInputRunner.EXPECT().InChan().Return(ith.PackSupply)
				ith.MockInputRunner.EXPECT().Ticker().Return(tickChan)

				mockDecoderRunner := pipelinemock.NewMockDecoderRunner(ctrl)

				// Stub out the DecoderRunner input channel so that we can
				// inspect bytes later on
				dRunnerInChan := make(chan *PipelinePack, 1)
				mockDecoderRunner.EXPECT().InChan().Return(dRunnerInChan)

				ith.MockInputRunner.EXPECT().Name().Return("HttpInput")
				ith.MockHelper.EXPECT().DecoderRunner(decoderName, "HttpInput-TestDecoder").Return(mockDecoderRunner, true)

				err = httpInput.Init(config)
				c.Assume(err, gs.IsNil)
				startInput()

				tickChan <- time.Now()

				// We need for the pipeline to finish up
				time.Sleep(50 * time.Millisecond)
			})

			c.Specify("supports configuring HTTP Basic Authentication", func() {
				// Spin up a http server which expects username "user" and password "password"
				server, err := plugins_ts.NewHttpBasicAuthServer("user", "password", "localhost", 9875)
				c.Expect(err, gs.IsNil)
				go server.Start("/BasicAuthTest")
				time.Sleep(10 * time.Millisecond)

				config.Url = "http://localhost:9875/BasicAuthTest"
				config.User = "user"
				config.Password = "password"
				tickChan := make(chan time.Time)

				ith.MockInputRunner.EXPECT().LogMessage(gomock.Any()).Times(2)

				ith.MockHelper.EXPECT().PipelineConfig().Return(pConfig)
				ith.MockInputRunner.EXPECT().InChan().Return(ith.PackSupply)
				ith.MockInputRunner.EXPECT().Ticker().Return(tickChan)

				mockDecoderRunner := pipelinemock.NewMockDecoderRunner(ctrl)

				// Stub out the DecoderRunner input channel so that we can
				// inspect bytes later on
				dRunnerInChan := make(chan *PipelinePack, 1)
				mockDecoderRunner.EXPECT().InChan().Return(dRunnerInChan)

				ith.MockInputRunner.EXPECT().Name().Return("HttpInput")
				ith.MockHelper.EXPECT().DecoderRunner(decoderName, "HttpInput-TestDecoder").Return(mockDecoderRunner, true)

				err = httpInput.Init(config)
				c.Assume(err, gs.IsNil)
				startInput()

				tickChan <- time.Now()

				// we expect a statuscode 200 (i.e. success)
				pack := <-dRunnerInChan
				statusCode, ok := pack.Message.GetFieldValue("StatusCode")
				c.Assume(ok, gs.IsTrue)
				c.Expect(statusCode, gs.Equals, int64(200))

				// We need for the pipeline to finish up
				time.Sleep(50 * time.Millisecond)
			})

			c.Specify("supports configuring a different HTTP method", func() {
				// Spin up a http server which expects requests with method "POST"
				server, err := plugins_ts.NewHttpMethodServer("POST", "localhost", 9874)
				c.Expect(err, gs.IsNil)
				go server.Start("/PostTest")
				time.Sleep(10 * time.Millisecond)

				config.Url = "http://localhost:9874/PostTest"
				config.Method = "POST"
				tickChan := make(chan time.Time)

				ith.MockInputRunner.EXPECT().LogMessage(gomock.Any()).Times(2)

				ith.MockHelper.EXPECT().PipelineConfig().Return(pConfig)
				ith.MockInputRunner.EXPECT().InChan().Return(ith.PackSupply)
				ith.MockInputRunner.EXPECT().Ticker().Return(tickChan)

				mockDecoderRunner := pipelinemock.NewMockDecoderRunner(ctrl)

				// Stub out the DecoderRunner input channel so that we can
				// inspect bytes later on
				dRunnerInChan := make(chan *PipelinePack, 1)
				mockDecoderRunner.EXPECT().InChan().Return(dRunnerInChan)

				ith.MockInputRunner.EXPECT().Name().Return("HttpInput")
				ith.MockHelper.EXPECT().DecoderRunner(decoderName, "HttpInput-TestDecoder").Return(mockDecoderRunner, true)

				err = httpInput.Init(config)
				c.Assume(err, gs.IsNil)
				startInput()

				tickChan <- time.Now()

				// we expect a statuscode 200 (i.e. success)
				pack := <-dRunnerInChan
				statusCode, ok := pack.Message.GetFieldValue("StatusCode")
				c.Assume(ok, gs.IsTrue)
				c.Expect(statusCode, gs.Equals, int64(200))

				// We need for the pipeline to finish up
				time.Sleep(50 * time.Millisecond)
			})

			c.Specify("supports configuring HTTP headers", func() {
				// Spin up a http server which expects requests with method "POST"
				server, err := plugins_ts.NewHttpHeadersServer(map[string]string{"Accept": "text/plain"}, "localhost", 9873)
				c.Expect(err, gs.IsNil)
				go server.Start("/HeadersTest")
				time.Sleep(10 * time.Millisecond)

				config.Url = "http://localhost:9873/HeadersTest"
				config.Headers = map[string]string{"Accept": "text/plain"}

				tickChan := make(chan time.Time)

				ith.MockInputRunner.EXPECT().LogMessage(gomock.Any()).Times(2)

				ith.MockHelper.EXPECT().PipelineConfig().Return(pConfig)
				ith.MockInputRunner.EXPECT().InChan().Return(ith.PackSupply)
				ith.MockInputRunner.EXPECT().Ticker().Return(tickChan)

				mockDecoderRunner := pipelinemock.NewMockDecoderRunner(ctrl)

				// Stub out the DecoderRunner input channel so that we can
				// inspect bytes later on
				dRunnerInChan := make(chan *PipelinePack, 1)
				mockDecoderRunner.EXPECT().InChan().Return(dRunnerInChan)

				ith.MockInputRunner.EXPECT().Name().Return("HttpInput")
				ith.MockHelper.EXPECT().DecoderRunner(decoderName, "HttpInput-TestDecoder").Return(mockDecoderRunner, true)

				err = httpInput.Init(config)
				c.Assume(err, gs.IsNil)
				startInput()

				tickChan <- time.Now()

				// we expect a statuscode 200 (i.e. success)
				pack := <-dRunnerInChan
				statusCode, ok := pack.Message.GetFieldValue("StatusCode")
				c.Assume(ok, gs.IsTrue)
				c.Expect(statusCode, gs.Equals, int64(200))

				// We need for the pipeline to finish up
				time.Sleep(50 * time.Millisecond)
			})

			c.Specify("supports configuring a request body", func() {
				// Spin up a http server that echoes back the request body
				server, err := plugins_ts.NewHttpBodyServer("localhost", 9872)
				c.Expect(err, gs.IsNil)
				go server.Start("/BodyTest")
				time.Sleep(10 * time.Millisecond)

				config.Url = "http://localhost:9872/BodyTest"
				config.Method = "POST"
				config.Body = json_post

				tickChan := make(chan time.Time)

				ith.MockInputRunner.EXPECT().LogMessage(gomock.Any()).Times(2)

				ith.MockHelper.EXPECT().PipelineConfig().Return(pConfig)
				ith.MockInputRunner.EXPECT().InChan().Return(ith.PackSupply)
				ith.MockInputRunner.EXPECT().Ticker().Return(tickChan)

				mockDecoderRunner := pipelinemock.NewMockDecoderRunner(ctrl)

				// Stub out the DecoderRunner input channel so that we can
				// inspect bytes later on
				dRunnerInChan := make(chan *PipelinePack, 1)
				mockDecoderRunner.EXPECT().InChan().Return(dRunnerInChan)

				ith.MockInputRunner.EXPECT().Name().Return("HttpInput")
				ith.MockHelper.EXPECT().DecoderRunner(decoderName, "HttpInput-TestDecoder").Return(mockDecoderRunner, true)

				err = httpInput.Init(config)
				c.Assume(err, gs.IsNil)
				startInput()

				tickChan <- time.Now()

				pack := <-dRunnerInChan
				c.Expect(*pack.Message.Payload, gs.Equals, json_post)

				// We need for the pipeline to finish up
				time.Sleep(50 * time.Millisecond)
			})

			ith.MockInputRunner.EXPECT().LogMessage(gomock.Any())
			httpInput.Stop()
			runOutput := <-runOutputChan
			c.Expect(runOutput, gs.Equals, "")
		})
	})
}
