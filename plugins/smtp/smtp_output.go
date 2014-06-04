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
	"errors"
	"fmt"
	. "github.com/mozilla-services/heka/pipeline"
	"net"
	"net/smtp"
)

type SmtpOutput struct {
	conf         *SmtpOutputConfig
	auth         smtp.Auth
	sendFunction func(addr string, a smtp.Auth, from string, to []string, msg []byte) error
	encoder      Encoder
}

type SmtpOutputConfig struct {
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

type smtpHeader struct {
	name  string
	value string
}

func (s *SmtpOutput) ConfigStruct() interface{} {
	return &SmtpOutputConfig{
		SendFrom: "heka@localhost.localdomain",
		Host:     "127.0.0.1:25",
		Auth:     "none",
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
	s.encoder = or.Encoder()
	if s.encoder == nil {
		return errors.New("Encoder required.")
	}
	inChan := or.InChan()

	var (
		pack       *PipelinePack
		contents   []byte
		subject    string
		message    string
		headerText string
	)

	if s.conf.Subject == "" {
		subject = "Heka [" + or.Name() + "]"
	} else {
		subject = s.conf.Subject
	}

	headers := make([]smtpHeader, 5)
	headers[0] = smtpHeader{"From", s.conf.SendFrom}
	headers[1] = smtpHeader{"Subject", subject}
	headers[2] = smtpHeader{"MIME-Version", "1.0"}
	headers[3] = smtpHeader{"Content-Type", "text/plain; charset=\"utf-8\""}
	headers[4] = smtpHeader{"Content-Transfer-Encoding", "base64"}

	for _, header := range headers {
		headerText += fmt.Sprintf("%s: %s\r\n", header.name, header.value)
	}

	for pack = range inChan {
		message = headerText

		if contents, err = s.encoder.Encode(pack); err == nil {
			message += "\r\n" + base64.StdEncoding.EncodeToString(contents)
			err = s.sendFunction(s.conf.Host, s.auth, s.conf.SendFrom, s.conf.SendTo, []byte(message))
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
