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
#   Mike Trinkala (trink@mozilla.com)
#
# ***** END LICENSE BLOCK *****/

package client

import (
	"net"
)

type Sender interface {
	SendMessage(outBytes []byte) (err error)
	Close()
}

type NetworkSender struct {
	connection net.Conn
}

func NewNetworkSender(proto, addr string) (self *NetworkSender, err error) {
	conn, err := net.Dial(proto, addr)
	if err == nil {
		self = &(NetworkSender{conn})
	}
	return
}

func (self *NetworkSender) SendMessage(outBytes []byte) (err error) {
	_, err = self.connection.Write(outBytes)
	return
}

func (self *NetworkSender) Close() {
	self.connection.Close()
}
