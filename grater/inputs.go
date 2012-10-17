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
	"os"
)

type Input interface {
	Start(pipeline func(*PipelinePack), recycleChan <-chan *PipelinePack)
}

type UdpInput struct {
	listener *net.PacketConn
}

func NewUdpInput(addrStr *string, fd *uintptr) *UdpInput {
	var listener net.PacketConn
	if *fd != 0 {
		udpFile := os.NewFile(*fd, "udpFile")
		fdConn, err := net.FilePacketConn(udpFile)
		if err != nil {
			log.Printf("Error accessing UDP fd: %s\n", err.Error())
			return nil
		}
		listener = fdConn
	} else {
		var err error
		listener, err = net.ListenPacket("udp", *addrStr)
		if err != nil {
			log.Printf("ListenPacket failed: %s\n", err.Error())
			return nil
		}
	}
	return &UdpInput{&listener}
}

func (self *UdpInput) Start(pipeline func(*PipelinePack),
	                    recycleChan <-chan *PipelinePack) {
	go func() {
		for {
			pipelinePack := <-recycleChan
			msgBytesFull := pipelinePack.MsgBytes
			n, _, error := (*self.listener).ReadFrom(*msgBytesFull)
			if error != nil {
				continue
			}
			msgBytes := (*msgBytesFull)[:n]
			pipelinePack.MsgBytes = &msgBytes
			go pipeline(pipelinePack)
		}
	}()
}
