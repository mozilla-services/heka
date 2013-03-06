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
	"code.google.com/p/go-uuid/uuid"
	"code.google.com/p/goprotobuf/proto"
	"fmt"
	. "github.com/mozilla-services/heka/message"
	"log"
	"net"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

type TimeoutError string

func (self *TimeoutError) Error() string {
	return fmt.Sprint("Error: Read timed out")
}

type Input interface {
	Start(helper PluginHelper, wg *sync.WaitGroup) error
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

func (self *UdpInput) Start(helper PluginHelper, wg *sync.WaitGroup) error {

	decoders := helper.NewDecoderSet()
	decoder := decoders[Header_JSON] // TODO: Diff encodings over UDP
	if decoder == nil {
		return fmt.Errorf("UdpInput error: No JSON decoder found.")
	}

	var stopped bool
	go func() {
		var pack *PipelinePack
		var err error
		var n int
		needOne := true
		packSupply := helper.PackSupply()
		for {
			if needOne {
				pack = <-packSupply
			}
			n, err = self.listener.Read(pack.MsgBytes)
			if err != nil {
				if stopped {
					break
				}
				log.Println("UdpInput read error: ", err)
				needOne = false
				pack.Zero()
				continue
			}
			pack.MsgBytes = pack.MsgBytes[:n]
			decoder.InChan() <- pack
			needOne = true
		}
	}()

	stopChan := helper.StopChan()
	go func() {
		_ = <-stopChan
		stopped = true
		self.listener.Close()
		for _, v := range decoders {
			close(v.InChan())
		}
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

func (self *TcpInput) handleConnection(helper PluginHelper, conn net.Conn) {
	defer conn.Close()

	buf := make([]byte, MAX_MESSAGE_SIZE+MAX_HEADER_SIZE)
	header := &Header{}
	var readPos, scanPos, posDelta int
	var pack *PipelinePack
	var msgOk bool

	packSupply := helper.PackSupply()
	decoders := helper.NewDecoderSet()
	var encoding Header_MessageEncoding

	for {
		n, err := conn.Read(buf[readPos:])
		if n > 0 {
			readPos += n
			for { // consume all available records
				pack = <-packSupply
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
					decoders[encoding].InChan() <- pack
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
	for _, v := range decoders {
		close(v.InChan())
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

func (self *TcpInput) listenForConnection(helper PluginHelper) {
	conn, err := self.listener.Accept()
	if err != nil {
		log.Println("TCP accept failed")
		return
	}
	go self.handleConnection(helper, conn)
}

func (self *TcpInput) Start(helper PluginHelper, wg *sync.WaitGroup) error {

	var stopped bool
	go func() {
		for {
			self.listenForConnection(helper)
			if stopped {
				break
			}
		}
	}()

	stopChan := helper.StopChan()
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
var MessageGenerator = new(msgGenerator)

type msgGenerator struct {
	MessageChan chan *messageHolder
	RecycleChan chan *messageHolder
	hostname    string
	pid         int32
}

func (self *msgGenerator) Init() {
	self.MessageChan = make(chan *messageHolder, PoolSize/2)
	self.RecycleChan = make(chan *messageHolder, PoolSize/2)
	for i := 0; i < PoolSize/2; i++ {
		msg := messageHolder{new(Message), 1}
		self.RecycleChan <- &msg
	}
	self.hostname, _ = os.Hostname()
	self.pid = int32(os.Getpid())
}

// Retrieve a message for use by the MessageGenerator.
func (self *msgGenerator) Retrieve() (msg *messageHolder) {
	msg = <-self.RecycleChan
	msg.Message.SetTimestamp(time.Now().UnixNano())
	msg.Message.SetUuid(uuid.NewRandom())
	msg.Message.SetHostname(self.hostname)
	msg.Message.SetPid(self.pid)
   msg.RefCount = 1
	return msg
}

// Injects a message using the MessageGenerator
func (self *msgGenerator) Inject(msg *messageHolder) {
	self.MessageChan <- msg
}

// MessageGeneratorInput
type MessageGeneratorInput struct {
	messageChan chan *messageHolder
	recycleChan chan *messageHolder
	name        string
}

type messageHolder struct {
	Message *Message
	RefCount int32
}

func (self *MessageGeneratorInput) Init(config interface{}) error {
	MessageGenerator.Init()
	self.messageChan = MessageGenerator.MessageChan
	self.recycleChan = MessageGenerator.RecycleChan
	return nil
}

func (self *MessageGeneratorInput) Name() string {
	return self.name
}

func (self *MessageGeneratorInput) SetName(name string) {
	self.name = name
}

func (self *MessageGeneratorInput) Start(helper PluginHelper,
	wg *sync.WaitGroup) error {

	var pack *PipelinePack
	packSupply := helper.PackSupply()
	stopChan := helper.StopChan()

	go func() {
	runnerLoop:
		for {
			select {
			case msgHolder := <-self.messageChan:
				select {
				case pack = <-packSupply:
					msgHolder.Message.Copy(pack.Message)
					pack.Decoded = true
					pack.Config.Router.InChan <- pack
					cnt := atomic.AddInt32(&msgHolder.RefCount, -1)
					if cnt == 0 {
						self.recycleChan <- msgHolder
					}
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
