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
	"code.google.com/p/goprotobuf/proto"
	"github.com/mozilla-services/heka/message"
	"net"
)

type Sender interface {
	SendMessage(msgBytes []byte, encoding message.Header_MessageEncoding,
		msc *message.MessageSigningConfig) error
}

type UdpSender struct {
	connection  *net.UDPConn
	buf         []byte
	protoBuffer *proto.Buffer
}

func NewUdpSender(addrStr string) (*UdpSender, error) {
	var self *UdpSender
	udpAddr, err := net.ResolveUDPAddr("udp", addrStr)
	conn, err := net.DialUDP("udp", nil, udpAddr)
	if err == nil {
		self = &(UdpSender{connection: conn})
		self.buf = make([]byte, message.MAX_MESSAGE_SIZE+message.MAX_HEADER_SIZE+3)
		self.protoBuffer = proto.NewBuffer(self.buf)
	} else {
		self = nil
	}
	return self, err
}

func (self *UdpSender) SendMessage(msgBytes []byte,
	encoding message.Header_MessageEncoding,
	msc *message.MessageSigningConfig) (err error) {
	err = EncodeStreamHeader(msgBytes, encoding, &self.buf, msc)
	if err != nil {
		return
	}
	self.buf = append(self.buf, msgBytes...)
	_, err = self.connection.Write(self.buf)
	return
}

type TcpSender struct {
	connection  net.Conn
	header      []byte
	protoBuffer *proto.Buffer
}

func NewTcpSender(addrStr string) (n *TcpSender, err error) {
	conn, err := net.Dial("tcp", addrStr)
	if err == nil {
		n = &(TcpSender{connection: conn})
		n.header = make([]byte, message.MAX_HEADER_SIZE+3)
		n.protoBuffer = proto.NewBuffer(n.header)
	}
	return
}

func (t *TcpSender) SendMessage(msgBytes []byte,
	encoding message.Header_MessageEncoding,
	msc *message.MessageSigningConfig) (err error) {
	err = EncodeStreamHeader(msgBytes, encoding, &t.header, msc)
	if err != nil {
		return
	}
	_, err = t.connection.Write(t.header)
	if err == nil {
		_, err = t.connection.Write(msgBytes)
	}
	return
}

func (t *TcpSender) Close() {
	t.connection.Close()
}
