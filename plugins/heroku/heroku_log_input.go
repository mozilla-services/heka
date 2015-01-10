/***** BEGIN LICENSE BLOCK *****
# This Source Code Form is subject to the terms of the Mozilla Public
# License, v. 2.0. If a copy of the MPL was not distributed with this file,
# You can obtain one at http://mozilla.org/MPL/2.0/.
#
# The Initial Developer of the Original Code is the Mozilla Foundation.
# Portions created by the Initial Developer are Copyright (C) 2014
# the Initial Developer. All Rights Reserved.
#
# This plugin is based on the HttpListenPlugin.
#
# Contributor(s):
#   Anton Lindström (carlantonlindstrom@gmail.com)
#
# ***** END LICENSE BLOCK *****/

package heroku

import (
	"code.google.com/p/go-uuid/uuid"
	"fmt"
	"github.com/mozilla-services/heka/message"
	"github.com/mozilla-services/heka/pipeline"
	"io/ioutil"
	"net"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type HerokuLogInput struct {
	conf        *HerokuLogInputConfig
	listener    net.Listener
	stopChan    chan bool
	ir          pipeline.InputRunner
	server      *http.Server
	starterFunc func(hli *HerokuLogInput) error
	hostname    string
}

const LOGPLEX_CONTENT_TYPE = "application/logplex-1"

// Heroku Log Input config struct
type HerokuLogInputConfig struct {
	// TCP Address to listen to
	// Defaults to "0.0.0.0:8325".
	Address string
}

func (hli *HerokuLogInput) ConfigStruct() interface{} {
	return &HerokuLogInputConfig{
		Address: "0.0.0.0:8325",
	}
}

func herokuStarter(hli *HerokuLogInput) (err error) {
	hli.listener, err = net.Listen("tcp", hli.conf.Address)
	if err != nil {
		return fmt.Errorf("Listener [%s] start fail: %s\n",
			hli.conf.Address, err.Error())
	} else {
		hli.ir.LogMessage(fmt.Sprintf("Listening on %s", hli.conf.Address))
	}

	err = hli.server.Serve(hli.listener)
	if err != nil {
		return fmt.Errorf("Serve fail: %s\n", err.Error())
	}

	return nil
}

// A Heroku log drain recieves messages on this format:
var herokuLogRegexp = regexp.MustCompile(`^\d+ <(\d+)>(\d+) (.*) (.*) (.*) (.*) - (.*)$`)

// Example:
//   83 <40>1 2012-11-30T06:45:29+00:00 host app web.3 - State changed from starting to up

func (hli *HerokuLogInput) RequestHandler(req *http.Request) error {
	if req.Method != "POST" || len(req.URL.Path) < 2 ||
		req.Header.Get("Content-Type") != LOGPLEX_CONTENT_TYPE {
		return fmt.Errorf("Bad request")
	}

	msgCount, err := strconv.Atoi(req.Header.Get("Logplex-Msg-Count"))
	if err != nil {
		return fmt.Errorf("Could not parse Logplex-Msg-Count: %s", err.Error())
	}

	body, err := ioutil.ReadAll(req.Body)
	defer req.Body.Close()
	if err != nil {
		return fmt.Errorf("Read HTTP request body fail: %s", err.Error())
	}

	messages := strings.Split(string(body), "\n")
	acctualMsgCount := len(messages) - 1
	if msgCount != acctualMsgCount {
		return fmt.Errorf("Expected %d messages, received %d",
			msgCount, acctualMsgCount)
	}

	// This is the unique identifier for the logplex drain.
	drainToken := req.Header.Get("Logplex-Drain-Token")

	// Use the path as app name (but remove the first /).
	app := req.URL.Path[1:]

	for _, line := range messages {
		if len(line) == 0 {
			continue
		}

		groups := herokuLogRegexp.FindStringSubmatch(line)
		if len(groups) != 8 {
			return fmt.Errorf("Could not parse message: %s", line)
		}

		pack := <-hli.ir.InChan()

		// See https://devcenter.heroku.com/articles/logging#log-format and
		// https://devcenter.heroku.com/articles/log-drains#http-s-drains

		pri := groups[1]
		timestamp := groups[3]
		dyno := groups[6]
		payload := groups[7]

		if ts, err := time.Parse(time.RFC3339Nano, timestamp); err != nil {
			hli.ir.LogError(fmt.Errorf("Could not parse timestamp: %s", err.Error()))
			continue
		} else {
			pack.Message.SetTimestamp(ts.UnixNano())
		}
		if priority, err := strconv.Atoi(pri); err != nil {
			hli.ir.LogError(fmt.Errorf("Could not parse priority: %s", err.Error()))
		} else {
			severity := int32(priority & 7)
			pack.Message.SetSeverity(severity)
		}

		pack.Message.SetUuid(uuid.NewRandom())
		pack.Message.SetType("HerokuLog")
		pack.Message.SetHostname(app)
		pack.Message.SetLogger(dyno)
		pack.Message.SetPayload(payload)
		message.NewStringField(pack.Message, "DrainToken", drainToken)

		hli.ir.Deliver(pack)
	}

	return nil
}

func (hli *HerokuLogInput) RequestHandlerWrapper(w http.ResponseWriter, req *http.Request) {
	if err := hli.RequestHandler(req); err != nil {
		hli.ir.LogError(err)
		http.Error(w, err.Error(), 500)
	}
}

func (hli *HerokuLogInput) Init(config interface{}) (err error) {
	hli.conf = config.(*HerokuLogInputConfig)
	if hli.starterFunc == nil {
		hli.starterFunc = herokuStarter
	}
	hli.stopChan = make(chan bool, 1)

	handler := http.HandlerFunc(hli.RequestHandlerWrapper)

	hli.server = &http.Server{
		Handler: handler,
	}

	return nil
}

func (hli *HerokuLogInput) Run(ir pipeline.InputRunner, h pipeline.PluginHelper) (err error) {
	hli.ir = ir
	hli.hostname = h.Hostname()
	err = hli.starterFunc(hli)
	if err != nil {
		return err
	}

	<-hli.stopChan

	return nil
}

func (hli *HerokuLogInput) Stop() {
	if hli.listener != nil {
		hli.listener.Close()
	}
	close(hli.stopChan)
}

func init() {
	pipeline.RegisterPlugin("HerokuLogInput", func() interface{} {
		return new(HerokuLogInput)
	})
}
