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
package pipeline

import (
	"log"
	"net"
	"os"
	"sync"
	"time"
)

type InputRunner struct {
	input   Input
	timeout *time.Duration
	running bool
}

func (self *InputRunner) Start(pipeline func(*PipelinePack),
	recycleChan <-chan *PipelinePack, wg *sync.WaitGroup) {
	self.running = true

	go func() {
		for self.running {
			pipelinePack := <-recycleChan
			msgBytes := pipelinePack.MsgBytes
			*msgBytes = (*msgBytes)[:cap(*msgBytes)]

			n, err := self.input.Read(msgBytes, self.timeout)
			if err != nil {
				continue
			}
			*msgBytes = (*msgBytes)[:n]
			pipelinePack.MsgBytes = msgBytes
			go pipeline(pipelinePack)
		}
		wg.Done()
	}()
}

func (self *InputRunner) Stop() {
	self.running = false
}

// Represents a generic config section for the given Input
type InputConfig interface{}

// An Input can initialize itself as appropriate when LoadConfig is
// called, before Read will be run.
type Input interface {
	LoadConfig(config *InputConfig) error
	Read(msgBytes *[]byte, timeout *time.Duration) (int, error)
}

type UdpInput struct {
	listener *net.PacketConn
	deadline time.Time
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
	return &UdpInput{listener: &listener}
}

func (self *UdpInput) Read(msgBytes *[]byte, timeout *time.Duration) (int, error) {
	self.deadline = time.Now().Add(*timeout)
	(*self.listener).SetReadDeadline(self.deadline)
	n, _, err := (*self.listener).ReadFrom(*msgBytes)
	return n, err
}

func (self *UdpInput) LoadConfig(config *InputConfig) error {
	return nil
}
