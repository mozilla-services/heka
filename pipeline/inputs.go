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
	"runtime"
	"strconv"
	"sync"
	"time"
)

const (
	MAX_HEADER_SIZE  = 255
	MAX_MESSAGE_SIZE = 64 * 1024
	RECORD_SEPARATOR = uint8(0x1e)
	UNIT_SEPARATOR   = uint8(0x1f)
)

type TimeoutError string

func (self *TimeoutError) Error() string {
	return fmt.Sprint("Error: Read timed out")
}

type Input interface {
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

func (self *InputRunner) Start(dataChan chan *PipelinePack,
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
				runtime.Gosched()
				select {
				case <-stopChan:
					break runnerLoop
				default:
					needOne = false
					continue
				}
			}
			dataChan <- pack
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

// TCP Input
type TcpInput struct {
	Listener    net.Listener
	messageChan chan *Message
}

type TcpInputConfig struct {
	Address string
}

func (self *TcpInput) ConfigStruct() interface{} {
	return new(TcpInputConfig)
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

func findMessage(buf []byte, header *Header, message *[]byte) (pos int) {
	pos = bytes.IndexByte(buf, RECORD_SEPARATOR)
	if pos != -1 {
		if len(buf) > 1 {
			headerLength := int(buf[pos+1])
			headerEnd := pos + headerLength + 3 // recsep+len+header+unitsep
			if len(buf) >= headerEnd {
				if header.MessageLength != nil || decodeHeader(buf[pos+2:headerEnd], header) {
					messageEnd := headerEnd + int(header.GetMessageLength())
					if len(buf) >= messageEnd {
						*message = buf[headerEnd:messageEnd]
						pos = messageEnd
					} else {
						*message = nil
					}
				} else {
					pos = findMessage(buf[pos+1:], header, message)
				}
			}
		}
	} else {
		pos = len(buf)
	}
	return
}

func (self *TcpInput) handleConnection(conn net.Conn) {
	defer conn.Close()

	buf := make([]byte, MAX_MESSAGE_SIZE+MAX_HEADER_SIZE)
	header := &Header{}
	var message []byte
	var readPos, scanPos int

	for {
		n, err := conn.Read(buf[readPos:])
		if n > 0 {
			readPos += n
			for { // consume all available records
				scanPos += findMessage(buf[scanPos:readPos], header, &message)
				if header.MessageLength != nil {
					if header.GetMessageLength() == 0 || (header.GetMessageLength() > 0 && message != nil) {
						if message != nil {
							/// @review by decoding here we are subverting the pipeline
							/// however, this keeps the messages from each host in order and
							/// allows parallel decoding across hosts
							/// the pipeline pack also allocates a MsgBytes buffer and initializes
							/// a Message struct; they are both unnecessary in this case
							msg := &Message{}
							switch header.GetMessageEncoding() {
							case Header_PROTOCOL_BUFFER:
								err := proto.Unmarshal(message, msg)
								if err == nil {
									//                                    log.Println("send msg")
									self.messageChan <- msg
								} else {
									log.Println("invalid Protobuf message received")
								}
							}
						}
						header.Reset()
						message = nil
					} else {
						break // incomplete message
					}
				} else {
					break // incomplete header
				}
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

func (self *TcpInput) acceptConnections() {
	for { /// @todo cleanly tear this down on exit?
		conn, err := self.Listener.Accept()
		if err != nil {
			log.Println("TCP accept failed")
			continue
		}
		go self.handleConnection(conn)
	}
}

func (self *TcpInput) Init(config interface{}) error {
	var err error
	conf := config.(*TcpInputConfig)
	self.Listener, err = net.Listen("tcp", conf.Address)
	if err != nil {
		return fmt.Errorf("ListenTCP failed: %s\n", err.Error())
	}
	self.messageChan = make(chan *Message, 1000000)
	go self.acceptConnections()
	return nil
}

func (self *TcpInput) Read(pipelinePack *PipelinePack,
	timeout *time.Duration) error {
	select {
	case pipelinePack.Message = <-self.messageChan:
		//        log.Println("read message")
		pipelinePack.Decoded = true
	case <-time.After(*timeout):
		//        log.Println("no messages to read")
		err := TimeoutError("No messages to read")
		return &err
	}
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
	msg         *messageHolder
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

func (self *MessageGeneratorInput) Read(pipeline *PipelinePack,
	timeout *time.Duration) error {
	select {
	case msgHolder := <-self.messageChan:
		msgHolder.Message.Copy(pipeline.Message)
		pipeline.Decoded = true
		pipeline.ChainCount = msgHolder.ChainCount
		self.recycleChan <- msgHolder
		return nil
	case <-time.After(*timeout):
		err := TimeoutError("No messages to read")
		return &err
	}
	// shouldn't get here, compiler makes us have a return
	return nil
}
