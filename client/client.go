/***** BEGIN LICENSE BLOCK *****
# This Source Code Form is subject to the terms of the Mozilla Public
# License, v. 2.0. If a copy of the MPL was not distributed with this file,
# You can obtain one at http://mozilla.org/MPL/2.0/.
#
# The Initial Developer of the Original Code is the Mozilla Foundation.
# Portions created by the Initial Developer are Copyright (C) 2012-2014
# the Initial Developer. All Rights Reserved.
#
# Contributor(s):
#   Rob Miller (rmiller@mozilla.com)
#   Mike Trinkala (trink@mozilla.com)
#
# ***** END LICENSE BLOCK *****/

/*

Client package to talk to heka from Go.

*/
package client

import (
	"github.com/mozilla-services/heka/message"
	"log"
	"os"
)

var (
	LogInfo  = log.New(os.Stdout, "", log.LstdFlags)
	LogError = log.New(os.Stderr, "", log.LstdFlags)
)

type Client struct {
	Sender  Sender
	Encoder StreamEncoder
	buf     []byte
}

func NewClient(sender Sender, encoder StreamEncoder) (self *Client) {
	self = &Client{Sender: sender, Encoder: encoder}
	return
}

func (self *Client) SendMessage(msg *message.Message) (err error) {
	err = self.Encoder.EncodeMessageStream(msg, &self.buf)
	if err == nil {
		err = self.Sender.SendMessage(self.buf)
	}
	return
}
