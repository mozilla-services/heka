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
#   Rob Miller (rmiller@mozilla.com)
#
# ***** END LICENSE BLOCK *****/
package client

import (
	"heka/message"
	"log"
	"os"
)

type Message message.Message

type Sender interface {
	SendMessage(msgBytes *[]byte) error
}

type Encoder interface {
	EncodeMessage(msg *Message) (*[]byte, error)
}

type Client struct {
	Sender   Sender
	Encoder  Encoder
	Logger   string
	Severity int
	Hostname string
	Pid      int
}

var defaultClient = Client{Logger: "", Severity: 6, Hostname: "", Pid: 0}

func NewHekaClient(sender Sender, encoder Encoder, logger *string,
	severity *int) *Client {
	hostname, err := os.Hostname()
	if err != nil {
		log.Printf("Error getting hostname: %s\n", err.Error())
		hostname = "ERROR"
	}
	pid := os.Getpid()
	self := defaultClient
	self.Sender = sender
	self.Encoder = encoder
	if logger != nil {
		self.Logger = *logger
	}
	if severity != nil {
		self.Severity = *severity
	}
	self.Hostname = hostname
	self.Pid = pid
	return &self
}

func (self *Client) SendMessage(msg *Message) error {
	var err error
	msgBytes, err := self.Encoder.EncodeMessage(msg)
	if err == nil {
		err = self.Sender.SendMessage(msgBytes)
	}
	return err
}
