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
#
# ***** END LICENSE BLOCK *****/

package http

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"time"

	"github.com/mozilla-services/heka/pipeline"
	pipeline_ts "github.com/mozilla-services/heka/pipeline/testsupport"
	"github.com/mozilla-services/heka/plugins"
	ts "github.com/mozilla-services/heka/plugins/testsupport"
	"github.com/rafrombrc/gomock/gomock"
	gs "github.com/rafrombrc/gospec/src/gospec"
)

type _test_handler struct {
	serveHttp func(http.ResponseWriter, *http.Request)
	respBody  string
	respCode  int
}

func (h *_test_handler) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	h.serveHttp(rw, req)
}

func HttpOutputSpec(c gs.Context) {
	t := new(pipeline_ts.SimpleT)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	var (
		reqMethod string
		reqBody   string
		reqHeader http.Header
		handleWg  sync.WaitGroup
		runWg     sync.WaitGroup
		delay     bool
	)

	handler := new(_test_handler)
	handler.serveHttp = func(rw http.ResponseWriter, req *http.Request) {
		defer handleWg.Done()
		if delay {
			time.Sleep(time.Duration(2) * time.Millisecond)
		}
		// Capture request body for test assertions.
		p := make([]byte, req.ContentLength)
		_, err := req.Body.Read(p)
		c.Expect(err.Error(), gs.Equals, "EOF")
		reqBody = string(p)
		// Capture request method and headers for test assertions.
		reqMethod = req.Method
		reqHeader = req.Header
		// Respond with either a response body or a response code.
		if len(handler.respBody) > 0 {
			rw.Write([]byte(handler.respBody))
		} else {
			rw.WriteHeader(handler.respCode)
		}
		flusher := rw.(http.Flusher)
		flusher.Flush()
	}

	oth := ts.NewOutputTestHelper(ctrl)
	encoder := new(plugins.PayloadEncoder)
	encConfig := new(plugins.PayloadEncoderConfig)
	err := encoder.Init(encConfig)
	c.Expect(err, gs.IsNil)
	inChan := make(chan *pipeline.PipelinePack, 1)
	recycleChan := make(chan *pipeline.PipelinePack, 1)
	pack := pipeline.NewPipelinePack(recycleChan)
	pack.Message = pipeline_ts.GetTestMessage()

	c.Specify("An HttpOutput", func() {
		httpOutput := new(HttpOutput)
		config := httpOutput.ConfigStruct().(*HttpOutputConfig)

		c.Specify("barfs on bogus URLs", func() {
			config.Address = "one-two-three-four"
			err := httpOutput.Init(config)
			c.Expect(err, gs.Not(gs.IsNil))
		})

		c.Specify("that is started", func() {
			server := httptest.NewServer(handler)
			defer server.Close()

			runOutput := func() {
				httpOutput.Run(oth.MockOutputRunner, oth.MockHelper)
				runWg.Done()
			}

			oth.MockOutputRunner.EXPECT().Encoder().Return(encoder)
			oth.MockOutputRunner.EXPECT().InChan().Return(inChan)
			oth.MockOutputRunner.EXPECT().UpdateCursor("").AnyTimes()
			payload := "this is the payload"
			pack.Message.SetPayload(payload)
			oth.MockOutputRunner.EXPECT().Encode(gomock.Any()).Return(
				[]byte(payload), nil)
			config.Address = server.URL
			handler.respBody = "Response Body"

			c.Specify("makes http POST requests by default", func() {
				err := httpOutput.Init(config)
				c.Expect(err, gs.IsNil)
				runWg.Add(1)
				go runOutput()
				handleWg.Add(1)
				inChan <- pack
				close(inChan)
				handleWg.Wait()
				runWg.Wait()
				c.Expect(reqBody, gs.Equals, payload)
				c.Expect(reqMethod, gs.Equals, "POST")
			})

			c.Specify("makes http PUT requests", func() {
				config.Method = "put"
				err := httpOutput.Init(config)
				c.Expect(err, gs.IsNil)
				runWg.Add(1)
				go runOutput()
				handleWg.Add(1)
				inChan <- pack
				close(inChan)
				handleWg.Wait()
				runWg.Wait()
				c.Expect(reqBody, gs.Equals, payload)
				c.Expect(reqMethod, gs.Equals, "PUT")
			})

			c.Specify("makes http GET requests", func() {
				config.Method = "get"
				err := httpOutput.Init(config)
				c.Expect(err, gs.IsNil)
				runWg.Add(1)
				go runOutput()
				handleWg.Add(1)
				inChan <- pack
				close(inChan)
				handleWg.Wait()
				runWg.Wait()
				c.Expect(reqBody, gs.Equals, "")
				c.Expect(reqMethod, gs.Equals, "GET")
			})

			c.Specify("correctly passes headers along", func() {
				config.Headers = http.Header{
					"One":  []string{"two", "three"},
					"Four": []string{"five", "six", "seven"},
				}
				err := httpOutput.Init(config)
				c.Expect(err, gs.IsNil)
				runWg.Add(1)
				go runOutput()
				handleWg.Add(1)
				inChan <- pack
				close(inChan)
				handleWg.Wait()
				runWg.Wait()
				c.Expect(reqBody, gs.Equals, payload)
				c.Expect(len(reqHeader["One"]), gs.Equals, len(config.Headers["One"]))
				c.Expect(len(reqHeader["Four"]), gs.Equals, len(config.Headers["Four"]))
			})

			c.Specify("uses http auth when specified", func() {
				config.Username = "user"
				config.Password = "pass"
				err := httpOutput.Init(config)
				c.Expect(err, gs.IsNil)
				runWg.Add(1)
				go runOutput()
				handleWg.Add(1)
				inChan <- pack
				close(inChan)
				handleWg.Wait()
				runWg.Wait()
				auth := reqHeader.Get("Authorization")
				c.Expect(strings.HasPrefix(auth, "Basic "), gs.IsTrue)
				decodedAuth, err := base64.StdEncoding.DecodeString(auth[6:])
				c.Expect(err, gs.IsNil)
				c.Expect(string(decodedAuth), gs.Equals, "user:pass")
			})

			c.Specify("logs error responses", func() {
				handler.respBody = ""
				handler.respCode = 500
				err := httpOutput.Init(config)
				c.Expect(err, gs.IsNil)

				pack.BufferedPack = true
				pack.DelivErrChan = make(chan error, 1)
				runWg.Add(1)
				go runOutput()
				handleWg.Add(1)
				inChan <- pack
				close(inChan)
				handleWg.Wait()
				runWg.Wait()
				e := <-pack.DelivErrChan
				c.Expect(strings.HasPrefix(e.Error(),
					"HTTP Error code returned: 500"), gs.IsTrue)
			})

			c.Specify("honors http timeout interval", func() {
				config.HttpTimeout = 1 // 1 millisecond
				err := httpOutput.Init(config)
				c.Expect(err, gs.IsNil)

				pack.BufferedPack = true
				pack.DelivErrChan = make(chan error, 1)
				delay = true
				runWg.Add(1)
				go runOutput()
				handleWg.Add(1)
				inChan <- pack
				close(inChan)
				handleWg.Wait()
				runWg.Wait()
				e := <-pack.DelivErrChan
				c.Expect(strings.Contains(e.Error(), "use of closed network connection"),
					gs.IsTrue)
			})
		})
	})
}
