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

type TimeoutError struct {
	*Error
}

type InputRunner struct {
	input   Input
	timeout *time.Duration
	running bool
}

func (self *InputRunner) Start(pipeline func(*PipelinePack),
	recycleChan <-chan *PipelinePack, wg *sync.WaitGroup) {
	self.running = true

	go func() {
		var err error
		for self.running {
			pipelinePack := <-recycleChan
			err = self.input.Read(pipelinePack, self.timeout)
			if err != nil {
				continue
			}
			go pipeline(pipelinePack)
		}
		wg.Done()
	}()
}

func (self *InputRunner) Stop() {
	self.running = false
}

// An Input can initialize itself as appropriate when LoadConfig is
// called, before Read will be run.
type Input interface {
	Init(config *PluginConfig) error
	Read(pipelinePack *PipelinePack, timeout *time.Duration) error
}

type UdpInput struct {
	listener *net.PacketConn
	deadline time.Time
}

type MessageGeneratorInput struct {
	messages chan *Message
}

func NewUdpInput(addrStr string, fd *uintptr) *UdpInput {
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
		listener, err = net.ListenPacket("udp", addrStr)
		if err != nil {
			log.Printf("ListenPacket failed: %s\n", err.Error())
			return nil
		}
	}
	return &UdpInput{listener: &listener}
}

func (self *UdpInput) Init(config *PluginConfig) error {
	return nil
}

func (self *UdpInput) Read(pipelinePack *PipelinePack,
	timeout *time.Duration) (int, error) {
	self.deadline = time.Now().Add(*timeout)
	(*self.listener).SetReadDeadline(self.deadline)
	n, _, err := (*self.listener).ReadFrom(pipelinePack.MsgBytes)
	if err == nil {
		pipelinePack.MsgBytes = pipelinePack.MsgBytes[:n]
	}
	return err
}

func (self *MessageGeneratorInput) Init(config *PluginConfig) error {
	self.messages = make(chan *Message, 100)
	return nil
}

func (self *MessageGeneratorInput) Deliver(msg *Message) {
	self.messages <- msg
}

func (self *MessageGeneratorInput) Read(pipeline *PipelinePack,
	timeout *time.Duration) error {
	select {
	case msg := <-self.messages:
		pipeline.Message = msg
		pipeline.Decoded = true
		return nil
	case <-time.After(timeout):
		return new(TimeoutError)
	}
}
