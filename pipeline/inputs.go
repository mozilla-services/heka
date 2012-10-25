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
	"encoding/gob"
	"fmt"
	. "heka/message"
	"log"
	"net"
	"os"
	"sync"
	"time"
)

type TimeoutError string

func (self *TimeoutError) Error() string {
	return fmt.Sprint("Error: Read timed out")
}

type Input interface {
	Plugin
	Read(pipelinePack *PipelinePack, timeout *time.Duration) error
}

// InputRunner
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
		var pipelinePack *PipelinePack
		needOne := true
		for self.running {
			if needOne {
				pipelinePack = <-recycleChan
			}
			err = self.input.Read(pipelinePack, self.timeout)
			if err != nil {
				needOne = false
				continue
			}
			go pipeline(pipelinePack)
			needOne = true
		}
		wg.Done()
	}()
}

func (self *InputRunner) Stop() {
	self.running = false
}

// UdpInput
type UdpInput struct {
	listener *net.Conn
	deadline time.Time
}

func NewUdpInput(addrStr string, fd *uintptr) *UdpInput {
	var listener net.Conn
	if *fd != 0 {
		udpFile := os.NewFile(*fd, "udpFile")
		fdConn, err := net.FileConn(udpFile)
		if err != nil {
			log.Printf("Error accessing UDP fd: %s\n", err.Error())
			return nil
		}
		listener = fdConn
	} else {
		udpAddr, err := net.ResolveUDPAddr("udp", addrStr)
		if err != nil {
			log.Printf("ResolveUDPAddr failed: %s\n", err.Error())
			return nil
		}
		listener, err = net.ListenUDP("udp", udpAddr)
		if err != nil {
			log.Printf("ListenUDP failed: %s\n", err.Error())
			return nil
		}
	}
	return &UdpInput{listener: &listener}
}

func (self *UdpInput) Init(config *PluginConfig) error {
	return nil
}

func (self *UdpInput) Read(pipelinePack *PipelinePack,
	timeout *time.Duration) error {
	self.deadline = time.Now().Add(*timeout)
	(*self.listener).SetReadDeadline(self.deadline)
	n, err := (*self.listener).Read(pipelinePack.MsgBytes)
	if err == nil {
		pipelinePack.MsgBytes = pipelinePack.MsgBytes[:n]
	}
	return err
}

// UdpGobInput
type UdpGobInput struct {
	listener *net.Conn
	deadline time.Time
	decoder  *gob.Decoder
}

func NewUdpGobInput(addrStr string, fd *uintptr) *UdpGobInput {
	var listener net.Conn
	if *fd != 0 {
		udpFile := os.NewFile(*fd, "udpFile")
		fdConn, err := net.FileConn(udpFile)
		if err != nil {
			log.Printf("Error accessing UDP fd: %s\n", err.Error())
			return nil
		}
		listener = fdConn
	} else {
		udpAddr, err := net.ResolveUDPAddr("udp", addrStr)
		if err != nil {
			log.Printf("ResolveUDPAddr failed: %s\n", err.Error())
			return nil
		}
		listener, err = net.ListenUDP("udp", udpAddr)
		if err != nil {
			log.Printf("ListenUDP failed: %s\n", err.Error())
			return nil
		}
	}
	decoder := gob.NewDecoder(listener)
	return &UdpGobInput{listener: &listener, decoder: decoder}
}

func (self *UdpGobInput) Init(config *PluginConfig) error {
	return nil
}

func (self *UdpGobInput) Read(pipelinePack *PipelinePack,
	timeout *time.Duration) error {
	self.deadline = time.Now().Add(*timeout)
	(*self.listener).SetReadDeadline(self.deadline)
	err := self.decoder.Decode(pipelinePack.Message)
	if err == nil {
		pipelinePack.Decoded = true
	}
	return err
}

// MessageGeneratorInput
type MessageGeneratorInput struct {
	messages chan *Message
}

func (self *MessageGeneratorInput) Init(config *PluginConfig) error {
	self.messages = make(chan *Message, 100)
	return nil
}

func (self *MessageGeneratorInput) Deliver(msg *Message) {
	newMessage := new(Message)
	msg.Copy(newMessage)
	self.messages <- newMessage
}

func (self *MessageGeneratorInput) Read(pipeline *PipelinePack,
	timeout *time.Duration) error {
	select {
	case msg := <-self.messages:
		pipeline.Message = msg
		pipeline.Decoded = true
		return nil
	case <-time.After(*timeout):
		return new(TimeoutError)
	}
	// shouldn't get here, compiler makes us have a return
	return nil
}
