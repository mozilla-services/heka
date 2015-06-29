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
#   Christian Vozar (christian@bellycard.com)
#   Rob Miller (rmiller@mozilla.com)
#   Anton Lindstrom (carlantonlindstrom@gmail.com)
#
# ***** END LICENSE BLOCK *****/

package http

import (
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"

	"github.com/mozilla-services/heka/message"
	. "github.com/mozilla-services/heka/pipeline"
	. "github.com/mozilla-services/heka/plugins/tcp"
)

type HttpListenInput struct {
	conf        *HttpListenInputConfig
	listener    net.Listener
	stopChan    chan bool
	ir          InputRunner
	server      *http.Server
	starterFunc func(hli *HttpListenInput) error
	hekaPid     int32
	hostname    string
}

// HTTP Listen Input config struct
type HttpListenInputConfig struct {
	// TCP Address to listen to for incoming requests.
	// Defaults to "127.0.0.1:8325".
	Address        string
	Headers        http.Header
	RequestHeaders []string `toml:"request_headers"`
	AuthType       string   `toml:"auth_type"`
	Username       string   `toml:"username"`
	Password       string   `toml:"password"`
	Key            string   `toml:"api_key"`
	// Set to true if the TCP connection should be tunneled through TLS.
	// Requires additional Tls config section.
	UseTls bool `toml:"use_tls"`
	// Subsection for TLS configuration.
	Tls TlsConfig
}

func (hli *HttpListenInput) ConfigStruct() interface{} {
	config := &HttpListenInputConfig{
		Address:        "127.0.0.1:8325",
		Headers:        make(http.Header),
		RequestHeaders: []string{},
	}
	config.Tls = TlsConfig{PreferServerCiphers: true}
	return config
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

	if hli.conf.UseTls {
		if err = hli.setupTls(&hli.conf.Tls); err != nil {
			return err
		}
	}

	err = hli.server.Serve(hli.listener)
	if err != nil {
		return fmt.Errorf("Serve fail: %s", err.Error())
	}

	return nil
}

func (hli *HttpListenInput) setupTls(tomlConf *TlsConfig) (err error) {
	if tomlConf.CertFile == "" || tomlConf.KeyFile == "" {
		return errors.New("TLS config requires both cert_file and key_file value.")
	}
	var goConf *tls.Config
	if goConf, err = CreateGoTlsConfig(tomlConf); err == nil {
		hli.listener = tls.NewListener(hli.listener, goConf)
	}
	return
}

func (hli *HttpListenInput) makeField(name string, value string) (field *message.Field, err error) {
	field, err = message.NewField(name, value, "")
	if err != nil {
		hli.ir.LogError(fmt.Errorf("can't add field %s: %s", name, err))
	}
	return
}

func (hli *HttpListenInput) makePackDecorator(req *http.Request) func(*PipelinePack) {
	packDecorator := func(pack *PipelinePack) {
		pack.Message.SetType("heka.httpdata.request")
		pack.Message.SetPid(hli.hekaPid)
		pack.Message.SetSeverity(int32(6))

		// Host on which heka is running.
		pack.Message.SetHostname(hli.hostname)
		pack.Message.SetEnvVersion("1")
		if field, err := hli.makeField("Protocol", req.Proto); err == nil {
			pack.Message.AddField(field)
		}
		if field, err := hli.makeField("UserAgent", req.UserAgent()); err == nil {
			pack.Message.AddField(field)
		}
		if field, err := hli.makeField("ContentType", req.Header.Get("Content-Type")); err == nil {
			pack.Message.AddField(field)
		}
		if field, err := hli.makeField("Path", req.URL.Path); err == nil {
			pack.Message.AddField(field)
		}

		// Host which the client requested.
		host, _, err := net.SplitHostPort(req.Host)
		if err != nil {
			// Fall back to the un-split value.
			host = req.Host
		}
		if field, err := hli.makeField("Host", host); err == nil {
			pack.Message.AddField(field)
		}

		host, _, err = net.SplitHostPort(req.RemoteAddr)
		if err != nil {
			// Fall back to the un-split value.
			host = req.RemoteAddr
		}
		if field, err := hli.makeField("RemoteAddr", host); err == nil {
			pack.Message.AddField(field)
		}
		for _, key := range hli.conf.RequestHeaders {
			value := req.Header.Get(key)
			if len(value) == 0 {
				continue
			} else if field, err := hli.makeField(key, value); err == nil {
				pack.Message.AddField(field)
			}
		}
		for key, values := range req.URL.Query() {
			for i := range values {
				value := values[i]
				if field, err := hli.makeField(key, value); err == nil {
					pack.Message.AddField(field)
				}
			}
		}
	}
	return packDecorator
}

func (hli *HttpListenInput) RequestHandler(w http.ResponseWriter, req *http.Request) {
	var err error

	if hli.conf.AuthType == "Basic" {
		if hli.conf.Username != "" && hli.conf.Password != "" {
			user, pass, ok := req.BasicAuth()
			if !ok || user != hli.conf.Username || pass != hli.conf.Password {
				err = fmt.Errorf("Basic Auth Failed")
				hli.ir.LogError(err)
			}
		}
	}
	if hli.conf.AuthType == "API" {
		if hli.conf.Key != "" {
			api_key := req.Header.Get("X-API-Key")
			if api_key != hli.conf.Key {
				err = fmt.Errorf("API Auth Failed")
				hli.ir.LogError(err)
			}
		}
	}
	if err == nil {
		sRunner := hli.ir.NewSplitterRunner(req.RemoteAddr)
		if !sRunner.UseMsgBytes() {
			sRunner.SetPackDecorator(hli.makePackDecorator(req))
		}
		err = splitStream(hli.ir, sRunner, req.Body)
		if err != nil && err != io.EOF {
			hli.ir.LogError(fmt.Errorf("receiving request body: %s", err.Error()))
		}
		sRunner.Done()
	}
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
	hli.hekaPid = int32(os.Getpid())
	return nil
}

func (hli *HttpListenInput) Run(ir InputRunner, h PluginHelper) (err error) {
	hli.ir = ir
	var hostname, _ = os.Hostname()
	hli.hostname = hostname
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
