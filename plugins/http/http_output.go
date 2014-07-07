/***** BEGIN LICENSE BLOCK *****
# This Source Code Form is subject to the terms of the Mozilla Public
# License, v. 2.0. If a copy of the MPL was not distributed with this file,
# You can obtain one at http://mozilla.org/MPL/2.0/.
#
# The Initial Developer of the Original Code is the Mozilla Foundation.
# Portions created by the Initial Developer are Copyright (C) 2014
# the Initial Developer. All Rights Reserved.
#
# Contributor(s):
#   Rob Miller (rmiller@mozilla.com)
#
# ***** END LICENSE BLOCK *****/

package http

import (
	"bytes"
	"errors"
	"fmt"
	"github.com/mozilla-services/heka/pipeline"
	"github.com/mozilla-services/heka/plugins/tcp"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

type HttpOutput struct {
	*HttpOutputConfig
	url          *url.URL
	client       *http.Client
	batchChan    chan []byte
	backChan     chan []byte
	useBasicAuth bool
	sendBody     bool
}

type HttpOutputConfig struct {
	HttpTimeout   uint32 `toml:"http_timeout"`
	FlushInterval uint32 `toml:"flush_interval"`
	FlushCount    uint32 `toml:"flush_count"`
	Address       string
	Method        string
	Headers       http.Header
	Username      string `toml:"username"`
	Password      string `toml:"password"`
	Tls           *tcp.TlsConfig
}

func (o *HttpOutput) ConfigStruct() interface{} {
	return &HttpOutputConfig{
		FlushInterval: 0,
		FlushCount:    1,
		HttpTimeout:   0,
		Headers:       make(http.Header),
		Method:        "POST",
	}
}

func (o *HttpOutput) Init(config interface{}) (err error) {
	o.HttpOutputConfig = config.(*HttpOutputConfig)
	o.batchChan = make(chan []byte)
	o.backChan = make(chan []byte, 2)
	if o.FlushInterval == 0 && o.FlushCount == 0 {
		return errors.New("`flush_count` and `flush_interval` cannot both be zero.")
	}
	if o.url, err = url.Parse(o.Address); err != nil {
		return fmt.Errorf("Can't parse URL '%s': %s", o.Address, err.Error())
	}
	if o.url.Scheme != "http" && o.url.Scheme != "https" {
		return errors.New("`address` must contain an absolute http or https URL.")
	}
	o.Method = strings.ToUpper(o.Method)
	if o.Method != "POST" && o.Method != "GET" && o.Method != "PUT" {
		return errors.New("HTTP Method must be POST, GET, or PUT.")
	}
	if o.Method != "GET" {
		o.sendBody = true
	}
	o.client = new(http.Client)
	if o.HttpTimeout > 0 {
		o.client.Timeout = time.Duration(o.HttpTimeout) * time.Millisecond
	}
	if o.Username != "" || o.Password != "" {
		o.useBasicAuth = true
	}
	if o.url.Scheme == "https" && o.Tls != nil {
		transport := &http.Transport{}
		if transport.TLSClientConfig, err = tcp.CreateGoTlsConfig(o.Tls); err != nil {
			return fmt.Errorf("TLS init error: %s", err.Error())
		}
		o.client.Transport = transport
	}
	return
}

func (o *HttpOutput) Run(or pipeline.OutputRunner, h pipeline.PluginHelper) (err error) {
	if or.Encoder() == nil {
		return errors.New("Encoder must be specified.")
	}
	// Create waitgroup and launch the committer goroutine.
	var wg sync.WaitGroup
	wg.Add(1)
	go o.committer(or, &wg)

	var (
		pack     *pipeline.PipelinePack
		e        error
		count    uint32
		outBytes []byte
	)
	ok := true
	ticker := time.Tick(time.Duration(o.FlushInterval) * time.Millisecond)
	outBatch := make([]byte, 0, 10000)
	inChan := or.InChan()

	for ok {
		select {
		case pack, ok = <-inChan:
			if !ok {
				// Closed inChan => we're shutting down, flush data.
				if len(outBatch) > 0 {
					o.batchChan <- outBatch
				}
				close(o.batchChan)
				break
			}
			outBytes, e = or.Encode(pack)
			pack.Recycle()
			if e != nil {
				or.LogError(e)
			} else {
				outBatch = append(outBatch, outBytes...)
				if count = count + 1; o.FlushCount > 0 && count >= o.FlushCount {
					if len(outBatch) > 0 {
						// This will block until the other side is ready to
						// accept this batch, so we can't get too far ahead.
						o.batchChan <- outBatch
						outBatch = <-o.backChan
						count = 0
					}
				}
			}
		case <-ticker:
			if len(outBatch) > 0 {
				// This will block until the other side is ready to accept
				// this batch, freeing us to start on the next one.
				o.batchChan <- outBatch
				outBatch = <-o.backChan
				count = 0
			}
		}
	}

	// Wait for the committer to finish before we exit.
	wg.Wait()
	return
}

func (o *HttpOutput) committer(or pipeline.OutputRunner, wg *sync.WaitGroup) {
	initBatch := make([]byte, 0, 10000)
	o.backChan <- initBatch
	var (
		outBatch   []byte
		req        *http.Request
		resp       *http.Response
		err        error
		reader     io.Reader
		readCloser io.ReadCloser
	)

	req = &http.Request{
		Method: o.Method,
		URL:    o.url,
		Header: o.Headers,
	}
	if o.useBasicAuth {
		req.SetBasicAuth(o.Username, o.Password)
	}

	for outBatch = range o.batchChan {
		if o.sendBody {
			req.ContentLength = int64(len(outBatch))
			reader = bytes.NewReader(outBatch)
			readCloser = ioutil.NopCloser(reader)
			req.Body = readCloser
		}
		if resp, err = o.client.Do(req); err != nil {
			or.LogError(fmt.Errorf("Error making HTTP request: %s", err.Error()))
			continue
		}
		if resp.StatusCode >= 400 {
			var body []byte
			if resp.ContentLength > 0 {
				body = make([]byte, resp.ContentLength)
				resp.Body.Read(body)
			}
			or.LogError(fmt.Errorf("HTTP Error code returned: %d %s - %s",
				resp.StatusCode, resp.Status, string(body)))
			continue
		}
		outBatch = outBatch[:0]
		o.backChan <- outBatch
	}
	wg.Done()
}

func init() {
	pipeline.RegisterPlugin("HttpOutput", func() interface{} {
		return new(HttpOutput)
	})
}
