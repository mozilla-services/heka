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
	"code.google.com/p/gomock/gomock"
	"code.google.com/p/goprotobuf/proto"
	"crypto/hmac"
	"crypto/md5"
	"crypto/sha1"
	"encoding/json"
	"errors"
	"github.com/mozilla-services/heka/message"
	ts "github.com/mozilla-services/heka/testsupport"
	gs "github.com/rafrombrc/gospec/src/gospec"
	"io/ioutil"
	"net"
	"os"
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
	Decoders        []DecoderRunner
	PackSupply      chan *PipelinePack
	DecodeChan      chan *PipelinePack
}

type PanicInput struct{}

func (p *PanicInput) Init(config interface{}) (err error) {
	return
}

func (p *PanicInput) Run(ir InputRunner, h PluginHelper) (err error) {
	panic("PANICINPUT")
}

func (p *PanicInput) Stop() {
	panic("PANICINPUT")
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

func getPayloadBytes(hbytes, mbytes []byte) func(msgBytes []byte) {
	return func(msgBytes []byte) {
		msgBytes[0] = message.RECORD_SEPARATOR
		msgBytes[1] = uint8(len(hbytes))
		copy(msgBytes[2:], hbytes)
		pos := 2 + len(hbytes)
		msgBytes[pos] = message.UNIT_SEPARATOR
		copy(msgBytes[pos+1:], mbytes)
	}
}

func createJournal() (journal string, err error) {
	var tmp_file *os.File
	tmp_file, err = ioutil.TempFile("", "")
	journal = tmp_file.Name()
	tmp_file.Close()
	return journal, nil
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
	ith.Decoders = make([]DecoderRunner, int(message.Header_JSON+1))
	ith.Decoders[message.Header_PROTOCOL_BUFFER] = NewMockDecoderRunner(ctrl)
	ith.Decoders[message.Header_JSON] = NewMockDecoderRunner(ctrl)
	ith.PackSupply = make(chan *PipelinePack, 1)
	ith.DecodeChan = make(chan *PipelinePack)
	ith.MockDecoderSet = NewMockDecoderSet(ctrl)
	key := "testkey"
	signers := map[string]Signer{"test_1": {key}}
	signer := "test"

	c.Specify("A UdpInput", func() {
		udpInput := UdpInput{}
		err := udpInput.Init(&UdpInputConfig{ith.AddrStr, signers})
		c.Assume(err, gs.IsNil)
		realListener := (udpInput.listener).(*net.UDPConn)
		c.Expect(realListener.LocalAddr().String(), gs.Equals, ith.ResolvedAddrStr)
		realListener.Close()

		// replace the listener object w/ a mock listener
		mockListener := ts.NewMockConn(ctrl)
		udpInput.listener = mockListener

		mbytes, _ := json.Marshal(ith.Msg)
		header := &message.Header{}
		header.SetMessageEncoding(message.Header_JSON)
		header.SetMessageLength(uint32(len(mbytes)))
		buf := make([]byte, message.MAX_MESSAGE_SIZE+message.MAX_HEADER_SIZE+3)
		readCall := mockListener.EXPECT().Read(buf)

		mockDecoderRunner := ith.Decoders[message.Header_JSON].(*MockDecoderRunner)
		mockDecoderRunner.EXPECT().InChan().Return(ith.DecodeChan)
		ith.MockInputRunner.EXPECT().InChan().Times(2).Return(ith.PackSupply)
		ith.MockHelper.EXPECT().DecoderSet().Return(ith.MockDecoderSet)
		encCall := ith.MockDecoderSet.EXPECT().ByEncodings()
		encCall.Return(ith.Decoders, nil)

		c.Specify("reads a message from the connection and passes it to the decoder", func() {
			hbytes, _ := proto.Marshal(header)
			buflen := 3 + len(hbytes) + len(mbytes)
			readCall.Return(buflen, nil)
			readCall.Do(getPayloadBytes(hbytes, mbytes))
			go func() {
				udpInput.Run(ith.MockInputRunner, ith.MockHelper)
			}()
			ith.PackSupply <- ith.Pack
			packRef := <-ith.DecodeChan
			c.Expect(ith.Pack, gs.Equals, packRef)
			c.Expect(string(ith.Pack.MsgBytes), gs.Equals, string(mbytes))
			c.Expect(ith.Pack.Decoded, gs.IsFalse)
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
			readCall.Return(buflen, err)
			readCall.Do(getPayloadBytes(hbytes, mbytes))
			go func() {
				udpInput.Run(ith.MockInputRunner, ith.MockHelper)
			}()
			ith.PackSupply <- ith.Pack
			packRef := <-ith.DecodeChan
			c.Expect(ith.Pack, gs.Equals, packRef)
			c.Expect(string(ith.Pack.MsgBytes), gs.Equals, string(mbytes))
			c.Expect(ith.Pack.Signer, gs.Equals, "test")
		})

		c.Specify("invalid header", func() {
			hbytes, _ := proto.Marshal(header)
			hbytes[0] = 0
			buflen := 3 + len(hbytes) + len(mbytes)
			readCall.Return(buflen, nil)
			readCall.Do(getPayloadBytes(hbytes, mbytes))
			go func() {
				udpInput.Run(ith.MockInputRunner, ith.MockHelper)
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

	c.Specify("A TcpInput", func() {
		tcpInput := TcpInput{}
		err := tcpInput.Init(&TcpInputConfig{ith.AddrStr, signers})
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
		buf := make([]byte, message.MAX_MESSAGE_SIZE+message.MAX_HEADER_SIZE+3)
		err = errors.New("connection closed") // used in the read return(s)
		readCall := mockConnection.EXPECT().Read(buf)

		neterr := ts.NewMockError(ctrl)
		neterr.EXPECT().Temporary().Return(false)
		acceptCall := mockListener.EXPECT().Accept().Return(mockConnection, nil)
		acceptCall.Do(func() {
			acceptCall = mockListener.EXPECT().Accept()
			acceptCall.Return(nil, neterr)
		})

		mockDecoderRunner := ith.Decoders[message.Header_PROTOCOL_BUFFER].(*MockDecoderRunner)
		mockDecoderRunner.EXPECT().InChan().Return(ith.DecodeChan)
		ith.MockInputRunner.EXPECT().InChan().Return(ith.PackSupply)
		ith.MockHelper.EXPECT().DecoderSet().Return(ith.MockDecoderSet)
		enccall := ith.MockDecoderSet.EXPECT().ByEncodings()
		enccall.Return(ith.Decoders, nil)

		c.Specify("reads a message from its connection", func() {
			hbytes, _ := proto.Marshal(header)
			buflen := 3 + len(hbytes) + len(mbytes)
			readCall.Return(buflen, err)
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
			readCall.Return(buflen, err)
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
			readCall.Return(buflen, err)
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
			readCall.Return(buflen, err)
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
			readCall.Return(buflen, err)
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
		cfgCall := ith.MockHelper.EXPECT().PipelineConfig().Times(3)
		cfgCall.Return(pc)
		wg.Add(1)
		iRunner.Start(ith.MockHelper, &wg)
		wg.Wait()
		c.Expect(stopinputTimes, gs.Equals, 2)
	})

	c.Specify("Runner recovers from panic in input's `Run()` method", func() {
		input := new(PanicInput)
		iRunner := NewInputRunner("panic", input, nil)
		var wg sync.WaitGroup
		cfgCall := ith.MockHelper.EXPECT().PipelineConfig()
		cfgCall.Return(config)
		wg.Add(1)
		iRunner.Start(ith.MockHelper, &wg) // no panic => success
		wg.Wait()
	})

	c.Specify("A LogFileInput", func() {
		var err error
		lfInput := new(LogfileInput)
		lfiConfig := lfInput.ConfigStruct().(*LogfileInputConfig)
		lfiConfig.SeekJournal, err = createJournal()
		c.Expect(err, gs.IsNil)
		lfiConfig.LogFile = "../testsupport/test-zeus.log"
		lfiConfig.Logger = "zeus"

		lfiConfig.DiscoverInterval = 1
		lfiConfig.StatInterval = 1
		err = lfInput.Init(lfiConfig)
		c.Expect(err, gs.IsNil)

		dName := "decoder-name"
		lfInput.decoderNames = []string{dName}
		mockDecoderRunner := NewMockDecoderRunner(ctrl)
		mockDecoder := NewMockDecoder(ctrl)

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
			ith.MockInputRunner.EXPECT().InChan().Return(ith.PackSupply)
			ith.MockInputRunner.EXPECT().Inject(gomock.Any()).Times(numLines)
			// Expect calls to get decoder and decode each message. Since the
			// decoding is a no-op, the message payload will be the log file
			// line, unchanged.
			ith.MockHelper.EXPECT().DecoderSet().Return(ith.MockDecoderSet)
			pbcall := ith.MockDecoderSet.EXPECT().ByName(dName)
			pbcall.Return(mockDecoderRunner, true)
			mockDecoderRunner.EXPECT().Decoder().Return(mockDecoder)
			decodeCall := mockDecoder.EXPECT().Decode(gomock.Any()).Times(numLines)
			decodeCall.Return(nil)
			go func() {
				err = lfInput.Run(ith.MockInputRunner, ith.MockHelper)
				c.Expect(err, gs.IsNil)
			}()
			for len(ith.PackSupply) > 0 {
				// Free up the scheduler while we wait for the log file lines
				// to be processed.
				runtime.Gosched()
			}

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
			close(lfInput.Monitor.stopChan)
		})

		c.Specify("uses the filename as the default logger name", func() {
			var err error

			lfInput := new(LogfileInput)
			lfiConfig := lfInput.ConfigStruct().(*LogfileInputConfig)
			lfiConfig.SeekJournal, err = createJournal()
			c.Expect(err, gs.Equals, nil)
			lfiConfig.LogFile = "../testsupport/test-zeus.log"

			lfiConfig.DiscoverInterval = 1
			lfiConfig.StatInterval = 1
			err = lfInput.Init(lfiConfig)
			c.Expect(err, gs.Equals, nil)

			c.Expect(lfInput.Monitor.logger_ident,
				gs.Equals,
				lfiConfig.LogFile)
		})
	})
}
