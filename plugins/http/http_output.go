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
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/mozilla-services/heka/pipeline"
	"github.com/mozilla-services/heka/plugins/tcp"
)

type HttpOutput struct {
	*HttpOutputConfig
	url          *url.URL
	client       *http.Client
	useBasicAuth bool
	sendBody     bool
}

type HttpOutputConfig struct {
	HttpTimeout uint32 `toml:"http_timeout"`
	Address     string
	Method      string
	Headers     http.Header
	Username    string `toml:"username"`
	Password    string `toml:"password"`
	Tls         tcp.TlsConfig
}

func (o *HttpOutput) ConfigStruct() interface{} {
	return &HttpOutputConfig{
		HttpTimeout: 0,
		Headers:     make(http.Header),
		Method:      "POST",
	}
}

func (o *HttpOutput) Init(config interface{}) (err error) {
	o.HttpOutputConfig = config.(*HttpOutputConfig)
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
	if o.url.Scheme == "https" {
		transport := &http.Transport{}
		if transport.TLSClientConfig, err = tcp.CreateGoTlsConfig(&o.Tls); err != nil {
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

	var (
		e        error
		outBytes []byte
	)
	inChan := or.InChan()

	for pack := range inChan {
		outBytes, e = or.Encode(pack)
		if e != nil {
			or.UpdateCursor(pack.QueueCursor)
			pack.Recycle(fmt.Errorf("can't encode: %s", e))
			continue
		}
		if outBytes == nil {
			or.UpdateCursor(pack.QueueCursor)
			pack.Recycle(nil)
			continue
		}
		if e = o.request(or, outBytes); e != nil {
			e = pipeline.NewRetryMessageError(e.Error())
			pack.Recycle(e)
		} else {
			or.UpdateCursor(pack.QueueCursor)
			pack.Recycle(nil)
		}
	}

	return
}

func (o *HttpOutput) request(or pipeline.OutputRunner, outBytes []byte) (err error) {
	var (
		resp       *http.Response
		reader     io.Reader
		readCloser io.ReadCloser
	)

	req := &http.Request{
		Method: o.Method,
		URL:    o.url,
		Header: o.Headers,
	}
	if o.useBasicAuth {
		req.SetBasicAuth(o.Username, o.Password)
	}

	if o.sendBody {
		req.ContentLength = int64(len(outBytes))
		reader = bytes.NewReader(outBytes)
		readCloser = ioutil.NopCloser(reader)
		req.Body = readCloser
	}
	if resp, err = o.client.Do(req); err != nil {
		return fmt.Errorf("Error making HTTP request: %s", err.Error())
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return fmt.Errorf("Error reading HTTP response: %s", err.Error())
		}
		return fmt.Errorf("HTTP Error code returned: %d %s - %s",
			resp.StatusCode, resp.Status, string(body))
	}
	return
}

func init() {
	pipeline.RegisterPlugin("HttpOutput", func() interface{} {
		return new(HttpOutput)
	})
}
