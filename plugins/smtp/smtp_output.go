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
#   Christian Vozar (christian@bellycard.com)
#   Rob Miller (rmiller@mozilla.com)
#   Kun Liu (git@lk.vc)
#
# ***** END LICENSE BLOCK *****/

package smtp

import (
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"net/smtp"
	"strings"
	"time"

	. "github.com/mozilla-services/heka/pipeline"
)

const BASE64_ENCODING_LINE_LENGTH = 76
const BASE64_SOURCE_CHARS_PER_LINE = BASE64_ENCODING_LINE_LENGTH / 4 * 3

type SmtpOutput struct {
	conf         *SmtpOutputConfig
	auth         smtp.Auth
	sendFunction func(addr string, a smtp.Auth, from string, to []string, msg []byte) error
	inMessage    chan []byte
	or           OutputRunner
	fullMsg      []byte
	headerLen    int
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
	// Set a minimum time interval between each email. The value indicates a
	// minimum number of seconds between each email. If more than one message
	// is received in the period, the mail text is concatenated. Default is 0,
	// meaning no limit.
	SendInterval uint `toml:"send_interval"`
}

type smtpHeader struct {
	name  string
	value string
}

func (s *SmtpOutput) ConfigStruct() interface{} {
	return &SmtpOutputConfig{
		SendFrom:     "heka@localhost.localdomain",
		Host:         "127.0.0.1:25",
		Auth:         "none",
		SendInterval: 0,
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
	s.or = or
	if or.Encoder() == nil {
		return errors.New("encoder required")
	}

	inChan := or.InChan()
	s.fullMsg = s.getHeader()
	s.headerLen = len(s.fullMsg)

	if s.conf.SendInterval != 0 {
		// Start sender. This will receive messages on the s.inMessage channel.
		s.inMessage = make(chan []byte, 1)
		go s.sendLoop()
	}

	for pack = range inChan {
		contents, err = or.Encode(pack)
		if contents == nil || err != nil {
			if err != nil {
				err = fmt.Errorf("encoding error: %s", err.Error())
			}
			pack.Recycle(err)
			continue
		}

		// We run this direct output if no minimum interval has been requested.
		if s.conf.SendInterval == 0 {
			err = s.sendMail(contents)
			if err != nil {
				e := NewRetryMessageError("sending error: %s", err.Error())
				pack.Recycle(e)
				continue
			}
		} else {
			s.inMessage <- contents
		}
		pack.Recycle(nil)
	}
	return nil
}

func (s *SmtpOutput) encodeFullMsg(contents []byte) {
	// RFC 2045 6.8 Base64 Content-Transfer-Encoding
	// The encoded output stream must be represented in lines of no more
	// than 76 characters each.
	contentLen := len(contents)
	encodedLen := base64.StdEncoding.EncodedLen(contentLen)
	if encodedLen > BASE64_ENCODING_LINE_LENGTH {
		// append "\r\n" for each line
		encodedLen += (encodedLen - 1) / BASE64_ENCODING_LINE_LENGTH * 2
	}
	if cap(s.fullMsg) < s.headerLen+encodedLen {
		newFullMsg := make([]byte, s.headerLen+encodedLen)
		copy(newFullMsg, s.fullMsg[:s.headerLen])
		s.fullMsg = newFullMsg
	}
	s.fullMsg = s.fullMsg[:s.headerLen+encodedLen]
	encodedLines := 0
	contentLines := 1 + (contentLen-1)/BASE64_SOURCE_CHARS_PER_LINE
	if contentLen <= 0 {
		contentLines = 0
	}
	for encodedLines < contentLines {
		offset := s.headerLen + encodedLines*(BASE64_ENCODING_LINE_LENGTH+2)
		contentsBegin := encodedLines * BASE64_SOURCE_CHARS_PER_LINE
		contentsEnd := contentsBegin + BASE64_SOURCE_CHARS_PER_LINE
		if contentsEnd > contentLen {
			contentsEnd = contentLen
		}
		base64.StdEncoding.Encode(s.fullMsg[offset:], contents[contentsBegin:contentsEnd])
		encodedLines++
		if encodedLines < contentLines {
			offset += BASE64_ENCODING_LINE_LENGTH
			s.fullMsg[offset] = '\r'
			s.fullMsg[offset+1] = '\n'
		}
	}
}

func (s SmtpOutput) sendMail(contents []byte) error {
	s.encodeFullMsg(contents)
	return s.sendFunction(s.conf.Host, s.auth, s.conf.SendFrom, s.conf.SendTo, s.fullMsg)
}

// Only called once at startup.
func (s SmtpOutput) getHeader() []byte {
	var subject string
	if s.conf.Subject != "" {
		subject = s.conf.Subject
	} else {
		subject = fmt.Sprintf("Heka [%s]", s.or.Name())
	}
	headers := make([]string, 5)
	headers[0] = "From: " + s.conf.SendFrom
	headers[1] = encodeSubject(subject)
	headers[2] = "MIME-Version: 1.0"
	headers[3] = "Content-Type: text/plain; charset=\"utf-8\""
	headers[4] = "Content-Transfer-Encoding: base64"
	return []byte(strings.Join(headers, "\r\n") + "\r\n\r\n")
}

func (s *SmtpOutput) sendLoop() {
	var err error

	// Bodies of all currently queued messages that will be sent in the next mail
	var queue []byte
	// Channel to indicate that timeout has been reached
	timeOut := make(chan bool, 1)
	// Minimum duration between each email
	tickerDur := time.Second * time.Duration(s.conf.SendInterval)
	// Time indicating when the last message was sent.
	lastSent := time.Now().Add(-tickerDur)

	for {
		select {
		case msg := <-s.inMessage:
			// If none are queued, and we are after the ticker duration, just
			// send it right away.
			if len(queue) == 0 && time.Now().After(lastSent.Add(tickerDur)) {
				err = s.sendMail(msg)
				lastSent = time.Now()
				if err != nil {
					s.or.LogError(err)
				}
				continue
			}
			if len(queue) == 0 {
				// The ticker duration has not expired yet, but no messages
				// are queued, so we start a timeout function that will fire
				// when the duration has passed.
				go func() {
					dur := lastSent.Sub(time.Now()) + tickerDur
					time.Sleep(dur)
					timeOut <- true
				}()
			}
			queue = append(queue, msg...)
			queue = append(queue, []byte("\r\n\r\n")...)
		case <-timeOut:
			// When the timeout has expired, send the messages that are
			// queued.
			contents := queue[:len(queue)-4]
			queue = queue[:0]
			err = s.sendMail(contents)
			lastSent = time.Now()
			if err != nil {
				s.or.LogError(err)
			}
		}
	}
}

func init() {
	RegisterPlugin("SmtpOutput", func() interface{} {
		return new(SmtpOutput)
	})
}
