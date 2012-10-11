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
package hekagrater

import (
	"log"
	"net"
)

type Input interface {
	Start(receivedChan chan *[]byte)
}

type UdpInput struct {
	Address *net.UDPAddr
}

func NewUdpInput(addrStr string) *UdpInput {
	address, _ := net.ResolveUDPAddr("udp", addrStr)
	input := UdpInput{address}
	return &input
}

func (self *UdpInput) Start(receivedChan chan *[]byte) {
	listener, err := net.ListenUDP("udp", self.Address)
	defer listener.Close()
	if err != nil {
		log.Fatalf("ListenUDP failed: %s", err.Error())
	}
	for {
		inData := make([]byte, 60000)
		n, _, error := listener.ReadFrom(inData)
		if error != nil {
			continue
		}
		inData = inData[:n]
		receivedChan <- &inData
	}
}