/***** BEGIN LICENSE BLOCK *****
# This Source Code Form is subject to the terms of the Mozilla Public
# License, v. 2.0. If a copy of the MPL was not distributed with this file,
# You can obtain one at http://mozilla.org/MPL/2.0/.
#
# The Initial Developer of the Original Code is the Mozilla Foundation.
# Portions created by the Initial Developer are Copyright (C) 2014-2015
# the Initial Developer. All Rights Reserved.
#
# Contributor(s):
#   Rob Miller (rmiller@mozilla.com)
#   Chance Zibolski (chance.zibolski@gmail.com)
#   Anton Lindstrom (carlantonlindstrom@gmail.com)
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
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
)

func HttpListenInputSpec(c gs.Context) {
	t := &pipeline_ts.SimpleT{}
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	pConfig := NewPipelineConfig(nil)
	ith := new(plugins_ts.InputTestHelper)
	ith.Pack = NewPipelinePack(pConfig.InputRecycleChan())

	httpListenInput := HttpListenInput{}
	ith.MockHelper = pipelinemock.NewMockPluginHelper(ctrl)
	ith.MockInputRunner = pipelinemock.NewMockInputRunner(ctrl)
	ith.MockSplitterRunner = pipelinemock.NewMockSplitterRunner(ctrl)

	errChan := make(chan error, 1)
	startInput := func() {
		go func() {
			err := httpListenInput.Run(ith.MockInputRunner, ith.MockHelper)
			errChan <- err
		}()
	}

	config := httpListenInput.ConfigStruct().(*HttpListenInputConfig)
	config.Address = "127.0.0.1:58325"

	c.Specify("A HttpListenInput", func() {
		startedChan := make(chan bool, 1)
		defer close(startedChan)
		ts := httptest.NewUnstartedServer(nil)

		httpListenInput.starterFunc = func(hli *HttpListenInput) error {
			ts.Start()
			startedChan <- true
			return nil
		}

		// These EXPECTs imply that every spec below will send exactly one
		// HTTP request to the input.
		ith.MockInputRunner.EXPECT().NewSplitterRunner(gomock.Any()).Return(
			ith.MockSplitterRunner)
		ith.MockSplitterRunner.EXPECT().UseMsgBytes().Return(false)

		decChan := make(chan func(*PipelinePack), 1)
		feedDecorator := func(decorator func(*PipelinePack)) {
			decChan <- decorator
		}
		setDecCall := ith.MockSplitterRunner.EXPECT().SetPackDecorator(gomock.Any())
		setDecCall.Do(feedDecorator)

		streamChan := make(chan io.Reader, 1)
		feedStream := func(r io.Reader) {
			streamChan <- r
		}
		getRecCall := ith.MockSplitterRunner.EXPECT().GetRecordFromStream(
			gomock.Any()).Do(feedStream)

		bytesChan := make(chan []byte, 1)
		deliver := func(msgBytes []byte, del Deliverer) {
			bytesChan <- msgBytes
		}

		c.Specify("Adds query parameters to the message pack as fields", func() {
			err := httpListenInput.Init(config)
			c.Assume(err, gs.IsNil)
			ts.Config = httpListenInput.server

			getRecCall.Return(0, make([]byte, 0), io.EOF)
			startInput()
			<-startedChan
			resp, err := http.Get(ts.URL + "/?test=Hello%20World")
			resp.Body.Close()
			c.Assume(err, gs.IsNil)
			c.Assume(resp.StatusCode, gs.Equals, 200)

			packDec := <-decChan
			packDec(ith.Pack)
			fieldValue, ok := ith.Pack.Message.GetFieldValue("test")
			c.Assume(ok, gs.IsTrue)
			c.Expect(fieldValue, gs.Equals, "Hello World")
		})

		c.Specify("Add custom headers", func() {
			config.Headers = http.Header{
				"One":  []string{"two", "three"},
				"Four": []string{"five", "six", "seven"},
			}
			err := httpListenInput.Init(config)
			c.Assume(err, gs.IsNil)
			ts.Config = httpListenInput.server

			getRecCall.Return(0, make([]byte, 0), io.EOF)
			startInput()
			<-startedChan
			resp, err := http.Get(ts.URL)
			c.Assume(err, gs.IsNil)
			resp.Body.Close()
			c.Assume(resp.StatusCode, gs.Equals, 200)

			packDec := <-decChan
			packDec(ith.Pack)
			// Verify headers are there
			eq := reflect.DeepEqual(resp.Header["One"], config.Headers["One"])
			c.Expect(eq, gs.IsTrue)
			eq = reflect.DeepEqual(resp.Header["Four"], config.Headers["Four"])
			c.Expect(eq, gs.IsTrue)
		})

		c.Specify("Request body is sent as record", func() {
			err := httpListenInput.Init(config)
			c.Assume(err, gs.IsNil)
			ts.Config = httpListenInput.server

			body := "1+2"
			getRecCall.Return(0, []byte(body), io.EOF)
			startInput()
			<-startedChan
			deliverCall := ith.MockSplitterRunner.EXPECT().DeliverRecord(gomock.Any(),
				nil)
			deliverCall.Do(deliver)
			resp, err := http.Post(ts.URL, "text/plain", strings.NewReader(body))
			c.Assume(err, gs.IsNil)
			resp.Body.Close()
			c.Assume(resp.StatusCode, gs.Equals, 200)

			msgBytes := <-bytesChan
			c.Expect(string(msgBytes), gs.Equals, "1+2")
		})

		c.Specify("Add request headers as fields", func() {
			config.RequestHeaders = []string{
				"X-REQUEST-ID",
			}
			err := httpListenInput.Init(config)
			c.Assume(err, gs.IsNil)
			ts.Config = httpListenInput.server

			getRecCall.Return(0, make([]byte, 0), io.EOF)
			startInput()
			<-startedChan

			client := &http.Client{}
			req, err := http.NewRequest("GET", ts.URL, nil)
			req.Header.Add("X-REQUEST-ID", "12345")
			resp, err := client.Do(req)

			c.Assume(err, gs.IsNil)
			resp.Body.Close()
			c.Assume(resp.StatusCode, gs.Equals, 200)

			packDec := <-decChan
			packDec(ith.Pack)
			fieldValue, ok := ith.Pack.Message.GetFieldValue("X-REQUEST-ID")
			c.Assume(ok, gs.IsTrue)
			c.Expect(fieldValue, gs.Equals, "12345")
		})

		ts.Close()
		httpListenInput.Stop()
		err := <-errChan
		c.Expect(err, gs.IsNil)
	})
}
