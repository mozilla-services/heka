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
	"crypto/subtle"
	"fmt"
	. "github.com/mozilla-services/heka/message"
	"hash"
	"io"
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
		h.PipelineConfig().inputsLock.Lock()
		pw := h.PipelineConfig().inputWrappers[ir.name]
		h.PipelineConfig().inputsLock.Unlock()

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

// ConfigStruct for NetworkInput plugins.
type NetworkInputConfig struct {
	// String representation of the address of the network connection on which
	// the listener should be listening (e.g. "127.0.0.1:5565").
	Address string
	// Set of message signer objects, keyed by signer id string.
	Signers map[string]Signer `toml:"signer"`
	// Name of configured decoder to receive the input
	Decoder string
	// Type of parser used to break the stream up into messages
	ParserType string `toml:"parser_type"`
	// Delimiter used to split the stream into messages
	Delimiter string
	// String indicating if the delimiter is at the start or end of the line,
	// only used for regexp delimiters
	DelimiterLocation string `toml:"delimiter_location"`
}

type networkParseFunction func(conn net.Conn,
	parser StreamParser,
	ir InputRunner,
	config *NetworkInputConfig,
	dr DecoderRunner) (err error)

// Standard text log file parser
func networkPayloadParser(conn net.Conn,
	parser StreamParser,
	ir InputRunner,
	config *NetworkInputConfig,
	dr DecoderRunner) (err error) {
	var (
		pack   *PipelinePack
		record []byte
	)
	_, record, err = parser.Parse(conn)
	if len(record) > 0 {
		pack = <-ir.InChan()
		pack.Message.SetUuid(uuid.NewRandom())
		pack.Message.SetTimestamp(time.Now().UnixNano())
		pack.Message.SetType("NetworkInput")
		pack.Message.SetSeverity(int32(0))
		pack.Message.SetEnvVersion("0.8")
		pack.Message.SetPid(0)
		// Only TCP packets have a remote address.
		if remoteAddr := conn.RemoteAddr(); remoteAddr != nil {
			pack.Message.SetHostname(remoteAddr.String())
		}
		pack.Message.SetLogger(ir.Name())
		pack.Message.SetPayload(string(record))
		if dr == nil {
			ir.Inject(pack)
		} else {
			dr.InChan() <- pack
		}
	}
	return
}

// Framed protobuf message parser
func networkMessageProtoParser(conn net.Conn,
	parser StreamParser,
	ir InputRunner,
	config *NetworkInputConfig,
	dr DecoderRunner) (err error) {
	var (
		pack   *PipelinePack
		record []byte
	)
	_, record, err = parser.Parse(conn)
	if err != nil {
		if err == io.ErrShortBuffer {
			ir.LogError(fmt.Errorf("record exceeded MAX_RECORD_SIZE %d", MAX_RECORD_SIZE))
			err = nil // non-fatal, keep going
		}
	}
	if len(record) > 0 {
		pack = <-ir.InChan()
		headerLen := int(record[1]) + HEADER_FRAMING_SIZE
		messageLen := len(record) - headerLen
		if headerLen > UUID_SIZE {
			header := new(Header)
			decodeHeader(record[2:headerLen], header)
			if authenticateMessage(config.Signers, header, record[headerLen:]) {
				pack.Signer = header.GetHmacSigner()
			} else {
				pack.Recycle()
				return
			}
		}
		if messageLen > cap(pack.MsgBytes) {
			pack.MsgBytes = make([]byte, messageLen)
		}
		pack.MsgBytes = pack.MsgBytes[:messageLen]
		copy(pack.MsgBytes, record[headerLen:])
		dr.InChan() <- pack
	}
	return
}

// Input plugin implementation that listens for Heka protocol messages on a
// specified UDP socket.
type UdpInput struct {
	listener      net.Conn
	name          string
	stopped       bool
	config        *NetworkInputConfig
	parser        StreamParser
	parseFunction networkParseFunction
}

func (u *UdpInput) ConfigStruct() interface{} {
	return new(NetworkInputConfig)
}

func (u *UdpInput) Init(config interface{}) (err error) {
	u.config = config.(*NetworkInputConfig)
	if len(u.config.Address) > 3 && u.config.Address[:3] == "fd:" {
		// File descriptor
		fdStr := u.config.Address[3:]
		fdInt, err := strconv.ParseUint(fdStr, 0, 0)
		if err != nil {
			log.Println(err)
			return fmt.Errorf("Invalid file descriptor: %s", u.config.Address)
		}
		fd := uintptr(fdInt)
		udpFile := os.NewFile(fd, "udpFile")
		u.listener, err = net.FileConn(udpFile)
		if err != nil {
			return fmt.Errorf("Error accessing UDP fd: %s\n", err.Error())
		}
	} else {
		// IP address
		udpAddr, err := net.ResolveUDPAddr("udp", u.config.Address)
		if err != nil {
			return fmt.Errorf("ResolveUDPAddr failed: %s\n", err.Error())
		}
		u.listener, err = net.ListenUDP("udp", udpAddr)
		if err != nil {
			return fmt.Errorf("ListenUDP failed: %s\n", err.Error())
		}
	}
	if u.config.ParserType == "message.proto" {
		mp := NewMessageProtoParser()
		u.parser = mp
		u.parseFunction = networkMessageProtoParser
		if u.config.Decoder == "" {
			return fmt.Errorf("The message.proto parser must have a decoder")
		}
	} else if u.config.ParserType == "regexp" {
		rp := NewRegexpParser()
		u.parser = rp
		u.parseFunction = networkPayloadParser
		if err = rp.SetDelimiter(u.config.Delimiter); err != nil {
			return err
		}
		if err = rp.SetDelimiterLocation(u.config.DelimiterLocation); err != nil {
			return err
		}
	} else if u.config.ParserType == "token" {
		tp := NewTokenParser()
		u.parser = tp
		u.parseFunction = networkPayloadParser
		switch len(u.config.Delimiter) {
		case 0: // no value was set, the default provided by the StreamParser will be used
		case 1:
			tp.SetDelimiter(u.config.Delimiter[0])
		default:
			return fmt.Errorf("invalid delimiter: %s", u.config.Delimiter)
		}
	} else {
		return fmt.Errorf("unknown parser type: %s", u.config.ParserType)
	}
	u.parser.SetMinimumBufferSize(1024 * 64)
	return
}

func (u *UdpInput) Run(ir InputRunner, h PluginHelper) error {
	var dr DecoderRunner
	var ok bool
	if u.config.Decoder != "" {
		dr, ok = h.DecoderSet().ByName(u.config.Decoder)
		if !ok {
			return fmt.Errorf("Error getting decoder: %s", u.config.Decoder)
		}
	}

	var err error
	for !u.stopped {
		if err = u.parseFunction(u.listener, u.parser, ir, u.config, dr); err != nil {
			if !strings.Contains(err.Error(), "use of closed") {
				ir.LogError(fmt.Errorf("Read error: ", err))
			}
		}
		u.parser.GetRemainingData() // reset the receiving buffer
	}
	return nil
}

func (u *UdpInput) Stop() {
	u.stopped = true
	u.listener.Close()
}

// Heka Message signer object.
type Signer struct {
	HmacKey string `toml:"hmac_key"`
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
		if len(buf)-pos > 1 {
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

// TODO this code duplicates what is in the StreamParser and should be removed
// AMQPInput is the only remaining consumer
//
// Returns true if the provided message is unsigned or has a valid signature
// from one of the provided signers.
func authenticateMessage(signers map[string]Signer, header *Header, msg []byte) bool {
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
		hm.Write(msg)
		expectedDigest := hm.Sum(nil)
		if subtle.ConstantTimeCompare(digest, expectedDigest) != 1 {
			return false
		}
	}
	return true
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
	config   *NetworkInputConfig
}

func (t *TcpInput) ConfigStruct() interface{} {
	return new(NetworkInputConfig)
}

// Listen on the provided TCP connection, extracting messages from the incoming
// data until the connection is closed or Stop is called on the input.
func (t *TcpInput) handleConnection(conn net.Conn) {
	defer func() {
		conn.Close()
		t.wg.Done()
	}()

	var dr DecoderRunner
	var ok bool
	if t.config.Decoder != "" {
		dr, ok = t.h.DecoderSet().ByName(t.config.Decoder)
		if !ok {
			t.ir.LogError(fmt.Errorf("Error getting decoder: %s", t.config.Decoder))
			return
		}
	}

	var parser StreamParser
	var parseFunction networkParseFunction
	if t.config.ParserType == "message.proto" {
		mp := NewMessageProtoParser()
		parser = mp
		parseFunction = networkMessageProtoParser
	} else if t.config.ParserType == "regexp" {
		rp := NewRegexpParser()
		parser = rp
		parseFunction = networkPayloadParser
		rp.SetDelimiter(t.config.Delimiter)
		rp.SetDelimiterLocation(t.config.DelimiterLocation)
	} else if t.config.ParserType == "token" {
		tp := NewTokenParser()
		parser = tp
		parseFunction = networkPayloadParser
		if len(t.config.Delimiter) == 1 {
			tp.SetDelimiter(t.config.Delimiter[0])
		}
	}

	var err error
	stopped := false
	for !stopped {
		conn.SetReadDeadline(time.Now().Add(5 * time.Second))
		select {
		case <-t.stopChan:
			stopped = true
		default:
			if err = parseFunction(conn, parser, t.ir, t.config, dr); err != nil {
				if neterr, ok := err.(net.Error); ok && neterr.Timeout() {
					// keep the connection open, we are just checking to see if
					// we are shutting down: Issue #354
				} else {
					stopped = true
				}
			}
		}
	}
}

func (t *TcpInput) Init(config interface{}) error {
	var err error
	t.config = config.(*NetworkInputConfig)
	t.listener, err = net.Listen("tcp", t.config.Address)
	if err != nil {
		return fmt.Errorf("ListenTCP failed: %s\n", err.Error())
	}
	if t.config.ParserType == "message.proto" {
		if t.config.Decoder == "" {
			return fmt.Errorf("The message.proto parser must have a decoder")
		}
	} else if t.config.ParserType == "regexp" {
		rp := NewRegexpParser() // temporary parser to test the config
		if err = rp.SetDelimiter(t.config.Delimiter); err != nil {
			return err
		}
		if err = rp.SetDelimiterLocation(t.config.DelimiterLocation); err != nil {
			return err
		}
	} else if t.config.ParserType == "token" {
		if len(t.config.Delimiter) > 1 {
			return fmt.Errorf("invalid delimiter: %s", t.config.Delimiter)
		}
	} else {
		return fmt.Errorf("unknown parser type: %s", t.config.ParserType)
	}
	return nil
}

func (t *TcpInput) Run(ir InputRunner, h PluginHelper) error {
	t.ir = ir
	t.h = h
	t.stopChan = make(chan bool)

	var conn net.Conn
	var e error
	for {
		if conn, e = t.listener.Accept(); e != nil {
			if e.(net.Error).Temporary() {
				t.ir.LogError(fmt.Errorf("TCP accept failed: %s", e))
				continue
			} else {
				break
			}
		}
		t.wg.Add(1)
		go t.handleConnection(conn)
	}
	t.wg.Wait()
	return nil
}

func (t *TcpInput) Stop() {
	t.listener.Close()
	close(t.stopChan)
}
