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
	Listener net.Conn
	Deadline time.Time
}

func NewUdpInput(addrStr string, fd *uintptr) *UdpInput {
	var listener net.Conn
	if fd != nil && *fd != 0 {
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
	return &UdpInput{Listener: listener}
}

func (self *UdpInput) Init(config *PluginConfig) error {
	return nil
}

func (self *UdpInput) Read(pipelinePack *PipelinePack,
	timeout *time.Duration) error {
	self.Deadline = time.Now().Add(*timeout)
	self.Listener.SetReadDeadline(self.Deadline)
	n, err := self.Listener.Read(pipelinePack.MsgBytes)
	if err == nil {
		pipelinePack.MsgBytes = pipelinePack.MsgBytes[:n]
	}
	return err
}

// UdpGobInput
type UdpGobInput struct {
	Listener net.Conn
	Deadline time.Time
	Decoder  *gob.Decoder
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
	return &UdpGobInput{Listener: listener, Decoder: decoder}
}

func (self *UdpGobInput) Init(config *PluginConfig) error {
	return nil
}

func (self *UdpGobInput) Read(pipelinePack *PipelinePack,
	timeout *time.Duration) error {
	self.Deadline = time.Now().Add(*timeout)
	self.Listener.SetReadDeadline(self.Deadline)
	err := self.Decoder.Decode(pipelinePack.Message)
	if err == nil {
		pipelinePack.Decoded = true
	}
	return err
}

// MessageGeneratorInput
type messageHolder struct {
	message    *Message
	chainCount int
}

type MessageGeneratorInput struct {
	messages chan *messageHolder
}

func (self *MessageGeneratorInput) Init(config *PluginConfig) error {
	self.messages = make(chan *messageHolder, 100)
	return nil
}

func (self *MessageGeneratorInput) Deliver(msg *Message, chainCount int) {
	newMessage := new(Message)
	msg.Copy(newMessage)
	msgHolder := messageHolder{newMessage, chainCount + 1}
	self.messages <- &msgHolder
}

func (self *MessageGeneratorInput) Read(pipeline *PipelinePack,
	timeout *time.Duration) error {
	select {
	case msgHolder := <-self.messages:
		pipeline.Message = msgHolder.message
		pipeline.Decoded = true
		pipeline.ChainCount = msgHolder.chainCount
		return nil
	case <-time.After(*timeout):
		err := TimeoutError("No messages to read")
		return &err
	}
	// shouldn't get here, compiler makes us have a return
	return nil
}
