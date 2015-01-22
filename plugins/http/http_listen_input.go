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
#   Christian Vozar (christian@bellycard.com)
#   Rob Miller (rmiller@mozilla.com)
#   Anton Lindstrom (carlantonlindstrom@gmail.com)
#
# ***** END LICENSE BLOCK *****/

package http

import (
	"code.google.com/p/go-uuid/uuid"
	"fmt"
	"github.com/mozilla-services/heka/message"
	. "github.com/mozilla-services/heka/pipeline"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"os"
	"time"
)

type HttpListenInput struct {
	conf        *HttpListenInputConfig
	listener    net.Listener
	stopChan    chan bool
	ir          InputRunner
	pConfig     *PipelineConfig
	server      *http.Server
	starterFunc func(hli *HttpListenInput) error
}

// HTTP Listen Input config struct
type HttpListenInputConfig struct {
	// TCP Address to listen to for SNS notifications.
	// Defaults to "0.0.0.0:8325".
	Address        string
	Headers        http.Header
	RequestHeaders []string `toml:"request_headers"`
	UnescapeBody   bool     `toml:"unescape_body"`
}

func (hli *HttpListenInput) ConfigStruct() interface{} {
	return &HttpListenInputConfig{
		Address:        "127.0.0.1:8325",
		Headers:        make(http.Header),
		RequestHeaders: []string{},
		UnescapeBody:   true,
	}
}

func defaultStarter(hli *HttpListenInput) (err error) {
	hli.listener, err = net.Listen("tcp", hli.conf.Address)
	if err != nil {
		return fmt.Errorf("Listener [%s] start fail: %s",
			hli.conf.Address, err.Error())
	} else {
		hli.ir.LogMessage(fmt.Sprintf("Listening on %s",
			hli.conf.Address))
	}

	err = hli.server.Serve(hli.listener)
	if err != nil {
		return fmt.Errorf("Serve fail: %s", err.Error())
	}

	return nil
}

func (hli *HttpListenInput) RequestHandler(w http.ResponseWriter, req *http.Request) {
	body, err := ioutil.ReadAll(req.Body)
	if err != nil {
		hli.ir.LogError(fmt.Errorf("Read HTTP request body fail: %s\n", err.Error()))
	}
	req.Body.Close()

	pack := <-hli.ir.InChan()
	pack.Message.SetUuid(uuid.NewRandom())
	pack.Message.SetTimestamp(time.Now().UnixNano())
	pack.Message.SetType("heka.httpdata.request")
	pack.Message.SetLogger(hli.ir.Name())
	pack.Message.SetHostname(req.RemoteAddr)
	pack.Message.SetPid(int32(os.Getpid()))
	pack.Message.SetSeverity(int32(6))
	if hli.conf.UnescapeBody {
		unEscapedBody, _ := url.QueryUnescape(string(body))
		pack.Message.SetPayload(unEscapedBody)
	} else {
		pack.Message.SetPayload(string(body))
	}
	if field, err := message.NewField("Protocol", req.Proto, ""); err == nil {
		pack.Message.AddField(field)
	} else {
		hli.ir.LogError(fmt.Errorf("can't add field: %s", err))
	}
	if field, err := message.NewField("UserAgent", req.UserAgent(), ""); err == nil {
		pack.Message.AddField(field)
	} else {
		hli.ir.LogError(fmt.Errorf("can't add field: %s", err))
	}
	if field, err := message.NewField("ContentType", req.Header.Get("Content-Type"), ""); err == nil {
		pack.Message.AddField(field)
	} else {
		hli.ir.LogError(fmt.Errorf("can't add field: %s", err))
	}

	for _, key := range hli.conf.RequestHeaders {
		value := req.Header.Get(key)
		if len(value) == 0 {
			continue
		} else if field, err := message.NewField(key, value, ""); err == nil {
			pack.Message.AddField(field)
		} else {
			hli.ir.LogError(fmt.Errorf("can't add field: %s", err))
		}
	}

	for key, values := range req.URL.Query() {
		for i := range values {
			value := values[i]
			if field, err := message.NewField(key, value, ""); err == nil {
				pack.Message.AddField(field)
			} else {
				hli.ir.LogError(fmt.Errorf("can't add field: %s", err))
			}
		}
	}

	hli.ir.Deliver(pack)
}

func (hli *HttpListenInput) Init(config interface{}) (err error) {
	hli.conf = config.(*HttpListenInputConfig)
	if hli.starterFunc == nil {
		hli.starterFunc = defaultStarter
	}
	hli.stopChan = make(chan bool, 1)

	handler := http.HandlerFunc(hli.RequestHandler)
	hli.server = &http.Server{
		Handler: CustomHeadersHandler(handler, hli.conf.Headers),
	}

	return nil
}

func (hli *HttpListenInput) Run(ir InputRunner, h PluginHelper) (err error) {
	hli.ir = ir
	hli.pConfig = h.PipelineConfig()
	err = hli.starterFunc(hli)
	if err != nil {
		return err
	}

	<-hli.stopChan

	return nil
}

func (hli *HttpListenInput) Stop() {
	if hli.listener != nil {
		hli.listener.Close()
	}
	close(hli.stopChan)
}

func init() {
	RegisterPlugin("HttpListenInput", func() interface{} {
		return new(HttpListenInput)
	})
}
