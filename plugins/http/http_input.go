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
	"time"
)

type HttpInput struct {
	name     string
	urls     []string
	respChan chan MonitorResponse
	errChan  chan MonitorResponse
	stopChan chan bool
	Monitor  *HttpInputMonitor
	conf     *HttpInputConfig
}

type MonitorResponse struct {
	ResponseData []byte
	StatusCode   int
	Status       string
	Proto        string
	Url          string
}

// Http Input config struct
type HttpInputConfig struct {
	// Url for HttpInput to GET.
	Url string
	// Urls for HttpInput to GET.
	Urls []string
	// Default interval at which http.Get will execute. Default is 10 seconds.
	TickerInterval uint `toml:"ticker_interval"`
	// Configured decoder instance used to decode http.Get payload.
	DecoderName string `toml:"decoder"`
	// Severity level of successful GETs. Default is 6 (information)
	SuccessSeverity int32 `toml:"success_severity"`
	// Severity level of errors and unsuccessful GETs. Default is 1 (alert)
	ErrorSeverity int32 `toml:"error_severity"`
}

func (hi *HttpInput) SetName(name string) {
	hi.name = name
}

func (hi *HttpInput) ConfigStruct() interface{} {
	return &HttpInputConfig{
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

	hi.respChan = make(chan MonitorResponse)
	hi.errChan = make(chan MonitorResponse)
	hi.stopChan = make(chan bool)
	hi.Monitor = new(HttpInputMonitor)
	hi.Monitor.Init(hi.urls, hi.respChan, hi.errChan, hi.stopChan)

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
	} else if dRunner, ok = h.DecoderRunner(hi.conf.DecoderName); !ok {
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
	respChan chan MonitorResponse
	errChan  chan MonitorResponse
	stopChan chan bool

	ir       InputRunner
	tickChan <-chan time.Time
}

func (hm *HttpInputMonitor) Init(urls []string, respChan chan MonitorResponse, errChan chan MonitorResponse, stopChan chan bool) {
	hm.urls = urls
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
				// Request URL(s)
				resp, err := http.Get(url)
				if err != nil {
					responsePayload = []byte(err.Error())
					response := MonitorResponse{ResponseData: responsePayload, Url: url}
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

				response := MonitorResponse{ResponseData: body, StatusCode: resp.StatusCode, Status: resp.Status, Proto: resp.Proto, Url: url}
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
