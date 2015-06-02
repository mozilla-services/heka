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
#   Mike Trinkala (trink@mozilla.com)
#   Rob Miller (rmiller@mozilla.com)
#
# ***** END LICENSE BLOCK *****/

package nagios

import (
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/mozilla-services/heka/message"
	. "github.com/mozilla-services/heka/pipeline"
	"github.com/mozilla-services/heka/plugins/process"
	"github.com/mozilla-services/heka/plugins/tcp"
)

// NagiosOutput can be configured to use the http client to submit the passive
// checks directly to the nagios cgi, or pipe them to the send_nsca program.
// To use send_nsca, one needs to provide the send_nsca_bin, and optionally
// send_nsca_args config entries. To use http, one needs to provide Url, and
// optionally username, password, and response_header_timeout.
type NagiosOutputConfig struct {
	// Must match Nagios service's service_description attribute; if not
	// specified in the config explicitly, the name of the output is used.
	NagiosServiceDescription string `toml:"nagios_service_description"`

	// Must match the hostname of the server in nagios. If not specified in
	// the config explicitly, use the Hostname attribute of the message
	NagiosHost string `toml:"nagios_host"`

	// SendNSCA tells the plugin to pipe the commands into a send_nsca program
	// rather than submitting each command using the go http client. The
	// timeout is in seconds.
	SendNscaBin            string   `toml:"send_nsca_bin"`
	SendNscaArgs           []string `toml:"send_nsca_args"`
	SendNscaTimeoutSeconds uint     `toml:"send_nsca_timeout"`

	// URL to the Nagios cmd.cgi
	Url string
	// Nagios username
	Username string
	// Nagios password
	Password string
	// Http ResponseHeaderTimeout in seconds.
	ResponseHeaderTimeout uint `toml:"response_header_timeout"`

	// Set to true if the HTTP connection should be tunneled through TLS.
	// Requires additional Tls config section.
	UseTls bool `toml:"use_tls"`
	// Subsection for TLS configuration.
	Tls tcp.TlsConfig
}

func (n *NagiosOutput) ConfigStruct() interface{} {
	return &NagiosOutputConfig{
		Url: "http://localhost/cgi-bin/cmd.cgi",
		ResponseHeaderTimeout:  2,
		SendNscaTimeoutSeconds: 5,
	}
}

type NagiosOutput struct {
	conf      *NagiosOutputConfig
	client    *http.Client
	submitter func(host, service_description, state, output string) (err error)
}

func (n *NagiosOutput) Init(config interface{}) (err error) {
	n.conf = config.(*NagiosOutputConfig)

	n.submitter = n.submitSendNsca
	if n.conf.SendNscaBin == "" {
		// HTTP is implied.
		n.submitter = n.submitHttp

		rht := time.Duration(n.conf.ResponseHeaderTimeout) * time.Second
		transport := &http.Transport{
			Proxy: http.ProxyFromEnvironment,
			ResponseHeaderTimeout: rht,
		}
		if n.conf.UseTls {
			var tlsConf *tls.Config
			if tlsConf, err = tcp.CreateGoTlsConfig(&n.conf.Tls); err != nil {
				return fmt.Errorf("TLS init error: %s", err)
			}
			transport.TLSClientConfig = tlsConf
		}
		n.client = &http.Client{
			Transport: transport,
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
		payload = payload[pos+1:]

		err = n.submitter(host, service_description, state, payload)
		if err != nil {
			err = NewRetryMessageError(err.Error())
			pack.Recycle(err)
			continue
		}
		or.UpdateCursor(pack.QueueCursor)
		pack.Recycle(nil)
	}
	return
}

func (n *NagiosOutput) submitSendNsca(host, service_description, state,
	output string) (err error) {

	c := process.NewManagedCmd(n.conf.SendNscaBin, n.conf.SendNscaArgs,
		time.Duration(n.conf.SendNscaTimeoutSeconds)*time.Second)

	var cmdin io.WriteCloser
	if cmdin, err = c.StdinPipe(); err != nil {
		return
	}
	defer cmdin.Close()

	if err = c.Start(false); err != nil {
		return
	}

	if _, err = fmt.Fprintf(cmdin, "%s\t%s\t%v\t%s\n", host, service_description,
		state, output); err != nil {
		return
	}

	// close the input pipe to terminate send_nsca
	cmdin.Close()

	err = c.Wait()
	return
}

func (n *NagiosOutput) submitHttp(host, service_description, state,
	output string) (err error) {

	data := url.Values{
		"cmd_typ":          {"30"}, // PROCESS_SERVICE_CHECK_RESULT
		"cmd_mod":          {"2"},  // CMDMODE_COMMIT
		"host":             {host},
		"service":          {service_description},
		"plugin_state":     {state},
		"plugin_output":    {output},
		"performance_data": {""},
	}

	var (
		req  *http.Request
		resp *http.Response
	)
	req, err = http.NewRequest("POST", n.conf.Url, strings.NewReader(data.Encode()))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	req.SetBasicAuth(n.conf.Username, n.conf.Password)
	if resp, err = n.client.Do(req); err != nil {
		return
	}
	err = resp.Body.Close()
	return
}

func init() {
	RegisterPlugin("NagiosOutput", func() interface{} {
		return new(NagiosOutput)
	})
}
