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
	"crypto/tls"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"

	. "github.com/mozilla-services/heka/pipeline"
	pipeline_ts "github.com/mozilla-services/heka/pipeline/testsupport"
	"github.com/mozilla-services/heka/pipelinemock"
	. "github.com/mozilla-services/heka/plugins/tcp"
	plugins_ts "github.com/mozilla-services/heka/plugins/testsupport"
	"github.com/rafrombrc/gomock/gomock"
	gs "github.com/rafrombrc/gospec/src/gospec"
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
			if hli.conf.UseTls {
				ts.StartTLS()
			} else {
				ts.Start()
			}

			startedChan <- true
			return nil
		}

		// These EXPECTs imply that every spec below will send exactly one
		// HTTP request to the input.
		ith.MockInputRunner.EXPECT().NewSplitterRunner(gomock.Any()).Return(
			ith.MockSplitterRunner)
		ith.MockSplitterRunner.EXPECT().UseMsgBytes().Return(false)
		ith.MockSplitterRunner.EXPECT().Done()

		decChan := make(chan func(*PipelinePack), 1)
		feedDecorator := func(decorator func(*PipelinePack)) {
			decChan <- decorator
		}
		setDecCall := ith.MockSplitterRunner.EXPECT().SetPackDecorator(gomock.Any())
		setDecCall.Do(feedDecorator)

		splitCall := ith.MockSplitterRunner.EXPECT().SplitStreamNullSplitterToEOF(gomock.Any(),
			nil)

		bytesChan := make(chan []byte, 1)
		splitAndDeliver := func(r io.Reader, del Deliverer) {
			msgBytes, _ := ioutil.ReadAll(r)
			bytesChan <- msgBytes
		}

		c.Specify("Adds query parameters to the message pack as fields", func() {
			err := httpListenInput.Init(config)
			c.Assume(err, gs.IsNil)
			ts.Config = httpListenInput.server

			splitCall.Return(io.EOF)
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

			splitCall.Return(io.EOF)
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
			splitCall.Return(io.EOF)
			splitCall.Do(splitAndDeliver)
			startInput()
			<-startedChan
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

			splitCall.Return(io.EOF)
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

		c.Specify("Add other request properties as fields", func() {
			err := httpListenInput.Init(config)
			c.Assume(err, gs.IsNil)
			ts.Config = httpListenInput.server

			splitCall.Return(io.EOF)
			startInput()
			<-startedChan

			client := &http.Client{}
			req, err := http.NewRequest("GET", ts.URL+"/foo/bar?baz=2", nil)
			c.Assume(err, gs.IsNil)
			req.Host = "incoming.example.com:8080"
			resp, err := client.Do(req)
			resp.Body.Close()
			c.Assume(err, gs.IsNil)
			c.Assume(resp.StatusCode, gs.Equals, 200)

			packDec := <-decChan
			packDec(ith.Pack)

			fieldValue, ok := ith.Pack.Message.GetFieldValue("Host")
			c.Assume(ok, gs.IsTrue)
			// Host should not include port number.
			c.Expect(fieldValue, gs.Equals, "incoming.example.com")
			fieldValue, ok = ith.Pack.Message.GetFieldValue("Path")
			c.Assume(ok, gs.IsTrue)
			c.Expect(fieldValue, gs.Equals, "/foo/bar")

			hostname, err := os.Hostname()
			c.Assume(err, gs.IsNil)
			c.Expect(*ith.Pack.Message.Hostname, gs.Equals, hostname)
			c.Expect(*ith.Pack.Message.EnvVersion, gs.Equals, "1")

			fieldValue, ok = ith.Pack.Message.GetFieldValue("RemoteAddr")
			c.Assume(ok, gs.IsTrue)
			c.Expect(fieldValue != nil, gs.IsTrue)
			fieldValueStr, ok := fieldValue.(string)
			c.Assume(ok, gs.IsTrue)
			c.Expect(len(fieldValueStr) > 0, gs.IsTrue)
			// Per the `Request` docs, this should be an IP address:
			// http://golang.org/pkg/net/http/#Request
			ip := net.ParseIP(fieldValueStr)
			c.Expect(ip != nil, gs.IsTrue)
		})

		c.Specify("Test API Authentication", func() {
			config.AuthType = "API"
			config.Key = "123"

			err := httpListenInput.Init(config)
			c.Assume(err, gs.IsNil)
			ts.Config = httpListenInput.server

			splitCall.Return(io.EOF)
			startInput()
			<-startedChan

			client := &http.Client{}
			req, err := http.NewRequest("GET", ts.URL, nil)
			req.Header.Add("X-API-KEY", "123")
			resp, err := client.Do(req)
			c.Assume(err, gs.IsNil)
			resp.Body.Close()
			c.Expect(resp.StatusCode, gs.Equals, 200)
		})

		c.Specify("Test Basic Auth", func() {
			config.AuthType = "Basic"
			config.Username = "foo"
			config.Password = "bar"

			err := httpListenInput.Init(config)
			c.Assume(err, gs.IsNil)
			ts.Config = httpListenInput.server

			splitCall.Return(io.EOF)
			startInput()
			<-startedChan

			client := &http.Client{}
			req, err := http.NewRequest("GET", ts.URL, nil)
			req.SetBasicAuth("foo", "bar")
			resp, err := client.Do(req)
			c.Assume(err, gs.IsNil)
			resp.Body.Close()
			c.Expect(resp.StatusCode, gs.Equals, 200)
		})

		c.Specify("Test TLS", func() {
			config.UseTls = true

			c.Specify("fails to init w/ missing key or cert file", func() {
				config.Tls = TlsConfig{}

				err := httpListenInput.setupTls(&config.Tls)
				c.Expect(err, gs.Not(gs.IsNil))
				c.Expect(err.Error(), gs.Equals,
					"TLS config requires both cert_file and key_file value.")
			})

			config.Tls = TlsConfig{
				CertFile: "./testsupport/cert.pem",
				KeyFile:  "./testsupport/key.pem",
			}

			err := httpListenInput.Init(config)
			c.Assume(err, gs.IsNil)
			ts.Config = httpListenInput.server

			splitCall.Return(io.EOF)
			startInput()
			<-startedChan

			tr := &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
			}
			client := &http.Client{Transport: tr}
			req, err := http.NewRequest("GET", ts.URL, nil)
			resp, err := client.Do(req)
			c.Assume(err, gs.IsNil)
			c.Expect(resp.TLS, gs.Not(gs.IsNil))
			c.Expect(resp.StatusCode, gs.Equals, 200)
			resp.Body.Close()

		})

		ts.Close()
		httpListenInput.Stop()
		err := <-errChan
		c.Expect(err, gs.IsNil)

	})
}
