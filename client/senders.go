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
	"github.com/mozilla-services/heka/pipeline"
	"net"
)

type Sender interface {
	SendMessage(msgBytes []byte) error
}

type UdpSender struct {
	connection *net.UDPConn
}

func NewUdpSender(addrStr string) (*UdpSender, error) {
	var self *UdpSender
	udpAddr, err := net.ResolveUDPAddr("udp", addrStr)
	conn, err := net.DialUDP("udp", nil, udpAddr)
	if err == nil {
		self = &(UdpSender{conn})
	} else {
		self = nil
	}
	return self, err
}

func (self *UdpSender) SendMessage(msgBytes []byte) error {
	_, err := self.connection.Write(msgBytes)
	return err
}

type TcpSender struct {
	connection net.Conn
}

func NewTcpSender(addrStr string) (n *TcpSender, err error) {
	conn, err := net.Dial("tcp", addrStr)
	if err == nil {
		n = &(TcpSender{conn})
	}
	return
}

func (n *TcpSender) SendMessage(msgBytes []byte) error {
	h := &message.Header{}
	h.SetMessageLength(uint32(len(msgBytes)))
	headerBytes, err := proto.Marshal(h)
	if err != nil {
		return err
	}
	headerLen := 3 + len(headerBytes)
	headerBuf := make([]byte, headerLen)
	headerBuf[0] = pipeline.RECORD_SEPARATOR
	headerBuf[1] = uint8(len(headerBytes))
	copy(headerBuf[2:], headerBytes)
	headerBuf[headerLen-1] = pipeline.UNIT_SEPARATOR
	_, err = n.connection.Write(headerBuf)
	if err == nil {
		_, err = n.connection.Write(msgBytes)
	}
	return err
}
