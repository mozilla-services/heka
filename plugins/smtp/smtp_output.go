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
#   Christian Vozar (christian@bellycard.com)
#
# ***** END LICENSE BLOCK *****/

package smtp

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/mozilla-services/heka/message"
	. "github.com/mozilla-services/heka/pipeline"
	"net"
	"net/mail"
	"net/smtp"
	"strings"
)

type SmtpOutput struct {
	conf         *SmtpOutputConfig
	auth         smtp.Auth
	sendFunction func(addr string, a smtp.Auth, from string, to []string, msg []byte) error
}

type SmtpOutputConfig struct {
	// Outputs the payload attribute in the email message vs a full JSON message dump
	PayloadOnly bool `toml:"payload_only"`
	// email addresses to send the output from
	SendFrom string `toml:"send_from"`
	// email addresses to send the output to
	SendTo []string `toml:"send_to"`
	// User defined email subject line
	Subject string
	// SMTP Host
	Host string
	// SMTP Authentication type
	Auth string
	// SMTP user
	User string
	// SMTP password
	Password string
}

func (s *SmtpOutput) ConfigStruct() interface{} {
	return &SmtpOutputConfig{
		PayloadOnly: true,
		SendFrom:    "heka@localhost.localdomain",
		Host:        "127.0.0.1:25",
		Auth:        "none",
	}
}

func (s *SmtpOutput) Init(config interface{}) (err error) {
	s.conf = config.(*SmtpOutputConfig)

	if s.conf.SendTo == nil {
		return fmt.Errorf("send_to must contain at least one recipient")
	}

	host, _, err := net.SplitHostPort(s.conf.Host)
	if err != nil {
		return fmt.Errorf("Host must contain a port specifier")
	}

	s.sendFunction = smtp.SendMail

	if s.conf.Auth == "Plain" {
		s.auth = smtp.PlainAuth("", s.conf.User, s.conf.Password, host)
	} else if s.conf.Auth == "CRAMMD5" {
		s.auth = smtp.CRAMMD5Auth(s.conf.User, s.conf.Password)
	} else if s.conf.Auth == "none" {
		s.auth = nil
	} else {
		return fmt.Errorf("Invalid auth type: %s", s.conf.Auth)
	}
	return
}

func (s *SmtpOutput) Run(or OutputRunner, h PluginHelper) (err error) {
	inChan := or.InChan()

	var (
		pack     *PipelinePack
		msg      *message.Message
		contents []byte
		subject  string
		message  string
	)

	if s.conf.Subject == "" {
		subject = "Heka [" + or.Name() + "]"
	} else {
		subject = s.conf.Subject
	}

	header := make(map[string]string)
	header["From"] = s.conf.SendFrom
	header["Subject"] = subject
	header["MIME-Version"] = "1.0"
	header["Content-Type"] = "text/plain; charset=\"utf-8\""
	header["Content-Transfer-Encoding"] = "base64"

	for k, v := range header {
		message += fmt.Sprintf("%s: %s\r\n", k, v)
	}

	for pack = range inChan {
		msg = pack.Message
		if s.conf.PayloadOnly {
			message += "\r\n" + base64.StdEncoding.EncodeToString([]byte(msg.GetPayload()))
			err = s.sendFunction(s.conf.Host, s.auth, s.conf.SendFrom, s.conf.SendTo, []byte(message))
		} else {
			if contents, err = json.Marshal(msg); err == nil {
				message += "\r\n" + base64.StdEncoding.EncodeToString(contents)
				err = s.sendFunction(s.conf.Host, s.auth, s.conf.SendFrom, s.conf.SendTo, []byte(message))
			} else {
				or.LogError(err)
			}
		}
		if err != nil {
			or.LogError(err)
		}
		pack.Recycle()
	}
	return
}

func init() {
	RegisterPlugin("SmtpOutput", func() interface{} {
		return new(SmtpOutput)
	})
}
