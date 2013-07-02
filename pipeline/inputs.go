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
	"time"
)

// Heka PluginRunner for Input plugins.
type InputRunner interface {
	PluginRunner
	// Input channel from which Inputs can get fresh PipelinePacks, ready to
	// be populated.
	InChan() chan *PipelinePack
	// Associated Input plugin object.
	Input() Input

	SetTickLength(tickLength time.Duration)

	// Returns a ticker channel configured to send ticks at an interval
	// specified by the plugin's ticker_interval config value, if provided.
	Ticker() (ticker <-chan time.Time)

	// Starts Input in a separate goroutine and returns. Should decrement the
	// plugin when the Input stops and the goroutine has completed.
	Start(h PluginHelper, wg *sync.WaitGroup) (err error)
	// Injects PipelinePack into the Heka Router's input channel for delivery
	// to all Filter and Output plugins with corresponding message_matchers.
	Inject(pack *PipelinePack)
}

type iRunner struct {
	pRunnerBase
	input      Input
	inChan     chan *PipelinePack
	tickLength time.Duration
	ticker     <-chan time.Time
}

func (ir *iRunner) SetTickLength(tickLength time.Duration) {
	ir.tickLength = tickLength
}

func (ir *iRunner) Ticker() (ticker <-chan time.Time) {
	return ir.ticker
}

// Creates and returns a new (not yet started) InputRunner associated w/ the
// provided Input.
func NewInputRunner(name string, input Input, pluginGlobals *PluginGlobals) (
	ir InputRunner) {
	return &iRunner{
		pRunnerBase: pRunnerBase{
			name:          name,
			plugin:        input.(Plugin),
			pluginGlobals: pluginGlobals,
		},
		input: input,
	}
}

func (ir *iRunner) Input() Input {
	return ir.plugin.(Input)
}

func (ir *iRunner) InChan() chan *PipelinePack {
	return ir.inChan
}

func (ir *iRunner) Start(h PluginHelper, wg *sync.WaitGroup) (err error) {
	ir.h = h
	ir.inChan = h.PipelineConfig().inputRecycleChan

	if ir.tickLength != 0 {
		ir.ticker = time.Tick(ir.tickLength)
	}

	go ir.Starter(h, wg)
	return
}

func (ir *iRunner) Starter(h PluginHelper, wg *sync.WaitGroup) {
	defer func() {
		if r := recover(); r != nil {
			// Panics in separate goroutines that are spun up by the input
			// will still bring the process down, but this protects us at
			// least a little bit. :P
			ir.LogError(fmt.Errorf("PANIC: %s", r))
		}
		wg.Done()
	}()

	globals := Globals()
	rh, err := NewRetryHelper(ir.pluginGlobals.Retries)
	if err != nil {
		ir.LogError(err)
		globals.ShutDown()
		return
	}

	for !globals.Stopping {
		// ir.Input().Run() shouldn't return unless error or shutdown
		if err := ir.Input().Run(ir, h); err != nil {
			ir.LogError(err)
		} else {
			ir.LogMessage("stopped")
		}

		// Are we supposed to stop? Save ourselves some time by exiting now
		if globals.Stopping {
			return
		}

		// We stop and let this quit if its not a restarting plugin
		if recon, ok := ir.plugin.(Restarting); ok {
			recon.CleanupForRestart()
		} else {
			ir.LogMessage("has stopped, shutting down.")
			globals.ShutDown()
			return
		}

		// Re-initialize our plugin using its wrapper
		pw := h.PipelineConfig().inputWrappers[ir.name]

		// Attempt to recreate the plugin until it works without error
		// or until we were told to stop
	createLoop:
		for !globals.Stopping {
			err := rh.Wait()
			if err != nil {
				ir.LogError(err)
				globals.ShutDown()
				return
			}
			p, err := pw.CreateWithError()
			if err != nil {
				ir.LogError(err)
				continue
			}
			ir.plugin = p.(Plugin)
			rh.Reset()
			break createLoop
		}
		ir.LogMessage("exited, now restarting.")
	}
}

func (ir *iRunner) Inject(pack *PipelinePack) {
	ir.h.PipelineConfig().router.InChan() <- pack
}

func (ir *iRunner) LogError(err error) {
	log.Printf("Input '%s' error: %s", ir.name, err)
}

func (ir *iRunner) LogMessage(msg string) {
	log.Printf("Input '%s': %s", ir.name, msg)
}

// Input plugin interface type.
type Input interface {
	// Start listening for / gathering incoming data, populating
	// PipelinePacks, and passing them along to a Decoder or the Router as
	// appropriate.
	Run(ir InputRunner, h PluginHelper) (err error)
	// Called as a signal to the Input to stop listening for / gathering
	// incoming data and to perform any necessary clean-up.
	Stop()
}

// Input plugin implementation that listens for Heka protocol messages on a
// specified UDP socket.
type UdpInput struct {
	listener net.Conn
	name     string
	stopped  bool
	config   *UdpInputConfig
}

// ConfigStruct for UdpInput plugin.
type UdpInputConfig struct {
	// String representation of the address of the UDP connection on which
	// the listener should be listening (e.g. "127.0.0.1:5565").
	Address string `toml:"address"`
	// Set of message signer objects, keyed by signer id string.
	Signers map[string]Signer `toml:"signer"`
}

func (self *UdpInput) ConfigStruct() interface{} {
	return new(UdpInputConfig)
}

func (self *UdpInput) Init(config interface{}) error {
	self.config = config.(*UdpInputConfig)
	if len(self.config.Address) > 3 && self.config.Address[:3] == "fd:" {
		// File descriptor
		fdStr := self.config.Address[3:]
		fdInt, err := strconv.ParseUint(fdStr, 0, 0)
		if err != nil {
			log.Println(err)
			return fmt.Errorf("Invalid file descriptor: %s", self.config.Address)
		}
		fd := uintptr(fdInt)
		udpFile := os.NewFile(fd, "udpFile")
		self.listener, err = net.FileConn(udpFile)
		if err != nil {
			return fmt.Errorf("Error accessing UDP fd: %s\n", err.Error())
		}
	} else {
		// IP address
		udpAddr, err := net.ResolveUDPAddr("udp", self.config.Address)
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
	var decoders []DecoderRunner
	if decoders, err = h.DecoderSet().ByEncodings(); err != nil {
		return
	}
	buf := make([]byte, MAX_MESSAGE_SIZE+MAX_HEADER_SIZE+3)
	header := &Header{}

	var (
		e        error
		n        int
		pack     *PipelinePack
		msgOk    bool
		decoder  DecoderRunner
		encoding Header_MessageEncoding
	)
	for !self.stopped {
		pack = <-ir.InChan()
		if n, e = self.listener.Read(buf); e != nil {
			if !strings.Contains(e.Error(), "use of closed") {
				ir.LogError(fmt.Errorf("Read error: ", e))
			}
			pack.Recycle()
			continue
		}
		_, msgOk = findMessage(buf[:n], header, &(pack.MsgBytes))
		if msgOk {
			if authenticateMessage(self.config.Signers, header, pack) {
				encoding = header.GetMessageEncoding()
				if decoder = decoders[encoding]; decoder != nil {
					decoder.InChan() <- pack
				} else {
					ir.LogError(fmt.Errorf("No decoder registered for encoding: %s",
						encoding.String()))
					pack.Recycle()
				}
			} else {
				pack.Recycle()
			}
		} else {
			pack.Recycle()
		}
		header.Reset()
	}

	self.listener.Close()
	return
}

func (self *UdpInput) Stop() {
	self.stopped = true
	self.listener.Close()
}

// Input plugin implementation that listens for Heka protocol messages on a
// specified TCP socket. Creates a separate goroutine for each TCP connection.
type TcpInput struct {
	listener net.Listener
	name     string
	wg       sync.WaitGroup
	stopChan chan bool
	ir       InputRunner
	h        PluginHelper
	config   *TcpInputConfig
}

// Heka Message signer object.
type Signer struct {
	HmacKey string `toml:"hmac_key"`
}

// ConfigStruct for TcpInput plugin.
type TcpInputConfig struct {
	// String representation of the address of the TCP connection on which
	// the listener should be listening (e.g. "127.0.0.1:5565").
	Address string
	// Set of message signer objects, keyed by signer id string.
	Signers map[string]Signer `toml:"signer"`
}

func (self *TcpInput) ConfigStruct() interface{} {
	return new(TcpInputConfig)
}

// Decodes provided byte slice into a Heka protocol header object.
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

// Scans provided byte slice for the next record separator and tries to
// populate provided header and message objects using the scanned data.
// Returns new starting index into the passed buffer, or ok == false if no
// message was able to be extracted.
func findMessage(buf []byte, header *Header, message *[]byte) (pos int, ok bool) {
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
						ok = true
					} else {
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

// Returns true if the provided message is unsigned or has a valid signature
// from one of the provided signers. If signed, the signer name is added to
// the PipelinePack.
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

// Listen on the provided TCP connection, extracting Heka protocol messages
// from the incoming data until the connection is closed or Stop is called on
// the input.
func (self *TcpInput) handleConnection(conn net.Conn) {
	buf := make([]byte, MAX_MESSAGE_SIZE+MAX_HEADER_SIZE+3)
	header := &Header{}
	var (
		readPos, scanPos, posDelta int
		pack                       *PipelinePack
		encoding                   Header_MessageEncoding
		decoder                    DecoderRunner
		ok, stopped                bool
	)

	packSupply := self.ir.InChan()
	decoders, err := self.h.DecoderSet().ByEncodings()
	if err != nil {
		self.ir.LogError(fmt.Errorf("Error getting decoders: %s", err))
	}

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
					posDelta, ok = findMessage(buf[scanPos:readPos], header, &(pack.MsgBytes))
					scanPos += posDelta

					// Recycle pack and bail if incomplete header or incomplete message.
					if header.MessageLength == nil ||
						header.GetMessageLength() != uint32(len(pack.MsgBytes)) {
						pack.Recycle()
						break
					}
					if ok {
						if authenticateMessage(self.config.Signers, header, pack) {
							encoding = header.GetMessageEncoding()
							if decoder = decoders[encoding]; decoder != nil {
								decoder.InChan() <- pack
							} else {
								err := fmt.Errorf("No decoder registered for encoding: %s",
									encoding.String())
								self.ir.LogError(err)
								pack.Recycle()
							}
						} else {
							pack.Recycle()
						}
					} else {
						pack.Recycle()
					}
					header.Reset()
				}
			}
			if err != nil {
				stopped = true
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
		self.wg.Add(1)
		go self.handleConnection(conn)
	}
	self.wg.Wait()
	return
}

func (self *TcpInput) Stop() {
	self.listener.Close()
	close(self.stopChan)
}
