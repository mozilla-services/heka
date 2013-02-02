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
	"bytes"
	"code.google.com/p/goprotobuf/proto"
	"fmt"
	. "github.com/mozilla-services/heka/message"
	"github.com/rafrombrc/go-notify"
	"log"
	"net"
	"os"
	"strconv"
	"sync"
	//"time"
)

const (
	MAX_HEADER_SIZE  = 255
	MAX_MESSAGE_SIZE = 64 * 1024
	RECORD_SEPARATOR = uint8(0x1e)
	UNIT_SEPARATOR   = uint8(0x1f)
)

type Input interface {
	Start(config *PipelineConfig, wg *sync.WaitGroup) error
	Name() string
	SetName(name string)
}

// UdpInput
type UdpInput struct {
	listener net.Conn
	name     string
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
		self.listener, err = net.FileConn(udpFile)
		if err != nil {
			return fmt.Errorf("Error accessing UDP fd: %s\n", err.Error())
		}
	} else {
		// IP address
		udpAddr, err := net.ResolveUDPAddr("udp", conf.Address)
		if err != nil {
			return fmt.Errorf("ResolveUDPAddr failed: %s\n", err.Error())
		}
		self.listener, err = net.ListenUDP("udp", udpAddr)
		if err != nil {
			return fmt.Errorf("ListenUDP failed: %s\n", err.Error())
		}
	}
	return nil
}

func (self *UdpInput) SetName(name string) {
	self.name = name
}

func (self *UdpInput) Name() string {
	return self.name
}

func (self *UdpInput) Start(config *PipelineConfig, wg *sync.WaitGroup) error {

	decoders := config.NewDecoderSet()
	decoder := decoders[Header_JSON] // TODO: Support for headers on UDP

	var stopped bool
	go func() {
		var pack *PipelinePack
		var err error
		var n int
		needOne := true
		for {
			if needOne {
				pack = <-config.RecycleChan
			}
			n, err = self.listener.Read(pack.MsgBytes)
			if err != nil {
				if stopped {
					break
				}
				log.Println("UdpInput read error: ", err)
				needOne = false
				continue
			}
			pack.MsgBytes = pack.MsgBytes[:n]
			decoder.InChan <- pack
		}
	}()

	stopChan := make(chan interface{})
	notify.Start(STOP, stopChan)
	go func() {
		_ = <-stopChan
		stopped = true
		self.listener.Close()
		log.Println("UdpInput stopped: ", self.name)
		wg.Done()
	}()

	return nil
}

// TCP Input

type TcpInput struct {
	listener net.Listener
	name     string
}

type TcpInputConfig struct {
	Address string
}

func (self *TcpInput) ConfigStruct() interface{} {
	return new(TcpInputConfig)
}

func (self *TcpInput) Name() string {
	return self.name
}

func (self *TcpInput) SetName(name string) {
	self.name = name
}

func decodeHeader(buf []byte, header *Header) bool {
	if buf[len(buf)-1] != UNIT_SEPARATOR {
		log.Println("missing unit separator")
		return false
	}
	err := proto.Unmarshal(buf[0:len(buf)-1], header)
	if err != nil {
		log.Println("error unmarshaling header:", err)
		return false
	}
	if header.GetMessageLength() > MAX_MESSAGE_SIZE {
		log.Printf("message exceeds the maximum length (bytes): %d", MAX_MESSAGE_SIZE)
		return false
	}
	return true
}

func findMessage(buf []byte, header *Header, message *[]byte) (pos int, ok bool) {
	ok = true
	pos = bytes.IndexByte(buf, RECORD_SEPARATOR)
	if pos != -1 {
		if len(buf) > 1 {
			headerLength := int(buf[pos+1])
			headerEnd := pos + headerLength + 3 // recsep+len+header+unitsep
			if len(buf) >= headerEnd {
				if header.MessageLength != nil || decodeHeader(buf[pos+2:headerEnd], header) {
					messageEnd := headerEnd + int(header.GetMessageLength())
					if len(buf) >= messageEnd {
						*message = (*message)[:messageEnd-headerEnd]
						copy(*message, buf[headerEnd:messageEnd])
						pos = messageEnd
					} else {
						ok = false
						*message = (*message)[:0]
					}
				} else {
					pos, ok = findMessage(buf[pos+1:], header, message)
				}
			}
		}
	} else {
		pos = len(buf)
	}
	return
}

func (self *TcpInput) handleConnection(config *PipelineConfig, conn net.Conn) {
	defer conn.Close()

	buf := make([]byte, MAX_MESSAGE_SIZE+MAX_HEADER_SIZE)
	header := &Header{}
	var readPos, scanPos, posDelta int
	var pack *PipelinePack
	var msgOk bool

	decoders := config.NewDecoderSet()
	var encoding Header_MessageEncoding

	for {
		n, err := conn.Read(buf[readPos:])
		if n > 0 {
			readPos += n
			for { // consume all available records
				pack = <-config.RecycleChan
				posDelta, msgOk = findMessage(buf[scanPos:readPos], header, &(pack.MsgBytes))
				scanPos += posDelta

				if header.MessageLength == nil {
					// incomplete header, recycle the pack and bail
					pack.Recycle()
					break
				}

				if header.GetMessageLength() != uint32(len(pack.MsgBytes)) {
					// incomplete message, recycle the pack and bail
					pack.Recycle()
					break
				}

				if msgOk {
					encoding = header.GetMessageEncoding()
					decoders[encoding].InChan <- pack
				}

				header.Reset()
			}
		}
		if err != nil {
			break
		}
		// make room at the end of the buffer
		if (header.MessageLength != nil &&
			int(header.GetMessageLength())+scanPos+MAX_HEADER_SIZE > cap(buf)) ||
			cap(buf)-scanPos < MAX_HEADER_SIZE {
			if scanPos == 0 { // out of buffer, dump the connection to the bad client
				return
			}
			copy(buf, buf[scanPos:readPos]) // src and dst are allowed to overlap
			readPos, scanPos = readPos-scanPos, 0
		}
	}
}

func (self *TcpInput) Init(config interface{}) error {
	var err error
	conf := config.(*TcpInputConfig)
	self.listener, err = net.Listen("tcp", conf.Address)
	if err != nil {
		return fmt.Errorf("ListenTCP failed: %s\n", err.Error())
	}
	return nil
}

func (self *TcpInput) Start(config *PipelineConfig, wg *sync.WaitGroup) error {

	var stopped bool
	go func() {
		for {
			conn, err := self.listener.Accept()
			if err != nil {
				if stopped {
					break
				}
				log.Println("TCP accept failed")
				continue
			}
			go self.handleConnection(config, conn)
		}
	}()

	stopChan := make(chan interface{})
	notify.Start(STOP, stopChan)
	go func() {
		_ = <-stopChan
		stopped = true
		self.listener.Close()
		log.Println("TcpInput stopped: ", self.name)
		wg.Done()
	}()

	return nil
}

// Global MessageGenerator
var MessageGenerator *msgGenerator = new(msgGenerator)

type msgGenerator struct {
	MessageChan chan *messageHolder
	RecycleChan chan *messageHolder
}

func (self *msgGenerator) Init() {
	self.MessageChan = make(chan *messageHolder, PoolSize/2)
	self.RecycleChan = make(chan *messageHolder, PoolSize/2)
	for i := 0; i < PoolSize/2; i++ {
		msg := messageHolder{new(Message), 0}
		self.RecycleChan <- &msg
	}
}

// Retrieve a message for use by the MessageGenerator.
// Must be passed the current pipeline.ChainCount.
//
// This is actually a messageHolder object that has a message and
// chainCount. The chainCount should remain untouched, and all the
// fields of the returned msg.Message should be overwritten as needed
// The msg.Message
func (self *msgGenerator) Retrieve(chainCount int) (msg *messageHolder) {
	msg = <-self.RecycleChan
	msg.ChainCount = chainCount
	return msg
}

// Injects a message using the MessageGenerator
func (self *msgGenerator) Inject(msg *messageHolder) {
	msg.ChainCount++
	self.MessageChan <- msg
}

// MessageGeneratorInput
type MessageGeneratorInput struct {
	messageChan chan *messageHolder
	recycleChan chan *messageHolder
}

type messageHolder struct {
	Message    *Message
	ChainCount int
}

func (self *MessageGeneratorInput) Init(config interface{}) error {
	MessageGenerator.Init()
	self.messageChan = MessageGenerator.MessageChan
	self.recycleChan = MessageGenerator.RecycleChan
	return nil
}

func (self *MessageGeneratorInput) Start(config *PipelineConfig,
	wg *sync.WaitGroup) error {

	var pack *PipelinePack
	chainRouter := config.ChainRouter()
	stopChan := make(chan interface{})
	notify.Start(STOP, stopChan)

	go func() {
	runnerLoop:
		for {
			select {
			case msgHolder := <-self.messageChan:
				select {
				case pack = <-config.RecycleChan:
					msgHolder.Message.Copy(pack.Message)
					pack.Decoded = true
					pack.ChainCount = msgHolder.ChainCount
					chainRouter.InChan <- pack
					self.recycleChan <- msgHolder
				case <-stopChan:
					break runnerLoop
				}
			case <-stopChan:
				break runnerLoop
			}
		}
	}()
	wg.Done()
	return nil
}
