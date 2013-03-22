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

type InputRunner interface {
	PluginRunner
	Input() Input
	Start(h PluginHelper, wg *sync.WaitGroup) (err error)
}

type iRunner struct {
	pRunnerBase
}

func NewInputRunner(name string, input Input) (ir InputRunner) {
	return &iRunner{pRunnerBase{name: name, plugin: input.(Plugin)}}
}
func (ir *iRunner) Input() Input {
	return ir.plugin.(Input)
}

func (ir *iRunner) Start(h PluginHelper, wg *sync.WaitGroup) (err error) {
	ir.inChan = h.PackSupply()
	if err = ir.Input().Start(ir, h, wg); err != nil {
		err = fmt.Errorf("Input '%s' failed to start: %s", ir.name, err)
	}
	return
}

func (ir *iRunner) LogError(err error) {
	log.Printf("Input '%s' error: %s", ir.name, err)
}

// Input plugin interface type
type Input interface {
	Start(ir InputRunner, h PluginHelper, wg *sync.WaitGroup) (err error)
	Stop()
}

// UdpInput
type UdpInput struct {
	listener net.Conn
	name     string
	stopped  bool
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

func (self *UdpInput) Start(ir InputRunner, h PluginHelper,
	wg *sync.WaitGroup) (err error) {

	decoders := h.DecodersByEncoding()
	decoder := decoders[Header_JSON] // TODO: Diff encodings over UDP
	if decoder == nil {
		return fmt.Errorf("No JSON decoder found.")
	}

	go func() {
		var err error
		var n int
		var pack *PipelinePack
		for !self.stopped {
			pack = <-ir.InChan()
			n, err = self.listener.Read(pack.MsgBytes)
			if err != nil {
				ir.LogError(fmt.Errorf("Read error: ", err))
				pack.Recycle()
				continue
			}
			pack.MsgBytes = pack.MsgBytes[:n]
			decoder.InChan() <- pack
		}
		self.listener.Close()
		for _, v := range decoders {
			close(v.InChan())
		}
		log.Println("UdpInput stopped: ", ir.Name())
		wg.Done()
	}()
	return nil
}

func (self *UdpInput) Stop() {
	self.stopped = true
}

// TCP Input

type TcpInput struct {
	listener net.Listener
	name     string
	wg       sync.WaitGroup
	stopChan chan bool
	ir       InputRunner
	h        PluginHelper
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

func (self *TcpInput) handleConnection(conn net.Conn) {
	buf := make([]byte, MAX_MESSAGE_SIZE+MAX_HEADER_SIZE)
	header := &Header{}
	var readPos, scanPos, posDelta int
	var pack *PipelinePack
	var msgOk bool

	packSupply := self.ir.InChan()
	decoders := self.h.DecodersByEncoding()
	var encoding Header_MessageEncoding

	var stopped bool
	for !stopped {
		select {
		case <-self.stopChan:
			stopped = true
			break
		default:
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
					stopped = true
					break
				}
				copy(buf, buf[scanPos:readPos]) // src and dst are allowed to overlap
				readPos, scanPos = readPos-scanPos, 0
			}
		}
	}

	conn.Close()
	for _, v := range decoders {
		close(v.InChan())
	}
	self.wg.Done()
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

func (self *TcpInput) Start(ir InputRunner, h PluginHelper,
	wg *sync.WaitGroup) (err error) {

	self.ir = ir
	self.h = h
	self.stopChan = make(chan bool)

	go func() {
		var conn net.Conn
		var err error

		for {
			if conn, err = self.listener.Accept(); err != nil {
				if err.(net.Error).Temporary() {
					self.ir.LogError(fmt.Errorf("TCP accept failed: %s", err))
					continue
				} else {
					break
				}
			}

			go self.handleConnection(conn)
			self.wg.Add(1)
		}
		log.Println("TcpInput stopped: ", ir.Name())
		wg.Done()
	}()

	return nil
}

func (self *TcpInput) Stop() {
	self.listener.Close()
	close(self.stopChan)
}

// Global MessageGenerator
var MessageGenerator = new(msgGenerator)

type msgGenerator struct {
	RouterChan  chan *messageHolder
	OutputChan  chan outputMsg
	RecycleChan chan *messageHolder
	hostname    string
	pid         int32
}

func (self *msgGenerator) Init() {
	self.RouterChan = make(chan *messageHolder, PoolSize/2)
	self.OutputChan = make(chan outputMsg, PoolSize/2)
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

// Injects a message using the MessageGenerator.
func (self *msgGenerator) Inject(msg *messageHolder) {
	self.RouterChan <- msg
}

// Sends a message directly to a specific output.
func (self *msgGenerator) Output(name string, msg *messageHolder) {
	outMsg := outputMsg{name, msg}
	self.OutputChan <- outMsg
}

// MessageGeneratorInput
type MessageGeneratorInput struct {
	routerChan  chan *messageHolder
	outputChan  chan outputMsg
	recycleChan chan *messageHolder
}

type messageHolder struct {
	Message  *Message
	RefCount int32
}

type outputMsg struct {
	outputName string
	msg        *messageHolder
}

func (self *MessageGeneratorInput) Init(config interface{}) error {
	MessageGenerator.Init()
	self.outputChan = MessageGenerator.OutputChan
	self.routerChan = MessageGenerator.RouterChan
	self.recycleChan = MessageGenerator.RecycleChan
	return nil
}

func (self *MessageGeneratorInput) Start(ir InputRunner, h PluginHelper,
	wg *sync.WaitGroup) (err error) {

	go func() {
		var pack *PipelinePack
		var msgHolder *messageHolder
		var outMsg outputMsg
		var output OutputRunner
		var outChan chan *PipelinePack
		ok := true
		packSupply := ir.InChan()

		for ok {
			pack = <-packSupply
			select {
			case msgHolder, ok = <-self.routerChan:
				// if !ok we'll fall through below
				outChan = h.Router().InChan
			case outMsg = <-self.outputChan:
				msgHolder = outMsg.msg
				if output, ok = h.Output(outMsg.outputName); !ok {
					ir.LogError(fmt.Errorf("No '%s' output", outMsg.outputName))
					ok = true // still deliver to the router
					outChan = h.Router().InChan
				} else {
					outChan = output.InChan()
				}
			}

			if ok {
				msgHolder.Message.Copy(pack.Message)
				pack.Decoded = true
				outChan <- pack
				cnt := atomic.AddInt32(&msgHolder.RefCount, -1)
				if cnt == 0 {
					self.recycleChan <- msgHolder
				}
			}
		}
		log.Println("MessageGeneratorInput stopped.")
		wg.Done()
	}()

	return
}

func (self *MessageGeneratorInput) Stop() {
	close(self.routerChan)
}
