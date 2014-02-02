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

package nagios

import (
	"github.com/mozilla-services/heka/plugins/process"
	"github.com/mozilla-services/heka/message"
	. "github.com/mozilla-services/heka/pipeline"
	"net/http"
	"net/url"
	"strings"
	"time"
	"crypto/tls"
	"fmt"
)

// NagiosOutput can be configured to use the http client to submit the passive
// checks directly to the nagios cgi, or pipe them to the send_nsca program.
//   -	To use send_nsca, one needs to provide the send_nsca_bin, and optionally
//	send_nsca_args config entries.
//   -	To use http, one needs to provide Url, and optionally username, password,
//	and responseheadertimeout
type NagiosOutputConfig struct {
	// Must match Nagios service's service_description attribute; if not
	// specified in the config explicitly, the name of the output is used.
	NagiosServiceDescription string `toml:"nagios_service_description"`

	// Must match the hostname of the server in nagios. If not specified in
	// the config explicitly, use the Hostname attribute of the message
	NagiosHost string `toml:"nagios_host"`

	// SendNSCA tells the plugin to pipe the commands into a send_nsca program rather than
	// submitting each command using the go http client. The timeout is in seconds.
	SendNscaBin string `toml:"send_nsca_bin"`
	SendNscaArgs []string `toml:"send_nsca_args"`
	SendNscaTimeoutSeconds uint `toml:"send_nsca_timeout"`

	// URL to the Nagios cmd.cgi
	Url string
	// Nagios username
	Username string
	// Nagios password
	Password string
	// Http ResponseHeaderTimeout in seconds
	ResponseHeaderTimeout uint
	// when set, insecure_tls_skip_verify bypasses the server SSL certificate validation (NOT SECURE!!!)
	SkipVerify bool `toml: "insecure_tls_skip_verify"`
}

func (n *NagiosOutput) ConfigStruct() interface{} {
	return &NagiosOutputConfig{
		Url: "http://localhost/cgi-bin/cmd.cgi",
		ResponseHeaderTimeout: 2,
		SendNscaTimeoutSeconds: 5,
	}
}

type NagiosOutput struct {
	conf      *NagiosOutputConfig
	client    *http.Client
	submitter func (or OutputRunner, h PluginHelper, host, service_description, state, output string) (err error)
}

func (n *NagiosOutput) Init(config interface{}) (err error) {
	n.conf = config.(*NagiosOutputConfig)

	n.submitter = n.submitSendNsca
	if n.conf.SendNscaBin == "" {
		// use http
		n.submitter = n.submitHttp
		var tlsconf *tls.Config
		if n.conf.SkipVerify {
			tlsconf = &tls.Config {InsecureSkipVerify: true}
		}

		n.client = &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: tlsconf,
				Proxy: http.ProxyFromEnvironment,
				ResponseHeaderTimeout: time.Duration(n.conf.ResponseHeaderTimeout) * time.Second,
			},
		}
	}
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

		host := n.conf.NagiosHost
		if host == "" {
			host = msg.GetHostname()
		}
		service_description := n.conf.NagiosServiceDescription
		if service_description == "" {
			service_description = msg.GetLogger()
		}
		payload = payload [pos+1:]

		err = n.submitter (or, h, host, service_description, state, payload)
		if err != nil {
			or.LogError (err)
		}

		pack.Recycle()
	}
	return
}

func (n *NagiosOutput) submitSendNsca(or OutputRunner, h PluginHelper, host, service_description, state, output string) (err error) {
	c := process.NewManagedCmd (n.conf.SendNscaBin, n.conf.SendNscaArgs, time.Duration(n.conf.SendNscaTimeoutSeconds) * time.Second)
	cmdin, err := c.StdinPipe()
	if err != nil {
		return
	}
	defer cmdin.Close()
	err = c.Start(false)
	if err != nil {
		return
	}

	_, err = fmt.Fprintf (cmdin, "%s\t%s\t%v\t%s\n", host, service_description, state, output)
	if err != nil {
		return
	}

	// close the input pipe to terminate send_nsca
	cmdin.Close()

	err = c.Wait()
	return
}

func (n *NagiosOutput) submitHttp(or OutputRunner, h PluginHelper, host, service_description, state, output string) (err error) {
	data := url.Values{
		"cmd_typ":          {"30"}, // PROCESS_SERVICE_CHECK_RESULT
		"cmd_mod":          {"2"},  // CMDMODE_COMMIT
		"host":             {host},
		"service":          {service_description},
		"plugin_state":     {state},
		"plugin_output":    { output },
		"performance_data": {""},
	}

	req, err := http.NewRequest("POST", n.conf.Url, strings.NewReader(data.Encode()))
	if err != nil {
		return
	}
	req.SetBasicAuth(n.conf.Username, n.conf.Password)
	resp, err := n.client.Do(req)
	if err != nil {
		return
	}
	err = resp.Body.Close()
	if err != nil {
		return
	}
	return
}

func init() {
	RegisterPlugin("NagiosOutput", func() interface{} {
		return new(NagiosOutput)
	})
}
