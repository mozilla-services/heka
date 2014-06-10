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
	dRunner     DecoderRunner
	pConfig     *PipelineConfig
	decoderName string
}

// HTTP Listen Input config struct
type HttpListenInputConfig struct {
	// TCP Address to listen to for SNS notifications.
	// Defaults to "0.0.0.0:8325".
	Address string
	// Name of configured decoder instance used to decode the messages.
	// Defaults to request body as payload.
	Decoder string
}

func (hli *HttpListenInput) ConfigStruct() interface{} {
	return &HttpListenInputConfig{
		Address: "127.0.0.1:8325",
	}
}

func (hli *HttpListenInput) RequestHandler(w http.ResponseWriter, req *http.Request) {

	body, err := ioutil.ReadAll(req.Body)
	if err != nil {
		fmt.Errorf("[HttpListenInput] Read HTTP request body fail: %s\n", err.Error())
	}
	req.Body.Close()

	unEscapedBody, _ := url.QueryUnescape(string(body))

	pack := <-hli.ir.InChan()
	pack.Message.SetUuid(uuid.NewRandom())
	pack.Message.SetTimestamp(time.Now().UnixNano())
	pack.Message.SetType("heka.httpdata.request")
	pack.Message.SetLogger(hli.ir.Name())
	pack.Message.SetHostname(req.RemoteAddr)
	pack.Message.SetPid(int32(os.Getpid()))
	pack.Message.SetSeverity(int32(6))
	pack.Message.SetPayload(unEscapedBody)
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

	if hli.dRunner == nil {
		hli.ir.Inject(pack)
	} else {
		hli.dRunner.InChan() <- pack
	}
}

func (hli *HttpListenInput) Init(config interface{}) (err error) {
	hli.stopChan = make(chan bool, 1)

	hli.conf = config.(*HttpListenInputConfig)
	hli.decoderName = hli.conf.Decoder

	return nil
}

func (hli *HttpListenInput) Run(ir InputRunner, h PluginHelper) (err error) {
	var ok bool

	hli.ir = ir
	hli.pConfig = h.PipelineConfig()

	if hli.decoderName != "" {
		if hli.dRunner, ok = h.DecoderRunner(hli.decoderName, fmt.Sprintf("%s-%s", ir.Name(), hli.decoderName)); !ok {
			return fmt.Errorf("Decoder not found: %s", hli.decoderName)
		}
	}

	hliEndpointMux := http.NewServeMux()
	hli.listener, err = net.Listen("tcp", hli.conf.Address)
	if err != nil {
		return fmt.Errorf("[HttpListenInput] Listener [%s] start fail: %s\n",
			hli.conf.Address, err.Error())
	} else {
		hli.ir.LogMessage(fmt.Sprintf("[HttpListenInput (%s)] Listening.",
			hli.conf.Address))
	}

	hliEndpointMux.HandleFunc("/", hli.RequestHandler)
	err = http.Serve(hli.listener, hliEndpointMux)
	if err != nil {
		return fmt.Errorf("[HttpListenInput] Serve fail: %s\n", err.Error())
	}

	<-hli.stopChan

	return nil
}

func (hli *HttpListenInput) Stop() {
	hli.listener.Close()
	close(hli.stopChan)
}

func init() {
	RegisterPlugin("HttpListenInput", func() interface{} {
		return new(HttpListenInput)
	})
}
