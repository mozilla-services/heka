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
	"strings"
	"time"
)

type SmtpOutput struct {
	conf         *SmtpOutputConfig
	auth         smtp.Auth
	sendFunction func(addr string, a smtp.Auth, from string, to []string, msg []byte) error
	encoder      Encoder
	inMessage    chan string
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
	// Set a minimum time interval between each email.
	// The value indicates a minimum number of seconds between each email.
	// If more than one message is received in the period, the mail text is concatenated.
	// Default is 0, meaning no limit.
	TickerInterval uint `toml:"ticker_interval"`
}

type smtpHeader struct {
	name  string
	value string
}

func (s *SmtpOutput) ConfigStruct() interface{} {
	return &SmtpOutputConfig{
		SendFrom:       "heka@localhost.localdomain",
		Host:           "127.0.0.1:25",
		Auth:           "none",
		TickerInterval: 0,
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
	var (
		pack     *PipelinePack
		contents []byte
	)
	s.encoder = or.Encoder()
	if s.encoder == nil {
		return errors.New("Encoder required.")
	}
	inChan := or.InChan()

	// We run this direct output if no minimum interval has been requested
	if s.conf.TickerInterval == 0 {
		headerText := s.getHeaderText(or.Name())
		for pack = range inChan {
			message := headerText

			if contents, err = s.encoder.Encode(pack); contents != nil && err == nil {
				message += "\r\n" + base64.StdEncoding.EncodeToString(contents)
				err = s.sendFunction(s.conf.Host, s.auth, s.conf.SendFrom, s.conf.SendTo, []byte(message))
			}

			if err != nil {
				or.LogError(err)
			}

			pack.Recycle()
		}

		return nil
	}

	// Start sender. This will recieve messages on the s.inMessage channel.
	s.inMessage = make(chan string, 1)
	go s.sendLoop(or)

	for pack = range inChan {
		if contents, err = s.encoder.Encode(pack); err == nil {
			s.inMessage <- string(contents)
		} else {
			or.LogError(err)
		}

		pack.Recycle()
	}
	return nil
}

func (s SmtpOutput) sendMail(headerText, message string) error {
	msg := append([]byte(headerText), base64.StdEncoding.EncodeToString([]byte(message))...)
	return s.sendFunction(s.conf.Host, s.auth, s.conf.SendFrom, s.conf.SendTo, msg)
}

func (s SmtpOutput) getHeaderText(name string) string {
	var (
		subject    string
		headerText string
	)
	if s.conf.Subject == "" {
		subject = "Heka [" + name + "]"
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
	return headerText
}

func (s *SmtpOutput) sendLoop(or OutputRunner) {
	var err error
	headerText := s.getHeaderText(or.Name())

	// Bodies of all currently queued messages that will be sent in the next mail
	var queue []string
	// Channel to indicate that timeout has been reached
	timeOut := make(chan bool, 1)
	// Minimum duration between each email
	tickerDur := time.Second * time.Duration(s.conf.TickerInterval)
	// Time indicating when the last message was sent.
	lastSent := time.Now().Add(-tickerDur)

	for {
		select {
		case msg := <-s.inMessage:
			// If none are queued, and we are after the ticker duration, just send it right away.
			if len(queue) == 0 && time.Now().After(lastSent.Add(tickerDur)) {
				err = s.sendMail(headerText, msg)
				lastSent = time.Now()
				if err != nil {
					or.LogError(err)
				}
				continue
			}
			if len(queue) == 0 {
				// The ticker duration has not expired yet, but no messages are queued,
				// so we start a timeout funtion that will fire when the duration has passed
				go func() {
					dur := lastSent.Sub(time.Now()) + tickerDur
					time.Sleep(dur)
					timeOut <- true
				}()
			}
			queue = append(queue, msg)
			// When the timeout has expired, send the messages that are queued
		case <-timeOut:
			message := strings.Join(queue, "\r\n\r\n")
			queue = nil
			err = s.sendMail(headerText, message)
			lastSent = time.Now()
			if err != nil {
				or.LogError(err)
			}
		}
	}
}

func init() {
	RegisterPlugin("SmtpOutput", func() interface{} {
		return new(SmtpOutput)
	})
}
