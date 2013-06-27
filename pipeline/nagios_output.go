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
#   Mike Trinkala (trink@mozilla.com)
#
# ***** END LICENSE BLOCK *****/

package pipeline

import (
	"github.com/mozilla-services/heka/message"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type NagiosOutputConfig struct {
	// URL to the Nagios cmd.cgi
	Url string
	// Nagios username
	Username string
	// Nagios password
	Password string
	// Http ResponseHeaderTimeout in seconds
	ResponseHeaderTimeout uint
}

func (n *NagiosOutput) ConfigStruct() interface{} {
	return &NagiosOutputConfig{
		Url: "http://localhost/cgi-bin/cmd.cgi",
		ResponseHeaderTimeout: 2,
	}
}

type NagiosOutput struct {
	conf      *NagiosOutputConfig
	client    *http.Client
	transport *http.Transport
}

func (n *NagiosOutput) Init(config interface{}) (err error) {
	n.conf = config.(*NagiosOutputConfig)
	n.transport = &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		ResponseHeaderTimeout: time.Duration(n.conf.ResponseHeaderTimeout) * time.Second}
	n.client = &http.Client{Transport: n.transport}
	return
}

func (n *NagiosOutput) Run(or OutputRunner, h PluginHelper) (err error) {
	inChan := or.InChan()

	var (
		pack    *PipelinePack
		msg     *message.Message
		payload string
	)

	for pack = range inChan {
		msg = pack.Message
		payload = msg.GetPayload()
		pos := strings.IndexAny(payload, ":")
		state := "3" // UNKNOWN
		if pos != -1 {
			switch payload[:pos] {
			case "OK":
				state = "0"
			case "WARNING":
				state = "1"
			case "CRITICAL":
				state = "2"
			}
		}

		data := url.Values{
			"cmd_typ":          {"30"}, // PROCESS_SERVICE_CHECK_RESULT
			"cmd_mod":          {"2"},  // CMDMODE_COMMIT
			"host":             {msg.GetHostname()},
			"service":          {msg.GetLogger()},
			"plugin_state":     {state},
			"plugin_output":    {payload[pos+1:]},
			"performance_data": {""}}
		req, err := http.NewRequest("POST", n.conf.Url,
			strings.NewReader(data.Encode()))
		if err == nil {
			req.SetBasicAuth(n.conf.Username, n.conf.Password)
			if resp, err := n.client.Do(req); err == nil {
				resp.Body.Close()
			} else {
				or.LogError(err)
			}
		} else {
			or.LogError(err)
		}
		pack.Recycle()
	}
	return
}
