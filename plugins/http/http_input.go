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
#   David Delassus (david.jose.delassus@gmail.com)
#   Victor Ng (vng@mozilla.com)
#   Christian Vozar (christian@bellycard.com)
#
# ***** END LICENSE BLOCK *****/

package http

import (
	"code.google.com/p/go-uuid/uuid"
	"fmt"
	"github.com/mozilla-services/heka/message"
	. "github.com/mozilla-services/heka/pipeline"
	"io/ioutil"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type HttpInput struct {
	name     string
	urls     []string
	respChan chan *MonitorResponse
	errChan  chan *MonitorResponse
	stopChan chan bool
	Monitor  *HttpInputMonitor
	conf     *HttpInputConfig
}

type MonitorResponse struct {
	ResponseData []byte
	ResponseSize int
	ResponseTime float64
	StatusCode   int
	Status       string
	Proto        string
	Url          string
}

// Http Input config struct
type HttpInputConfig struct {
	// Url for HttpInput to request.
	Url string
	// Urls for HttpInput to request.
	Urls []string
	// Request method. Default is "GET"
	Method string
	// Request headers
	Headers map[string]string
	// Request body for POST
	Body string
	// User and password for Basic Authentication
	User     string
	Password string
	// Default interval at which http.Get will execute. Default is 10 seconds.
	TickerInterval uint `toml:"ticker_interval"`
	// Configured decoder instance used to decode http.Get payload.
	DecoderName string `toml:"decoder"`
	// Severity level of successful requests. Default is 6 (information)
	SuccessSeverity int32 `toml:"success_severity"`
	// Severity level of errors and unsuccessful requests. Default is 1 (alert)
	ErrorSeverity int32 `toml:"error_severity"`
}

func (hi *HttpInput) SetName(name string) {
	hi.name = name
}

func (hi *HttpInput) ConfigStruct() interface{} {
	return &HttpInputConfig{
		Method:          "GET",
		TickerInterval:  uint(10),
		SuccessSeverity: int32(6),
		ErrorSeverity:   int32(1),
	}
}

func (hi *HttpInput) Init(config interface{}) error {
	hi.conf = config.(*HttpInputConfig)

	if (hi.conf.Urls == nil) && (hi.conf.Url == "") {
		return fmt.Errorf("Url or Urls must contain at least one URL")
	}

	if hi.conf.Urls != nil {
		hi.urls = hi.conf.Urls
	} else {
		hi.urls = []string{hi.conf.Url}
	}

	hi.respChan = make(chan *MonitorResponse)
	hi.errChan = make(chan *MonitorResponse)
	hi.stopChan = make(chan bool)
	hi.Monitor = new(HttpInputMonitor)
	hi.Monitor.Init(hi.urls, hi.conf, hi.respChan, hi.errChan, hi.stopChan)

	return nil
}

func (hi *HttpInput) Run(ir InputRunner, h PluginHelper) (err error) {
	var (
		pack                *PipelinePack
		dRunner             DecoderRunner
		ok                  bool
		router_shortcircuit bool
	)

	ir.LogMessage(fmt.Sprintf("[HttpInput (%s)] Running...",
		hi.Monitor.urls))

	go hi.Monitor.Monitor(ir)

	pConfig := h.PipelineConfig()
	hostname := pConfig.Hostname()
	packSupply := ir.InChan()

	if hi.conf.DecoderName == "" {
		router_shortcircuit = true
	} else if dRunner, ok = h.DecoderRunner(hi.conf.DecoderName, fmt.Sprintf("%s-%s", ir.Name(), hi.conf.DecoderName)); !ok {
		return fmt.Errorf("Decoder not found: %s", hi.conf.DecoderName)
	}

	for {
		select {
		case data := <-hi.respChan:
			pack = <-packSupply
			pack.Message.SetUuid(uuid.NewRandom())
			pack.Message.SetTimestamp(time.Now().UnixNano())
			pack.Message.SetType("heka.httpinput.data")
			pack.Message.SetHostname(hostname)
			pack.Message.SetPayload(string(data.ResponseData))
			if data.StatusCode != 200 {
				pack.Message.SetSeverity(hi.conf.ErrorSeverity)
			} else {
				pack.Message.SetSeverity(hi.conf.SuccessSeverity)
			}
			pack.Message.SetLogger(data.Url)
			if field, err := message.NewField("StatusCode", data.StatusCode, ""); err == nil {
				pack.Message.AddField(field)
			} else {
				ir.LogError(fmt.Errorf("can't add field: %s", err))
			}
			if field, err := message.NewField("Status", data.Status, ""); err == nil {
				pack.Message.AddField(field)
			} else {
				ir.LogError(fmt.Errorf("can't add field: %s", err))
			}
			if field, err := message.NewField("ResponseSize", data.ResponseSize, "B"); err == nil {
				pack.Message.AddField(field)
			} else {
				ir.LogError(fmt.Errorf("can't add field: %s", err))
			}
			if field, err := message.NewField("ResponseTime", data.ResponseTime, "s"); err == nil {
				pack.Message.AddField(field)
			} else {
				ir.LogError(fmt.Errorf("can't add field: %s", err))
			}
			if field, err := message.NewField("Protocol", data.Proto, ""); err == nil {
				pack.Message.AddField(field)
			} else {
				ir.LogError(fmt.Errorf("can't add field: %s", err))
			}
			if router_shortcircuit {
				pConfig.Router().InChan() <- pack
			} else {
				dRunner.InChan() <- pack
			}
		case data := <-hi.errChan:
			pack = <-packSupply
			pack.Message.SetUuid(uuid.NewRandom())
			pack.Message.SetTimestamp(time.Now().UnixNano())
			pack.Message.SetType("heka.httpinput.error")
			pack.Message.SetPayload(string(data.ResponseData))
			pack.Message.SetSeverity(hi.conf.ErrorSeverity)
			pack.Message.SetLogger(data.Url)
			if router_shortcircuit {
				pConfig.Router().InChan() <- pack
			} else {
				dRunner.InChan() <- pack
			}
		case <-hi.stopChan:
			return
		}
	}

	return nil
}

func (hi *HttpInput) Stop() {
	close(hi.stopChan)
}

type HttpInputMonitor struct {
	urls     []string
	method   string
	headers  map[string]string
	body     string
	user     string
	password string
	respChan chan *MonitorResponse
	errChan  chan *MonitorResponse
	stopChan chan bool

	ir       InputRunner
	tickChan <-chan time.Time
}

func (hm *HttpInputMonitor) Init(urls []string, config *HttpInputConfig, respChan, errChan chan *MonitorResponse, stopChan chan bool) {
	hm.urls = urls
	hm.method = config.Method
	hm.headers = config.Headers
	hm.body = config.Body
	hm.user = config.User
	hm.password = config.Password
	hm.respChan = respChan
	hm.errChan = errChan
	hm.stopChan = stopChan
}

func (hm *HttpInputMonitor) Monitor(ir InputRunner) {
	ir.LogMessage("[HttpInputMonitor] Monitoring...")

	hm.ir = ir
	hm.tickChan = ir.Ticker()

	for {
		select {
		case <-hm.tickChan:
			for _, url := range hm.urls {
				responsePayload := []byte{}
				responseTimeStart := time.Now()
				// Request URL(s)
				httpClient := &http.Client{}
				req, err := http.NewRequest(hm.method, url, strings.NewReader(hm.body))

				// HTTP Basic Authentication
				if hm.user != "" {
					req.SetBasicAuth(hm.user, hm.password)
				}

				// Request headers
				req.Header.Add("User-Agent", "Heka")
				for key, value := range hm.headers {
					req.Header.Add(key, value)
				}

				resp, err := httpClient.Do(req)

				responseTime := time.Since(responseTimeStart)
				if err != nil {
					responsePayload = []byte(err.Error())
					response := &MonitorResponse{ResponseData: responsePayload, Url: url}
					hm.errChan <- response
					continue
				}

				// Consume HTTP response body
				body, err := ioutil.ReadAll(resp.Body)
				if err != nil {
					ir.LogError(fmt.Errorf("[HttpInputMonitor] [%s]", err.Error()))
					continue
				}
				resp.Body.Close()

				contentLength, _ := strconv.Atoi(resp.Header.Get("Content-Length"))

				response := &MonitorResponse{
					ResponseData: body,
					ResponseSize: contentLength,
					ResponseTime: responseTime.Seconds(),
					StatusCode:   resp.StatusCode,
					Status:       resp.Status,
					Proto:        resp.Proto,
					Url:          url,
				}
				hm.respChan <- response
			}
		case <-hm.stopChan:
			ir.LogMessage(fmt.Sprintf("[HttpInputMonitor (%s)] Stop", hm.urls))
			return
		}
	}
}

func init() {
	RegisterPlugin("HttpInput", func() interface{} {
		return new(HttpInput)
	})
}
