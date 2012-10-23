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
package hekaclient

import (
	"net"
)

type UdpSender struct {
	connection *net.UDPConn
}

func NewUdpSender(addrStr *string) (*UdpSender, error) {
	var self *UdpSender
	udpAddr, err := net.ResolveUDPAddr("udp", *addrStr)
	conn, err := net.DialUDP("udp", nil, udpAddr)
	if err == nil {
		self = &(UdpSender{conn})
	} else {
		self = nil
	}
	return self, err
}

func (self *UdpSender) SendMessage(msgBytes *[]byte) error {
	_, err := self.connection.Write(*msgBytes)
	return err
}
