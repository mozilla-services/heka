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
#   Victor Ng (vng@mozilla.com)
#
# ***** END LICENSE BLOCK *****/

package pipeline

import (
	"bytes"
	"code.google.com/p/gomock/gomock"
	"code.google.com/p/goprotobuf/proto"
	"crypto/hmac"
	"crypto/md5"
	"crypto/sha1"
	"errors"
	"fmt"
	"github.com/mozilla-services/heka/message"
	ts "github.com/mozilla-services/heka/testsupport"
	gs "github.com/rafrombrc/gospec/src/gospec"
	"io/ioutil"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

type InputTestHelper struct {
	Msg             *message.Message
	Pack            *PipelinePack
	AddrStr         string
	ResolvedAddrStr string
	MockHelper      *MockPluginHelper
	MockInputRunner *MockInputRunner
	MockDecoderSet  *MockDecoderSet
	Decoder         DecoderRunner
	PackSupply      chan *PipelinePack
	DecodeChan      chan *PipelinePack
}

type address struct {
	network string
	str     string
}

func (a *address) Network() string {
	return a.network
}

func (a *address) String() string {
	return a.str
}

var stopinputTimes int

type StoppingInput struct{}

func (s *StoppingInput) Init(config interface{}) (err error) {
	if stopinputTimes > 1 {
		err = errors.New("Stopped enough, done")
	}
	return
}

func (s *StoppingInput) Run(ir InputRunner, h PluginHelper) (err error) {
	return
}

func (s *StoppingInput) CleanupForRestart() {
	stopinputTimes += 1
}

func (s *StoppingInput) Stop() {
	return
}

func encodeMessage(hbytes, mbytes []byte) (emsg []byte) {
	emsg = make([]byte, 3+len(hbytes)+len(mbytes))
	emsg[0] = message.RECORD_SEPARATOR
	emsg[1] = uint8(len(hbytes))
	copy(emsg[2:], hbytes)
	pos := 2 + len(hbytes)
	emsg[pos] = message.UNIT_SEPARATOR
	copy(emsg[pos+1:], mbytes)
	return
}

func getPayloadBytes(hbytes, mbytes []byte) func(msgBytes []byte) {
	return func(msgBytes []byte) {
		copy(msgBytes, encodeMessage(hbytes, mbytes))
	}
}

func getPayloadText(mbytes []byte) func(msgBytes []byte) {
	return func(msgBytes []byte) {
		copy(msgBytes, mbytes)
	}
}

func InputsSpec(c gs.Context) {
	t := &ts.SimpleT{}
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	config := NewPipelineConfig(nil)
	ith := new(InputTestHelper)
	ith.Msg = getTestMessage()
	ith.Pack = NewPipelinePack(config.inputRecycleChan)

	// Specify localhost, but we're not really going to use the network
	ith.AddrStr = "localhost:55565"
	ith.ResolvedAddrStr = "127.0.0.1:55565"

	// set up mock helper, decoder set, and packSupply channel
	ith.MockHelper = NewMockPluginHelper(ctrl)
	ith.MockInputRunner = NewMockInputRunner(ctrl)
	ith.Decoder = NewMockDecoderRunner(ctrl)
	ith.PackSupply = make(chan *PipelinePack, 1)
	ith.DecodeChan = make(chan *PipelinePack)
	ith.MockDecoderSet = NewMockDecoderSet(ctrl)
	key := "testkey"
	signers := map[string]Signer{"test_1": {key}}
	signer := "test"

	c.Specify("A ProcessInput", func() {
		pInput := ProcessInput{}

		ith.MockInputRunner.EXPECT().InChan().Return(ith.PackSupply).AnyTimes()
		ith.MockInputRunner.EXPECT().Name().Return("logger").AnyTimes()
		ith.MockHelper.EXPECT().DecoderSet().Return(ith.MockDecoderSet).AnyTimes()

		ith.MockHelper.EXPECT().DecoderSet().Return(ith.MockDecoderSet).AnyTimes()
		enccall := ith.MockDecoderSet.EXPECT().ByName("RegexpDecoder").AnyTimes()
		enccall.Return(ith.Decoder, true)

		mockDecoderRunner := ith.Decoder.(*MockDecoderRunner)
		mockDecoderRunner.EXPECT().InChan().Return(ith.DecodeChan).AnyTimes()

		config := pInput.ConfigStruct().(*ProcessInputConfig)
		config.Command = make(map[string]cmd_config)

		pConfig := NewPipelineConfig(nil)
		ith.MockHelper.EXPECT().PipelineConfig().Return(pConfig)

		tickChan := make(chan time.Time)
		ith.MockInputRunner.EXPECT().Ticker().Return(tickChan)

		c.Specify("reads a message from ProcessInput", func() {

			config.Name = "SimpleTest"
			config.Decoder = "RegexpDecoder"
			config.ParserType = "token"
			config.Delimiter = "|"

			// Note that no working directory is explicitly specified
			config.Command["0"] = cmd_config{Bin: "/bin/cat", Args: []string{"../testsupport/process_input_test.txt"}}
			err := pInput.Init(config)
			c.Assume(err, gs.IsNil)

			go func() {
				pInput.Run(ith.MockInputRunner, ith.MockHelper)
			}()
			tickChan <- time.Now()

			expected_payloads := []string{"this|", "is|", "a|", "test|"}
			actual_payloads := []string{}

			for x := 0; x < 4; x++ {
				ith.PackSupply <- ith.Pack
				packRef := <-ith.DecodeChan
				c.Expect(ith.Pack, gs.Equals, packRef)
				actual_payloads = append(actual_payloads, *packRef.Message.Payload)
				fPInputName := *packRef.Message.FindFirstField("ProcessInputName")
				c.Expect(fPInputName.ValueString[0], gs.Equals, "SimpleTest.stdout")
				// Free up the scheduler
				runtime.Gosched()
			}

			for x := 0; x < 4; x++ {
				c.Expect(expected_payloads[x], gs.Equals, actual_payloads[x])
			}

			pInput.Stop()
		})

		c.Specify("handles bad arguments", func() {

			config.Name = "BadArgs"
			config.ParseStdout = false
			config.ParseStderr = true
			config.Decoder = "RegexpDecoder"
			config.ParserType = "token"
			config.Delimiter = "|"

			// Note that no working directory is explicitly specified
			config.Command["0"] = cmd_config{Bin: "/bin/cat", Args: []string{"../testsupport/not_a_file.txt"}}

			err := pInput.Init(config)
			c.Assume(err, gs.IsNil)

			expected_err := fmt.Errorf("BadArgs CommandChain::Wait() error: [exit status 1]")
			ith.MockInputRunner.EXPECT().LogError(expected_err)

			go func() {
				pInput.Run(ith.MockInputRunner, ith.MockHelper)
			}()
			tickChan <- time.Now()

			ith.PackSupply <- ith.Pack
			<-ith.DecodeChan
			runtime.Gosched()

			pInput.Stop()
		})

		c.Specify("can pipe multiple commands together", func() {

			config.Name = "PipedCmd"
			config.Decoder = "RegexpDecoder"
			config.ParserType = "token"
			// Overload the delimiter
			config.Delimiter = " "

			// Note that no working directory is explicitly specified
			config.Command["0"] = cmd_config{Bin: "cat", Args: []string{"../testsupport/process_input_pipes_test.txt"}}
			config.Command["1"] = cmd_config{Bin: "grep", Args: []string{"ignore"}}
			err := pInput.Init(config)
			c.Assume(err, gs.IsNil)

			go func() {
				pInput.Run(ith.MockInputRunner, ith.MockHelper)
			}()
			tickChan <- time.Now()

			expected_payloads := []string{"ignore ", "this ", "line"}
			actual_payloads := []string{}

			for x := 0; x < 3; x++ {
				ith.PackSupply <- ith.Pack
				packRef := <-ith.DecodeChan
				c.Expect(ith.Pack, gs.Equals, packRef)
				actual_payloads = append(actual_payloads, *packRef.Message.Payload)
				fPInputName := *packRef.Message.FindFirstField("ProcessInputName")
				c.Expect(fPInputName.ValueString[0], gs.Equals, "PipedCmd.stdout")
				// Free up the scheduler
				runtime.Gosched()
			}

			for x := 0; x < 3; x++ {
				c.Expect(actual_payloads[x], gs.Equals, expected_payloads[x])
			}

			pInput.Stop()
		})

	})

	c.Specify("A UdpInput", func() {
		udpInput := UdpInput{}
		err := udpInput.Init(&NetworkInputConfig{Address: ith.AddrStr,
			Decoder:    "ProtobufDecoder",
			ParserType: "message.proto"})
		c.Assume(err, gs.IsNil)
		realListener := (udpInput.listener).(*net.UDPConn)
		c.Expect(realListener.LocalAddr().String(), gs.Equals, ith.ResolvedAddrStr)

		mbytes, _ := proto.Marshal(ith.Msg)
		header := &message.Header{}
		header.SetMessageLength(uint32(len(mbytes)))

		mockDecoderRunner := ith.Decoder.(*MockDecoderRunner)
		mockDecoderRunner.EXPECT().InChan().Return(ith.DecodeChan)
		ith.MockInputRunner.EXPECT().InChan().Return(ith.PackSupply)
		ith.MockHelper.EXPECT().DecoderSet().Return(ith.MockDecoderSet)
		encCall := ith.MockDecoderSet.EXPECT().ByName("ProtobufDecoder")
		encCall.Return(ith.Decoder, true)

		c.Specify("reads a message from the connection and passes it to the decoder", func() {
			hbytes, _ := proto.Marshal(header)
			go func() {
				udpInput.Run(ith.MockInputRunner, ith.MockHelper)
			}()
			conn, err := net.Dial("udp", ith.AddrStr) // a mock connection will not work here since the mock read cannot block
			c.Assume(err, gs.IsNil)
			buf := encodeMessage(hbytes, mbytes)
			_, err = conn.Write(buf)
			c.Assume(err, gs.IsNil)
			ith.PackSupply <- ith.Pack
			packRef := <-ith.DecodeChan
			udpInput.Stop()
			c.Expect(ith.Pack, gs.Equals, packRef)
			c.Expect(string(ith.Pack.MsgBytes), gs.Equals, string(mbytes))
			c.Expect(ith.Pack.Decoded, gs.IsFalse)
		})
	})

	c.Specify("A TcpInput protobuf parser", func() {
		tcpInput := TcpInput{}
		err := tcpInput.Init(&NetworkInputConfig{Address: ith.AddrStr,
			Signers:    signers,
			Decoder:    "ProtobufDecoder",
			ParserType: "message.proto"})
		c.Assume(err, gs.IsNil)
		realListener := tcpInput.listener
		c.Expect(realListener.Addr().String(), gs.Equals, ith.ResolvedAddrStr)
		realListener.Close()

		mockConnection := ts.NewMockConn(ctrl)
		mockListener := ts.NewMockListener(ctrl)
		tcpInput.listener = mockListener

		mbytes, _ := proto.Marshal(ith.Msg)
		header := &message.Header{}
		header.SetMessageLength(uint32(len(mbytes)))
		err = errors.New("connection closed") // used in the read return(s)
		readCall := mockConnection.EXPECT().Read(gomock.Any())
		readEnd := mockConnection.EXPECT().Read(gomock.Any()).After(readCall)
		readEnd.Return(0, err)
		mockConnection.EXPECT().SetReadDeadline(gomock.Any()).Return(nil).AnyTimes()
		mockConnection.EXPECT().Close()

		neterr := ts.NewMockError(ctrl)
		neterr.EXPECT().Temporary().Return(false)
		acceptCall := mockListener.EXPECT().Accept().Return(mockConnection, nil)
		acceptCall.Do(func() {
			acceptCall = mockListener.EXPECT().Accept()
			acceptCall.Return(nil, neterr)
		})

		mockDecoderRunner := ith.Decoder.(*MockDecoderRunner)
		mockDecoderRunner.EXPECT().InChan().Return(ith.DecodeChan)
		ith.MockInputRunner.EXPECT().InChan().Return(ith.PackSupply)
		ith.MockHelper.EXPECT().DecoderSet().Return(ith.MockDecoderSet)
		enccall := ith.MockDecoderSet.EXPECT().ByName("ProtobufDecoder").AnyTimes()
		enccall.Return(ith.Decoder, true)

		c.Specify("reads a message from its connection", func() {
			hbytes, _ := proto.Marshal(header)
			buflen := 3 + len(hbytes) + len(mbytes)
			readCall.Return(buflen, nil)
			readCall.Do(getPayloadBytes(hbytes, mbytes))
			go func() {
				tcpInput.Run(ith.MockInputRunner, ith.MockHelper)
			}()
			ith.PackSupply <- ith.Pack
			packRef := <-ith.DecodeChan
			c.Expect(ith.Pack, gs.Equals, packRef)
			c.Expect(string(ith.Pack.MsgBytes), gs.Equals, string(mbytes))
		})

		c.Specify("reads a MD5 signed message from its connection", func() {
			header.SetHmacHashFunction(message.Header_MD5)
			header.SetHmacSigner(signer)
			header.SetHmacKeyVersion(uint32(1))
			hm := hmac.New(md5.New, []byte(key))
			hm.Write(mbytes)
			header.SetHmac(hm.Sum(nil))
			hbytes, _ := proto.Marshal(header)
			buflen := 3 + len(hbytes) + len(mbytes)
			readCall.Return(buflen, nil)
			readCall.Do(getPayloadBytes(hbytes, mbytes))

			go func() {
				tcpInput.Run(ith.MockInputRunner, ith.MockHelper)
			}()
			ith.PackSupply <- ith.Pack
			timeout := make(chan bool)
			go func() {
				time.Sleep(100 * time.Millisecond)
				timeout <- true
			}()
			select {
			case packRef := <-ith.DecodeChan:
				c.Expect(ith.Pack, gs.Equals, packRef)
				c.Expect(string(ith.Pack.MsgBytes), gs.Equals, string(mbytes))
				c.Expect(ith.Pack.Signer, gs.Equals, "test")
			case t := <-timeout:
				c.Expect(t, gs.IsNil)
			}
		})

		c.Specify("reads a SHA1 signed message from its connection", func() {
			header.SetHmacHashFunction(message.Header_SHA1)
			header.SetHmacSigner(signer)
			header.SetHmacKeyVersion(uint32(1))
			hm := hmac.New(sha1.New, []byte(key))
			hm.Write(mbytes)
			header.SetHmac(hm.Sum(nil))
			hbytes, _ := proto.Marshal(header)
			buflen := 3 + len(hbytes) + len(mbytes)
			readCall.Return(buflen, nil)
			readCall.Do(getPayloadBytes(hbytes, mbytes))

			go func() {
				tcpInput.Run(ith.MockInputRunner, ith.MockHelper)
			}()
			ith.PackSupply <- ith.Pack
			timeout := make(chan bool)
			go func() {
				time.Sleep(100 * time.Millisecond)
				timeout <- true
			}()
			select {
			case packRef := <-ith.DecodeChan:
				c.Expect(ith.Pack, gs.Equals, packRef)
				c.Expect(string(ith.Pack.MsgBytes), gs.Equals, string(mbytes))
				c.Expect(ith.Pack.Signer, gs.Equals, "test")
			case t := <-timeout:
				c.Expect(t, gs.IsNil)
			}
		})

		c.Specify("reads a signed message with an expired key from its connection", func() {
			header.SetHmacHashFunction(message.Header_MD5)
			header.SetHmacSigner(signer)
			header.SetHmacKeyVersion(uint32(11)) // non-existent key version
			hm := hmac.New(md5.New, []byte(key))
			hm.Write(mbytes)
			header.SetHmac(hm.Sum(nil))
			hbytes, _ := proto.Marshal(header)
			buflen := 3 + len(hbytes) + len(mbytes)
			readCall.Return(buflen, nil)
			readCall.Do(getPayloadBytes(hbytes, mbytes))

			go func() {
				tcpInput.Run(ith.MockInputRunner, ith.MockHelper)
			}()
			ith.PackSupply <- ith.Pack
			timeout := make(chan bool)
			go func() {
				time.Sleep(100 * time.Millisecond)
				timeout <- true
			}()
			select {
			case packRef := <-mockDecoderRunner.InChan():
				c.Expect(packRef, gs.IsNil)
			case t := <-timeout:
				c.Expect(t, gs.IsTrue)
			}
		})

		c.Specify("reads a signed message with an incorrect hmac from its connection", func() {
			header.SetHmacHashFunction(message.Header_MD5)
			header.SetHmacSigner(signer)
			header.SetHmacKeyVersion(uint32(1))
			hm := hmac.New(md5.New, []byte(key))
			hm.Write([]byte("some bytes"))
			header.SetHmac(hm.Sum(nil))
			hbytes, _ := proto.Marshal(header)
			buflen := 3 + len(hbytes) + len(mbytes)
			readCall.Return(buflen, nil)
			readCall.Do(getPayloadBytes(hbytes, mbytes))

			go func() {
				tcpInput.Run(ith.MockInputRunner, ith.MockHelper)
			}()
			ith.PackSupply <- ith.Pack
			timeout := make(chan bool)
			go func() {
				time.Sleep(100 * time.Millisecond)
				timeout <- true
			}()
			select {
			case packRef := <-mockDecoderRunner.InChan():
				c.Expect(packRef, gs.IsNil)
			case t := <-timeout:
				c.Expect(t, gs.IsTrue)
			}
		})
	})

	c.Specify("A TcpInput regexp parser", func() {
		tcpInput := TcpInput{}
		err := tcpInput.Init(&NetworkInputConfig{Address: ith.AddrStr,
			Decoder:    "RegexpDecoder",
			ParserType: "regexp",
			Delimiter:  "\n"})
		c.Assume(err, gs.IsNil)
		realListener := tcpInput.listener
		c.Expect(realListener.Addr().String(), gs.Equals, ith.ResolvedAddrStr)
		realListener.Close()

		mockConnection := ts.NewMockConn(ctrl)
		mockListener := ts.NewMockListener(ctrl)
		tcpInput.listener = mockListener

		addr := new(address)
		addr.str = "123"
		mockConnection.EXPECT().RemoteAddr().Return(addr)
		mbytes := []byte("this is a test message\n")
		err = errors.New("connection closed") // used in the read return(s)
		readCall := mockConnection.EXPECT().Read(gomock.Any())
		readEnd := mockConnection.EXPECT().Read(gomock.Any()).After(readCall)
		readEnd.Return(0, err)
		mockConnection.EXPECT().SetReadDeadline(gomock.Any()).Return(nil).AnyTimes()
		mockConnection.EXPECT().Close()

		neterr := ts.NewMockError(ctrl)
		neterr.EXPECT().Temporary().Return(false)
		acceptCall := mockListener.EXPECT().Accept().Return(mockConnection, nil)
		acceptCall.Do(func() {
			acceptCall = mockListener.EXPECT().Accept()
			acceptCall.Return(nil, neterr)
		})

		mockDecoderRunner := ith.Decoder.(*MockDecoderRunner)
		mockDecoderRunner.EXPECT().InChan().Return(ith.DecodeChan)
		ith.MockInputRunner.EXPECT().InChan().Return(ith.PackSupply)
		ith.MockInputRunner.EXPECT().Name().Return("logger")
		ith.MockHelper.EXPECT().DecoderSet().Return(ith.MockDecoderSet)
		enccall := ith.MockDecoderSet.EXPECT().ByName("RegexpDecoder").AnyTimes()
		enccall.Return(ith.Decoder, true)

		c.Specify("reads a message from its connection", func() {
			readCall.Return(len(mbytes), nil)
			readCall.Do(getPayloadText(mbytes))
			go func() {
				tcpInput.Run(ith.MockInputRunner, ith.MockHelper)
			}()
			ith.PackSupply <- ith.Pack
			packRef := <-ith.DecodeChan
			c.Expect(ith.Pack, gs.Equals, packRef)
			c.Expect(ith.Pack.Message.GetPayload(), gs.Equals, string(mbytes[:len(mbytes)-1]))
			c.Expect(ith.Pack.Message.GetLogger(), gs.Equals, "logger")
			c.Expect(ith.Pack.Message.GetHostname(), gs.Equals, "123")
		})
	})

	c.Specify("A TcpInput token parser", func() {
		tcpInput := TcpInput{}
		err := tcpInput.Init(&NetworkInputConfig{Address: ith.AddrStr,
			Decoder:    "TokenDecoder",
			ParserType: "token",
			Delimiter:  "\n"})
		c.Assume(err, gs.IsNil)
		realListener := tcpInput.listener
		c.Expect(realListener.Addr().String(), gs.Equals, ith.ResolvedAddrStr)
		realListener.Close()

		mockConnection := ts.NewMockConn(ctrl)
		mockListener := ts.NewMockListener(ctrl)
		tcpInput.listener = mockListener

		addr := new(address)
		addr.str = "123"
		mockConnection.EXPECT().RemoteAddr().Return(addr)
		mbytes := []byte("this is a test message\n")
		err = errors.New("connection closed") // used in the read return(s)
		readCall := mockConnection.EXPECT().Read(gomock.Any())
		readEnd := mockConnection.EXPECT().Read(gomock.Any()).After(readCall)
		readEnd.Return(0, err)
		mockConnection.EXPECT().SetReadDeadline(gomock.Any()).Return(nil).AnyTimes()
		mockConnection.EXPECT().Close()

		neterr := ts.NewMockError(ctrl)
		neterr.EXPECT().Temporary().Return(false)
		acceptCall := mockListener.EXPECT().Accept().Return(mockConnection, nil)
		acceptCall.Do(func() {
			acceptCall = mockListener.EXPECT().Accept()
			acceptCall.Return(nil, neterr)
		})

		mockDecoderRunner := ith.Decoder.(*MockDecoderRunner)
		mockDecoderRunner.EXPECT().InChan().Return(ith.DecodeChan)
		ith.MockInputRunner.EXPECT().InChan().Return(ith.PackSupply)
		ith.MockInputRunner.EXPECT().Name().Return("logger")
		ith.MockHelper.EXPECT().DecoderSet().Return(ith.MockDecoderSet)
		enccall := ith.MockDecoderSet.EXPECT().ByName("TokenDecoder").AnyTimes()
		enccall.Return(ith.Decoder, true)

		c.Specify("reads a message from its connection", func() {
			readCall.Return(len(mbytes), nil)
			readCall.Do(getPayloadText(mbytes))
			go func() {
				tcpInput.Run(ith.MockInputRunner, ith.MockHelper)
			}()
			ith.PackSupply <- ith.Pack
			packRef := <-ith.DecodeChan
			c.Expect(ith.Pack, gs.Equals, packRef)
			c.Expect(ith.Pack.Message.GetPayload(), gs.Equals, string(mbytes))
			c.Expect(ith.Pack.Message.GetLogger(), gs.Equals, "logger")
			c.Expect(ith.Pack.Message.GetHostname(), gs.Equals, "123")
		})
	})

	c.Specify("Runner restarts a plugin on the first time only", func() {
		var pluginGlobals PluginGlobals
		pluginGlobals.Retries = RetryOptions{
			MaxDelay:   "1us",
			Delay:      "1us",
			MaxJitter:  "1us",
			MaxRetries: 1,
		}
		pc := new(PipelineConfig)
		pc.inputWrappers = make(map[string]*PluginWrapper)

		pw := &PluginWrapper{
			name:          "stopping",
			configCreator: func() interface{} { return nil },
			pluginCreator: func() interface{} { return new(StoppingInput) },
		}
		pc.inputWrappers["stopping"] = pw

		input := new(StoppingInput)
		iRunner := NewInputRunner("stopping", input, &pluginGlobals)
		var wg sync.WaitGroup
		cfgCall := ith.MockHelper.EXPECT().PipelineConfig().Times(7)
		cfgCall.Return(pc)
		wg.Add(1)
		iRunner.Start(ith.MockHelper, &wg)
		wg.Wait()
		c.Expect(stopinputTimes, gs.Equals, 2)
	})

	c.Specify("A LogFileInput", func() {
		tmpDir, tmpErr := ioutil.TempDir("", "hekad-tests-")
		c.Expect(tmpErr, gs.Equals, nil)
		origBaseDir := Globals().BaseDir
		Globals().BaseDir = tmpDir
		defer func() {
			Globals().BaseDir = origBaseDir
			tmpErr = os.RemoveAll(tmpDir)
			c.Expect(tmpErr, gs.IsNil)
		}()
		lfInput := new(LogfileInput)
		lfiConfig := lfInput.ConfigStruct().(*LogfileInputConfig)
		lfiConfig.SeekJournalName = "test-seekjournal"
		lfiConfig.LogFile = "../testsupport/test-zeus.log"
		lfiConfig.Logger = "zeus"
		lfiConfig.UseSeekJournal = true
		lfiConfig.Decoder = "decoder-name"
		lfiConfig.DiscoverInterval = 1
		lfiConfig.StatInterval = 1
		err := lfInput.Init(lfiConfig)
		c.Expect(err, gs.IsNil)
		mockDecoderRunner := NewMockDecoderRunner(ctrl)

		// Create pool of packs.
		numLines := 95 // # of lines in the log file we're parsing.
		packs := make([]*PipelinePack, numLines)
		ith.PackSupply = make(chan *PipelinePack, numLines)
		for i := 0; i < numLines; i++ {
			packs[i] = NewPipelinePack(ith.PackSupply)
			ith.PackSupply <- packs[i]
		}

		c.Specify("reads a log file", func() {
			// Expect InputRunner calls to get InChan and inject outgoing msgs
			ith.MockInputRunner.EXPECT().LogError(gomock.Any()).AnyTimes()
			ith.MockInputRunner.EXPECT().LogMessage(gomock.Any()).AnyTimes()
			ith.MockInputRunner.EXPECT().InChan().Return(ith.PackSupply).Times(numLines)
			// Expect calls to get decoder and decode each message. Since the
			// decoding is a no-op, the message payload will be the log file
			// line, unchanged.
			ith.MockHelper.EXPECT().DecoderSet().Return(ith.MockDecoderSet)
			pbcall := ith.MockDecoderSet.EXPECT().ByName(lfiConfig.Decoder)
			pbcall.Return(mockDecoderRunner, true)
			decodeCall := mockDecoderRunner.EXPECT().InChan().Times(numLines)
			decodeCall.Return(ith.DecodeChan)
			go func() {
				err = lfInput.Run(ith.MockInputRunner, ith.MockHelper)
				c.Expect(err, gs.IsNil)
			}()
			for x := 0; x < numLines; x++ {
				_ = <-ith.DecodeChan
				// Free up the scheduler while we wait for the log file lines
				// to be processed.
				runtime.Gosched()
			}
			close(lfInput.Monitor.stopChan)

			fileBytes, err := ioutil.ReadFile(lfiConfig.LogFile)
			c.Expect(err, gs.IsNil)
			fileStr := string(fileBytes)
			lines := strings.Split(fileStr, "\n")
			for i, line := range lines {
				if line == "" {
					continue
				}
				c.Expect(packs[i].Message.GetPayload(), gs.Equals, line+"\n")
				c.Expect(packs[i].Message.GetLogger(), gs.Equals, "zeus")
			}

			// Wait for the file update to hit the disk; better suggestions are welcome
			runtime.Gosched()
			time.Sleep(time.Millisecond * 250)
			journalData := []byte(`{"last_hash":"f0b60af7f2cb35c3724151422e2f999af6e21fc0","last_len":300,"last_start":28650,"seek":28950}`)
			journalFile, err := ioutil.ReadFile(filepath.Join(tmpDir, "seekjournals", lfiConfig.SeekJournalName))
			c.Expect(err, gs.IsNil)
			c.Expect(bytes.Compare(journalData, journalFile), gs.Equals, 0)
		})

		c.Specify("uses the filename as the default logger name", func() {
			lfInput := new(LogfileInput)
			lfiConfig := lfInput.ConfigStruct().(*LogfileInputConfig)
			lfiConfig.LogFile = "../testsupport/test-zeus.log"

			lfiConfig.DiscoverInterval = 1
			lfiConfig.StatInterval = 1
			err := lfInput.Init(lfiConfig)
			c.Expect(err, gs.Equals, nil)

			c.Expect(lfInput.Monitor.logger_ident,
				gs.Equals,
				lfiConfig.LogFile)
		})
	})

	c.Specify("A Regex LogFileInput", func() {
		tmpDir, tmpErr := ioutil.TempDir("", "hekad-tests-")
		c.Expect(tmpErr, gs.Equals, nil)
		origBaseDir := Globals().BaseDir
		Globals().BaseDir = tmpDir
		defer func() {
			Globals().BaseDir = origBaseDir
			tmpErr = os.RemoveAll(tmpDir)
			c.Expect(tmpErr, gs.Equals, nil)
		}()
		lfInput := new(LogfileInput)
		lfiConfig := lfInput.ConfigStruct().(*LogfileInputConfig)
		lfiConfig.SeekJournalName = "regex-seekjournal"
		lfiConfig.LogFile = "../testsupport/test-zeus.log"
		lfiConfig.Logger = "zeus"
		lfiConfig.ParserType = "regexp"
		lfiConfig.Delimiter = "(\n)"
		lfiConfig.UseSeekJournal = true
		lfiConfig.Decoder = "decoder-name"
		lfiConfig.DiscoverInterval = 1
		lfiConfig.StatInterval = 1
		err := lfInput.Init(lfiConfig)
		c.Expect(err, gs.IsNil)

		mockDecoderRunner := NewMockDecoderRunner(ctrl)

		// Create pool of packs.
		numLines := 95 // # of lines in the log file we're parsing.
		packs := make([]*PipelinePack, numLines)
		ith.PackSupply = make(chan *PipelinePack, numLines)
		for i := 0; i < numLines; i++ {
			packs[i] = NewPipelinePack(ith.PackSupply)
			ith.PackSupply <- packs[i]
		}

		c.Specify("reads a log file", func() {
			// Expect InputRunner calls to get InChan and inject outgoing msgs
			ith.MockInputRunner.EXPECT().LogError(gomock.Any()).AnyTimes()
			ith.MockInputRunner.EXPECT().LogMessage(gomock.Any()).AnyTimes()
			ith.MockInputRunner.EXPECT().InChan().Return(ith.PackSupply).Times(numLines)
			// Expect calls to get decoder and decode each message. Since the
			// decoding is a no-op, the message payload will be the log file
			// line, unchanged.
			ith.MockHelper.EXPECT().DecoderSet().Return(ith.MockDecoderSet)
			pbcall := ith.MockDecoderSet.EXPECT().ByName(lfiConfig.Decoder)
			pbcall.Return(mockDecoderRunner, true)
			decodeCall := mockDecoderRunner.EXPECT().InChan().Times(numLines)
			decodeCall.Return(ith.DecodeChan)
			go func() {
				err = lfInput.Run(ith.MockInputRunner, ith.MockHelper)
				c.Expect(err, gs.IsNil)
			}()
			for x := 0; x < numLines; x++ {
				_ = <-ith.DecodeChan
				// Free up the scheduler while we wait for the log file lines
				// to be processed.
				runtime.Gosched()
			}
			close(lfInput.Monitor.stopChan)

			fileBytes, err := ioutil.ReadFile(lfiConfig.LogFile)
			c.Expect(err, gs.IsNil)
			fileStr := string(fileBytes)
			lines := strings.Split(fileStr, "\n")
			for i, line := range lines {
				if line == "" {
					continue
				}
				c.Expect(packs[i].Message.GetPayload(), gs.Equals, line+"\n")
				c.Expect(packs[i].Message.GetLogger(), gs.Equals, "zeus")
			}

			// Wait for the file update to hit the disk; better suggestions are welcome
			runtime.Gosched()
			time.Sleep(time.Millisecond * 250)
			journalData := []byte(`{"last_hash":"f0b60af7f2cb35c3724151422e2f999af6e21fc0","last_len":300,"last_start":28650,"seek":28950}`)
			journalFile, err := ioutil.ReadFile(filepath.Join(tmpDir, "seekjournals", lfiConfig.SeekJournalName))
			c.Expect(err, gs.IsNil)
			c.Expect(bytes.Compare(journalData, journalFile), gs.Equals, 0)
		})
	})

	c.Specify("A Regex Multiline LogFileInput", func() {
		tmpDir, tmpErr := ioutil.TempDir("", "hekad-tests-")
		c.Expect(tmpErr, gs.Equals, nil)
		origBaseDir := Globals().BaseDir
		Globals().BaseDir = tmpDir
		defer func() {
			Globals().BaseDir = origBaseDir
			tmpErr = os.RemoveAll(tmpDir)
			c.Expect(tmpErr, gs.Equals, nil)
		}()
		lfInput := new(LogfileInput)
		lfiConfig := lfInput.ConfigStruct().(*LogfileInputConfig)
		lfiConfig.SeekJournalName = "multiline-seekjournal"
		lfiConfig.LogFile = "../testsupport/multiline.log"
		lfiConfig.Logger = "multiline"
		lfiConfig.ParserType = "regexp"
		lfiConfig.Delimiter = "\n(\\d{4}-\\d{2}-\\d{2})"
		lfiConfig.DelimiterLocation = "start"
		lfiConfig.UseSeekJournal = true
		lfiConfig.Decoder = "decoder-name"
		lfiConfig.DiscoverInterval = 1
		lfiConfig.StatInterval = 1
		err := lfInput.Init(lfiConfig)
		c.Expect(err, gs.IsNil)

		mockDecoderRunner := NewMockDecoderRunner(ctrl)

		// Create pool of packs.
		numLines := 4 // # of lines in the log file we're parsing.
		packs := make([]*PipelinePack, numLines)
		ith.PackSupply = make(chan *PipelinePack, numLines)
		for i := 0; i < numLines; i++ {
			packs[i] = NewPipelinePack(ith.PackSupply)
			ith.PackSupply <- packs[i]
		}

		c.Specify("reads a log file", func() {
			// Expect InputRunner calls to get InChan and inject outgoing msgs
			ith.MockInputRunner.EXPECT().LogError(gomock.Any()).AnyTimes()
			ith.MockInputRunner.EXPECT().LogMessage(gomock.Any()).AnyTimes()
			ith.MockInputRunner.EXPECT().InChan().Return(ith.PackSupply).Times(numLines)
			// Expect calls to get decoder and decode each message. Since the
			// decoding is a no-op, the message payload will be the log file
			// line, unchanged.
			ith.MockHelper.EXPECT().DecoderSet().Return(ith.MockDecoderSet)
			pbcall := ith.MockDecoderSet.EXPECT().ByName(lfiConfig.Decoder)
			pbcall.Return(mockDecoderRunner, true)
			decodeCall := mockDecoderRunner.EXPECT().InChan().Times(numLines)
			decodeCall.Return(ith.DecodeChan)
			go func() {
				err = lfInput.Run(ith.MockInputRunner, ith.MockHelper)
				c.Expect(err, gs.IsNil)
			}()
			for x := 0; x < numLines; x++ {
				_ = <-ith.DecodeChan
				// Free up the scheduler while we wait for the log file lines
				// to be processed.
				runtime.Gosched()
			}
			close(lfInput.Monitor.stopChan)

			lines := []string{
				"2012-07-13 18:48:01 debug    readSocket()",
				"2012-07-13 18:48:21 info     Processing queue id 3496 -> subm id 2817 from site ms",
				"2012-07-13 18:48:25 debug    line0\nline1\nline2",
				"2012-07-13 18:48:26 debug    readSocket()",
			}
			for i, line := range lines {
				c.Expect(packs[i].Message.GetPayload(), gs.Equals, line)
				c.Expect(packs[i].Message.GetLogger(), gs.Equals, "multiline")
			}

			// Wait for the file update to hit the disk; better suggestions are welcome
			runtime.Gosched()
			time.Sleep(time.Millisecond * 250)
			journalData := []byte(`{"last_hash":"39e4c3e6e9c88a794b3e7c91c155682c34cf1a4a","last_len":41,"last_start":172,"seek":214}`)
			journalFile, err := ioutil.ReadFile(filepath.Join(tmpDir, "seekjournals", lfiConfig.SeekJournalName))
			c.Expect(err, gs.IsNil)
			c.Expect(bytes.Compare(journalData, journalFile), gs.Equals, 0)
		})
	})

	c.Specify("A message.proto LogFileInput", func() {
		tmpDir, tmpErr := ioutil.TempDir("", "hekad-tests-")
		c.Expect(tmpErr, gs.Equals, nil)
		origBaseDir := Globals().BaseDir
		Globals().BaseDir = tmpDir
		defer func() {
			Globals().BaseDir = origBaseDir
			tmpErr = os.RemoveAll(tmpDir)
			c.Expect(tmpErr, gs.Equals, nil)
		}()
		lfInput := new(LogfileInput)
		lfiConfig := lfInput.ConfigStruct().(*LogfileInputConfig)
		lfiConfig.SeekJournalName = "protobuf-seekjournal"
		lfiConfig.LogFile = "../testsupport/protobuf.log"
		lfiConfig.ParserType = "message.proto"
		lfiConfig.UseSeekJournal = true
		lfiConfig.Decoder = "decoder-name"
		lfiConfig.DiscoverInterval = 1
		lfiConfig.StatInterval = 1
		err := lfInput.Init(lfiConfig)
		c.Expect(err, gs.IsNil)

		mockDecoderRunner := NewMockDecoderRunner(ctrl)

		// Create pool of packs.
		numLines := 7 // # of lines in the log file we're parsing.
		packs := make([]*PipelinePack, numLines)
		ith.PackSupply = make(chan *PipelinePack, numLines)
		for i := 0; i < numLines; i++ {
			packs[i] = NewPipelinePack(ith.PackSupply)
			ith.PackSupply <- packs[i]
		}

		c.Specify("reads a log file", func() {
			// Expect InputRunner calls to get InChan and decode outgoing msgs
			ith.MockInputRunner.EXPECT().LogError(gomock.Any()).AnyTimes()
			ith.MockInputRunner.EXPECT().LogMessage(gomock.Any()).AnyTimes()
			ith.MockInputRunner.EXPECT().InChan().Return(ith.PackSupply).Times(numLines)
			ith.MockHelper.EXPECT().DecoderSet().Return(ith.MockDecoderSet)
			pbcall := ith.MockDecoderSet.EXPECT().ByName(lfiConfig.Decoder)
			pbcall.Return(mockDecoderRunner, true)
			decodeCall := mockDecoderRunner.EXPECT().InChan().Times(numLines)
			decodeCall.Return(ith.DecodeChan)
			go func() {
				err = lfInput.Run(ith.MockInputRunner, ith.MockHelper)
				c.Expect(err, gs.IsNil)
			}()
			for x := 0; x < numLines; x++ {
				_ = <-ith.DecodeChan
				// Free up the scheduler while we wait for the log file lines
				// to be processed.
				runtime.Gosched()
			}
			close(lfInput.Monitor.stopChan)

			lines := []int{36230, 41368, 42310, 41343, 37171, 56727, 46492}
			for i, line := range lines {
				c.Expect(len(packs[i].MsgBytes), gs.Equals, line)
				err = proto.Unmarshal(packs[i].MsgBytes, packs[i].Message)
				c.Expect(err, gs.IsNil)
				c.Expect(packs[i].Message.GetType(), gs.Equals, "hekabench")
			}

			// Wait for the file update to hit the disk; better suggestions are welcome
			runtime.Gosched()
			time.Sleep(time.Millisecond * 250)
			journalData := []byte(`{"last_hash":"f67dc6bbbbb6a91b59e661b6170de50c96eab100","last_len":46499,"last_start":255191,"seek":301690}`)
			journalFile, err := ioutil.ReadFile(filepath.Join(tmpDir, "seekjournals", lfiConfig.SeekJournalName))
			c.Expect(err, gs.IsNil)
			c.Expect(bytes.Compare(journalData, journalFile), gs.Equals, 0)
		})
	})

	c.Specify("A message.proto LogFileInput no decoder", func() {
		tmpDir, tmpErr := ioutil.TempDir("", "hekad-tests-")
		c.Expect(tmpErr, gs.Equals, nil)
		origBaseDir := Globals().BaseDir
		Globals().BaseDir = tmpDir
		defer func() {
			Globals().BaseDir = origBaseDir
			tmpErr = os.RemoveAll(tmpDir)
			c.Expect(tmpErr, gs.Equals, nil)
		}()
		lfInput := new(LogfileInput)
		lfiConfig := lfInput.ConfigStruct().(*LogfileInputConfig)
		lfiConfig.SeekJournalName = "protobuf-seekjournal"
		lfiConfig.LogFile = "../testsupport/protobuf.log"
		lfiConfig.ParserType = "message.proto"
		lfiConfig.UseSeekJournal = true
		lfiConfig.Decoder = ""
		lfiConfig.DiscoverInterval = 1
		lfiConfig.StatInterval = 1
		err := lfInput.Init(lfiConfig)
		c.Expect(err, gs.Not(gs.IsNil))
	})
}
