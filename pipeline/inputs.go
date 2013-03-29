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
	"crypto/hmac"
	"crypto/md5"
	"crypto/sha1"
	"fmt"
	. "github.com/mozilla-services/heka/message"
	"hash"
	"log"
	"net"
	"os"
	"strconv"
	"strings"
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
	InChan() chan *PipelinePack
	Input() Input
	Start(h PluginHelper, wg *sync.WaitGroup) (err error)
}

type iRunner struct {
	pRunnerBase
	inChan chan *PipelinePack
}

func NewInputRunner(name string, input Input) InputRunner {
	iRunner := new(iRunner)
	iRunner.name = name
	iRunner.plugin = input.(Plugin)
	return iRunner
}

func (ir *iRunner) Input() Input {
	return ir.plugin.(Input)
}

func (ir *iRunner) InChan() chan *PipelinePack {
	return ir.inChan
}

func (ir *iRunner) Start(h PluginHelper, wg *sync.WaitGroup) (err error) {
	ir.inChan = h.PackSupply()
	go func() {
		defer func() {
			if r := recover(); r != nil {
				// Panics in separate goroutines that are spun up by the input
				// will still bring the process down, but this protects us at
				// least a little bit. :P
				ir.LogError(fmt.Errorf("PANIC: %s", r))
			}
			wg.Done()
		}()

		// ir.Input().Run() shouldn't return unless error or shutdown
		if err = ir.Input().Run(ir, h); err != nil {
			err = fmt.Errorf("Input '%s' error: %s", ir.name, err)
		} else {
			ir.LogMessage("stopped")
		}
	}()
	return
}

func (ir *iRunner) LogError(err error) {
	log.Printf("Input '%s' error: %s", ir.name, err)
}

func (ir *iRunner) LogMessage(msg string) {
	log.Printf("Input '%s': %s", ir.name, msg)
}

// Input plugin interface type
type Input interface {
	Run(ir InputRunner, h PluginHelper) (err error)
	Stop()
}

// UdpInput
type UdpInput struct {
	listener net.Conn
	name     string
	stopped  bool
}

type UdpInputConfig struct {
	Address string `toml:"address"`
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

func (self *UdpInput) Run(ir InputRunner, h PluginHelper) (err error) {
	decoders := h.DecodersByEncoding()
	decoder := decoders[Header_JSON] // TODO: Diff encodings over UDP
	if decoder == nil {
		return fmt.Errorf("No JSON decoder found.")
	}

	var e error
	var n int
	var pack *PipelinePack
	for !self.stopped {
		pack = <-ir.InChan()
		if n, e = self.listener.Read(pack.MsgBytes); e != nil {
			if !strings.Contains(e.Error(), "use of closed") {
				ir.LogError(fmt.Errorf("Read error: ", e))
			}
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

	return
}

func (self *UdpInput) Stop() {
	self.stopped = true
	self.listener.Close()
}

// TCP Input

type TcpInput struct {
	listener net.Listener
	name     string
	wg       sync.WaitGroup
	stopChan chan bool
	ir       InputRunner
	h        PluginHelper
	config   *TcpInputConfig
}

type Signer struct {
	HmacKey string `toml:"hmac_key"`
}

type TcpInputConfig struct {
	Address string
	Signers map[string]Signer
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

func authenticateMessage(signers map[string]Signer, header *Header,
	pack *PipelinePack) bool {
	digest := header.GetHmac()
	if digest != nil {
		var key string
		signer := fmt.Sprintf("%s_%d", header.GetHmacSigner(),
			header.GetHmacKeyVersion())
		if s, ok := signers[signer]; ok {
			key = s.HmacKey
		} else {
			return false
		}

		var hm hash.Hash
		switch header.GetHmacHashFunction() {
		case Header_MD5:
			hm = hmac.New(md5.New, []byte(key))
		case Header_SHA1:
			hm = hmac.New(sha1.New, []byte(key))
		}
		hm.Write(pack.MsgBytes)
		expectedDigest := hm.Sum(nil)
		if bytes.Compare(digest, expectedDigest) != 0 {
			return false
		}
		pack.Signer = header.GetHmacSigner()
	}
	return true
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
						if authenticateMessage(self.config.Signers, header, pack) {
							encoding = header.GetMessageEncoding()
							decoders[encoding].InChan() <- pack
						} else {
							pack.Recycle()
						}
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
	self.config = config.(*TcpInputConfig)
	self.listener, err = net.Listen("tcp", self.config.Address)
	if err != nil {
		return fmt.Errorf("ListenTCP failed: %s\n", err.Error())
	}
	return nil
}

func (self *TcpInput) Run(ir InputRunner, h PluginHelper) (err error) {
	self.ir = ir
	self.h = h
	self.stopChan = make(chan bool)

	var conn net.Conn
	var e error

	for {
		if conn, e = self.listener.Accept(); e != nil {
			if e.(net.Error).Temporary() {
				self.ir.LogError(fmt.Errorf("TCP accept failed: %s", e))
				continue
			} else {
				break
			}
		}

		go self.handleConnection(conn)
		self.wg.Add(1)
	}

	return
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

func (self *MessageGeneratorInput) Run(ir InputRunner, h PluginHelper) (err error) {
	var pack *PipelinePack
	var msgHolder *messageHolder
	var outMsg outputMsg
	var output OutputRunner
	var outChan chan *PipelinePack
	ok := true
	packSupply := ir.InChan()

	for ok {
		output = nil
		outChan = nil
		pack = <-packSupply
		select {
		case msgHolder, ok = <-self.routerChan:
			// if !ok we'll fall through below
			outChan = h.Router().InChan
		case outMsg = <-self.outputChan:
			msgHolder = outMsg.msg
			if output, ok = h.Output(outMsg.outputName); !ok {
				ir.LogError(fmt.Errorf("No '%s' output", outMsg.outputName))
				ok = true // still deliver to the router; is this what we want?
				outChan = h.Router().InChan
			}
		}

		if ok {
			msgHolder.Message.Copy(pack.Message)
			pack.Decoded = true
			if output != nil {
				output.Deliver(pack)
			} else {
				outChan <- pack
			}
			cnt := atomic.AddInt32(&msgHolder.RefCount, -1)
			if cnt == 0 {
				self.recycleChan <- msgHolder
			}
		}
	}

	return
}

func (self *MessageGeneratorInput) Stop() {
	close(self.routerChan)
}
