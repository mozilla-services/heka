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
	"fmt"
	"github.com/rafrombrc/go-notify"
	. "heka/message"
	"log"
	"net"
	"os"
	"runtime"
	"strconv"
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

// An InputRunner wraps each Input plugin and starts a goroutine for waiting
// for message data to come in via that input. It also listens for STOP events
// to cleanly exit the goroutine.
type InputRunner struct {
	name    string
	input   Input
	timeout *time.Duration
}

func (self *InputRunner) Start(pipeline func(*PipelinePack),
	recycleChan <-chan *PipelinePack, wg *sync.WaitGroup) {

	stopChan := make(chan interface{})
	notify.Start(STOP, stopChan)

	go func() {
		var err error
		var pack *PipelinePack
		needOne := true
	runnerLoop:
		for {
			if needOne {
				runtime.Gosched()
				select {
				case pack = <-recycleChan:
				case <-stopChan:
					break runnerLoop
				}
			}

			err = self.input.Read(pack, self.timeout)
			if err != nil {
				select {
				case <-stopChan:
					break runnerLoop
				default:
					needOne = false
					continue
				}
			}
			go pipeline(pack)
			needOne = true
		}
		log.Println("Input stopped: ", self.name)
		wg.Done()
	}()
}

// UdpInput

// Deadline is stored on the struct so we don't have to allocate / GC
// a new time.Time object for each message received.
type UdpInput struct {
	Listener net.Conn
	Deadline time.Time
}

type UdpInputConfig struct {
	Address string
}

func (self *UdpInput) ConfigStruct() interface{} {
	return new(UdpInputConfig)
}

func (self *UdpInput) Init(config interface{}) error {
	conf := config.(*UdpInputConfig)
	if len(conf.Address) > 3 && conf.Address[:3] == "fd:" {
		// File descriptor
		fdStr := conf.Address[3:]
		fdInt, err := strconv.ParseUint(fdStr, 0, 0)
		if err != nil {
			log.Println(err)
			return fmt.Errorf("Invalid file descriptor: %s", conf.Address)
		}
		fd := uintptr(fdInt)
		udpFile := os.NewFile(fd, "udpFile")
		self.Listener, err = net.FileConn(udpFile)
		if err != nil {
			return fmt.Errorf("Error accessing UDP fd: %s\n", err.Error())
		}
	} else {
		// IP address
		udpAddr, err := net.ResolveUDPAddr("udp", conf.Address)
		if err != nil {
			return fmt.Errorf("ResolveUDPAddr failed: %s\n", err.Error())
		}
		self.Listener, err = net.ListenUDP("udp", udpAddr)
		if err != nil {
			return fmt.Errorf("ListenUDP failed: %s\n", err.Error())
		}
	}
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

// MessageGeneratorInput

type MessageGeneratorInput struct {
	messages chan *messageHolder
}

type messageHolder struct {
	message    *Message
	chainCount int
}

func (self *MessageGeneratorInput) Init(config interface{}) error {
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
