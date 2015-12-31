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
#   David Delassus (david.jose.delassus@gmail.com)
#   Victor Ng (vng@mozilla.com)
#   Christian Vozar (christian@bellycard.com)
#   Rob Miller (rmiller@mozilla.com)
#
# ***** END LICENSE BLOCK *****/

package http

import (
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/mozilla-services/heka/message"
	. "github.com/mozilla-services/heka/pipeline"
	"github.com/pborman/uuid"
)

type ResponseData struct {
	Time       float64
	Size       int
	StatusCode int
	Status     string
	Proto      string
	Url        string
}

type HttpInput struct {
	name            string
	urls            []string
	stopChan        chan bool
	conf            *HttpInputConfig
	ir              InputRunner
	sRunners        []SplitterRunner
	hostname        string
	packSupply      chan *PipelinePack
	customUserAgent bool
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
	hi.stopChan = make(chan bool)

	// Check to see if a custom user-agent is in use.
	h := make(http.Header)
	for key, value := range hi.conf.Headers {
		h.Add(key, value)
	}
	if h.Get("User-Agent") != "" {
		hi.customUserAgent = true
	}

	return nil
}

func (hi *HttpInput) addField(pack *PipelinePack, name string, value interface{},
	representation string) {

	if field, err := message.NewField(name, value, representation); err == nil {
		pack.Message.AddField(field)
	} else {
		hi.ir.LogError(fmt.Errorf("can't add '%s' field: %s", name, err.Error()))
	}
}

func (hi *HttpInput) makePackDecorator(respData ResponseData) func(*PipelinePack) {
	packDecorator := func(pack *PipelinePack) {
		pack.Message.SetType("heka.httpinput.data")
		pack.Message.SetHostname(hi.hostname)
		if respData.StatusCode != 200 {
			pack.Message.SetSeverity(hi.conf.ErrorSeverity)
		} else {
			pack.Message.SetSeverity(hi.conf.SuccessSeverity)
		}
		pack.Message.SetLogger(respData.Url)
		hi.addField(pack, "StatusCode", respData.StatusCode, "")
		hi.addField(pack, "Status", respData.Status, "")
		hi.addField(pack, "ResponseSize", respData.Size, "B")
		hi.addField(pack, "ResponseTime", respData.Time, "s")
		hi.addField(pack, "Protocol", respData.Proto, "")
	}
	return packDecorator
}

func (hi *HttpInput) fetchUrl(url string, sRunner SplitterRunner) {
	responseTimeStart := time.Now()
	httpClient := &http.Client{}
	req, err := http.NewRequest(hi.conf.Method, url, strings.NewReader(hi.conf.Body))
	if err != nil {
		hi.ir.LogError(fmt.Errorf("can't create HTTP request for %s: %s", url, err.Error()))
		return
	}
	// HTTP Basic Auth
	if hi.conf.User != "" {
		req.SetBasicAuth(hi.conf.User, hi.conf.Password)
	}
	// Request headers
	for key, value := range hi.conf.Headers {
		req.Header.Add(key, value)
	}
	if !hi.customUserAgent {
		req.Header.Add("User-Agent", "Heka")
	}
	resp, err := httpClient.Do(req)
	responseTime := time.Since(responseTimeStart)
	if err != nil {
		pack := <-hi.ir.InChan()
		pack.Message.SetUuid(uuid.NewRandom())
		pack.Message.SetTimestamp(time.Now().UnixNano())
		pack.Message.SetType("heka.httpinput.error")
		pack.Message.SetPayload(err.Error())
		pack.Message.SetSeverity(hi.conf.ErrorSeverity)
		pack.Message.SetLogger(url)
		hi.ir.Deliver(pack)
		return
	}
	contentLength, _ := strconv.Atoi(resp.Header.Get("Content-Length"))
	respData := ResponseData{
		Size:       contentLength,
		Time:       responseTime.Seconds(),
		StatusCode: resp.StatusCode,
		Status:     resp.Status,
		Proto:      resp.Proto,
		Url:        url,
	}

	if !sRunner.UseMsgBytes() {
		sRunner.SetPackDecorator(hi.makePackDecorator(respData))
	}

	err = sRunner.SplitStreamNullSplitterToEOF(resp.Body, nil)
	if err != nil && err != io.EOF {
		hi.ir.LogError(fmt.Errorf("fetching %s response input: %s", url, err.Error()))
	}
	resp.Body.Close()
}

func (hi *HttpInput) Run(ir InputRunner, h PluginHelper) (err error) {
	hi.ir = ir
	hi.sRunners = make([]SplitterRunner, len(hi.urls))
	hi.hostname = h.Hostname()

	for i, _ := range hi.urls {
		token := strconv.Itoa(i)
		hi.sRunners[i] = ir.NewSplitterRunner(token)
	}

	defer func() {
		for _, sRunner := range hi.sRunners {
			sRunner.Done()
		}
	}()

	ticker := ir.Ticker()
	for {
		select {
		case <-ticker:
			for i, url := range hi.urls {
				sRunner := hi.sRunners[i]
				hi.fetchUrl(url, sRunner)
			}
		case <-hi.stopChan:
			return nil
		}
	}
}

func (hi *HttpInput) Stop() {
	close(hi.stopChan)
}

func init() {
	RegisterPlugin("HttpInput", func() interface{} {
		return new(HttpInput)
	})
}
