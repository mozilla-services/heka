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
	"fmt"
	. "github.com/mozilla-services/heka/pipeline"
	"io/ioutil"
	"net/http"
	"time"
)

type HttpInput struct {
	dataChan    chan []byte
	failChan    chan []byte
	stopChan    chan bool
	Monitor     *HttpInputMonitor
	decoderName string
}

// Http Input config struct
type HttpInputConfig struct {
	// URLs for Input to consume.
	URLs []string
	// Default interval at which dashboard will update is 10 seconds.
	TickerInterval uint `toml:"ticker_interval"`
	// Name of configured decoder instance used to decode the messages.
	DecoderName string `toml:"decoder"`
}

func (hi *HttpInput) ConfigStruct() interface{} {
	return &HttpInputConfig{
		TickerInterval: uint(10),
	}
}

func (hi *HttpInput) Init(config interface{}) error {
	conf := config.(*HttpInputConfig)

	if conf.URLs == nil {
		return fmt.Errorf("urls must contain at least one URL")
	}

	hi.dataChan = make(chan []byte)
	hi.failChan = make(chan []byte)
	hi.stopChan = make(chan bool)
	hi.decoderName = conf.DecoderName
	hi.Monitor = new(HttpInputMonitor)
	hi.Monitor.Init(conf.URLs, hi.dataChan, hi.failChan, hi.stopChan)

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

	if hi.decoderName == "" {
		router_shortcircuit = true
	} else if dRunner, ok = h.DecoderRunner(hi.decoderName); !ok {
		return fmt.Errorf("Decoder not found: %s", hi.decoderName)
	}

	for {
		select {
		case data := <-hi.dataChan:
			pack = <-packSupply
			pack.Message.SetTimestamp(time.Now().UnixNano())
			pack.Message.SetType("heka.httpdata")
			pack.Message.SetHostname(hostname)
			pack.Message.SetPayload(string(data))
			pack.Message.SetSeverity(int32(6))
			pack.Message.SetLogger("HttpInput")
			if router_shortcircuit {
				pConfig.Router().InChan() <- pack
			} else {
				dRunner.InChan() <- pack
			}
		case data := <-hi.failChan:
			pack = <-packSupply
			pack.Message.SetTimestamp(time.Now().UnixNano())
			pack.Message.SetType("heka.httpdata.error")
			pack.Message.SetPayload(string(data))
			pack.Message.SetSeverity(int32(1))
			pack.Message.SetLogger("HttpInput")
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
	dataChan chan []byte
	failChan chan []byte
	stopChan chan bool

	ir       InputRunner
	tickChan <-chan time.Time
}

func (hm *HttpInputMonitor) Init(urls []string, dataChan chan []byte, failChan chan []byte, stopChan chan bool) {
	hm.urls = urls
	hm.dataChan = dataChan
	hm.failChan = failChan
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
				// Fetch URLs
				resp, err := http.Get(url)
				if err != nil {
					ir.LogError(fmt.Errorf("[HttpInputMonitor] %s", err))
					hm.failChan <- []byte(err.Error())
					continue
				}

				// Read content
				body, err := ioutil.ReadAll(resp.Body)
				if err != nil {
					ir.LogError(fmt.Errorf("[HttpInputMonitor] [%s]", err.Error()))
					continue
				}
				resp.Body.Close()

				if resp.StatusCode != 200 {
					hm.failChan <- body
				} else {
					hm.dataChan <- body
				}

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
