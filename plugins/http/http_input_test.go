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

package http

import (
	"io"
	"time"

	. "github.com/mozilla-services/heka/pipeline"
	pipeline_ts "github.com/mozilla-services/heka/pipeline/testsupport"
	"github.com/mozilla-services/heka/pipelinemock"
	plugins_ts "github.com/mozilla-services/heka/plugins/testsupport"
	"github.com/rafrombrc/gomock/gomock"
	gs "github.com/rafrombrc/gospec/src/gospec"
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
		ith.MockSplitterRunner = pipelinemock.NewMockSplitterRunner(ctrl)

		runOutputChan := make(chan error, 1)
		startInput := func() {
			go func() {
				runOutputChan <- httpInput.Run(ith.MockInputRunner, ith.MockHelper)
			}()
		}

		ith.Pack = NewPipelinePack(pConfig.InputRecycleChan())

		// These assume that every sub-spec starts the input.
		config := httpInput.ConfigStruct().(*HttpInputConfig)
		tickChan := make(chan time.Time)
		ith.MockInputRunner.EXPECT().Ticker().Return(tickChan)
		ith.MockHelper.EXPECT().Hostname().Return("hekatests.example.com")

		// These assume that every sub-spec makes exactly one HTTP request.
		ith.MockInputRunner.EXPECT().NewSplitterRunner("0").Return(ith.MockSplitterRunner)
		ith.MockSplitterRunner.EXPECT().Done()

		splitCall := ith.MockSplitterRunner.EXPECT().SplitStreamNullSplitterToEOF(gomock.Any(), nil)
		splitCall.Return(io.EOF)
		ith.MockSplitterRunner.EXPECT().UseMsgBytes().Return(false)
		decChan := make(chan func(*PipelinePack), 1)
		packDecCall := ith.MockSplitterRunner.EXPECT().SetPackDecorator(gomock.Any())
		packDecCall.Do(func(dec func(*PipelinePack)) {
			decChan <- dec
		})

		c.Specify("honors time ticker to flush", func() {
			// Spin up a http server.
			server, err := plugins_ts.NewOneHttpServer(json_post, "localhost", 9876)
			c.Expect(err, gs.IsNil)
			go server.Start("/")
			time.Sleep(10 * time.Millisecond)

			config.Url = "http://localhost:9876/"

			err = httpInput.Init(config)
			c.Assume(err, gs.IsNil)
			startInput()
			tickChan <- time.Now()

			// Getting the decorator means we've made our HTTP request.
			<-decChan
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

			err = httpInput.Init(config)
			c.Assume(err, gs.IsNil)
			startInput()
			tickChan <- time.Now()

			dec := <-decChan
			dec(ith.Pack)

			// we expect a statuscode 200 (i.e. success)
			statusCode, ok := ith.Pack.Message.GetFieldValue("StatusCode")
			c.Assume(ok, gs.IsTrue)
			c.Expect(statusCode, gs.Equals, int64(200))
		})

		c.Specify("supports configuring a different HTTP method", func() {
			// Spin up a http server which expects requests with method "POST"
			server, err := plugins_ts.NewHttpMethodServer("POST", "localhost", 9874)
			c.Expect(err, gs.IsNil)
			go server.Start("/PostTest")
			time.Sleep(10 * time.Millisecond)

			config.Url = "http://localhost:9874/PostTest"
			config.Method = "POST"

			err = httpInput.Init(config)
			c.Assume(err, gs.IsNil)
			startInput()
			tickChan <- time.Now()

			dec := <-decChan
			dec(ith.Pack)

			// we expect a statuscode 200 (i.e. success)
			statusCode, ok := ith.Pack.Message.GetFieldValue("StatusCode")
			c.Assume(ok, gs.IsTrue)
			c.Expect(statusCode, gs.Equals, int64(200))
		})

		c.Specify("supports configuring HTTP headers", func() {
			// Spin up a http server which expects requests with method "POST"
			headers := map[string]string{
				"Accept":     "text/plain",
				"user-agent": "CustomUserAgent",
			}

			server, err := plugins_ts.NewHttpHeadersServer(headers, "localhost", 9873)
			c.Expect(err, gs.IsNil)
			go server.Start("/HeadersTest")
			time.Sleep(10 * time.Millisecond)

			config.Url = "http://localhost:9873/HeadersTest"
			config.Headers = headers

			err = httpInput.Init(config)
			c.Assume(err, gs.IsNil)
			startInput()
			tickChan <- time.Now()

			dec := <-decChan
			dec(ith.Pack)

			// we expect a statuscode 200 (i.e. success)
			statusCode, ok := ith.Pack.Message.GetFieldValue("StatusCode")
			c.Assume(ok, gs.IsTrue)
			c.Expect(statusCode, gs.Equals, int64(200))
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

			err = httpInput.Init(config)
			c.Assume(err, gs.IsNil)
			respBodyChan := make(chan []byte, 1)
			splitCall.Do(func(r io.Reader, d Deliverer) {
				respBody := make([]byte, len(config.Body))
				n, err := r.Read(respBody)
				c.Expect(n, gs.Equals, len(config.Body))
				c.Expect(err, gs.Equals, io.EOF)
				respBodyChan <- respBody
			})

			startInput()
			tickChan <- time.Now()

			respBody := <-respBodyChan
			c.Expect(string(respBody), gs.Equals, json_post)
		})

		httpInput.Stop()
		runOutput := <-runOutputChan
		c.Expect(runOutput, gs.IsNil)
	})
}
