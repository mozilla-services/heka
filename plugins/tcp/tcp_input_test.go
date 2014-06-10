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

package tcp

import (
	"code.google.com/p/gomock/gomock"
	"code.google.com/p/goprotobuf/proto"
	"crypto/hmac"
	"crypto/md5"
	"crypto/sha1"
	"crypto/tls"
	"errors"
	"github.com/mozilla-services/heka/message"
	. "github.com/mozilla-services/heka/pipeline"
	pipeline_ts "github.com/mozilla-services/heka/pipeline/testsupport"
	"github.com/mozilla-services/heka/pipelinemock"
	plugins_ts "github.com/mozilla-services/heka/plugins/testsupport"
	gs "github.com/rafrombrc/gospec/src/gospec"
	"time"
)

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

func TcpInputSpec(c gs.Context) {
	t := &pipeline_ts.SimpleT{}
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	config := NewPipelineConfig(nil)
	ith := new(plugins_ts.InputTestHelper)
	ith.Msg = pipeline_ts.GetTestMessage()
	ith.Pack = NewPipelinePack(config.InputRecycleChan())

	ith.AddrStr = "localhost:55565"
	ith.ResolvedAddrStr = "127.0.0.1:55565"

	// set up mock helper, decoder set, and packSupply channel
	ith.MockHelper = pipelinemock.NewMockPluginHelper(ctrl)
	ith.MockInputRunner = pipelinemock.NewMockInputRunner(ctrl)
	ith.Decoder = pipelinemock.NewMockDecoderRunner(ctrl)
	ith.PackSupply = make(chan *PipelinePack, 1)
	ith.DecodeChan = make(chan *PipelinePack)
	key := "testkey"
	signers := map[string]Signer{"test_1": {key}}
	signer := "test"

	c.Specify("A TcpInput protobuf parser", func() {
		ith.MockInputRunner.EXPECT().Name().Return("TcpInput")
		tcpInput := TcpInput{}
		err := tcpInput.Init(&TcpInputConfig{Net: "tcp", Address: ith.AddrStr,
			Signers:    signers,
			Decoder:    "ProtobufDecoder",
			ParserType: "message.proto"})
		c.Assume(err, gs.IsNil)
		realListener := tcpInput.listener
		c.Expect(realListener.Addr().String(), gs.Equals, ith.ResolvedAddrStr)
		realListener.Close()

		mockConnection := pipeline_ts.NewMockConn(ctrl)
		mockListener := pipeline_ts.NewMockListener(ctrl)
		tcpInput.listener = mockListener

		addr := new(address)
		addr.str = "123"
		mockConnection.EXPECT().RemoteAddr().Return(addr)
		mbytes, _ := proto.Marshal(ith.Msg)
		header := &message.Header{}
		header.SetMessageLength(uint32(len(mbytes)))
		err = errors.New("connection closed") // used in the read return(s)
		readCall := mockConnection.EXPECT().Read(gomock.Any())
		readEnd := mockConnection.EXPECT().Read(gomock.Any()).After(readCall)
		readEnd.Return(0, err)
		mockConnection.EXPECT().SetReadDeadline(gomock.Any()).Return(nil).AnyTimes()
		mockConnection.EXPECT().Close()

		neterr := pipeline_ts.NewMockError(ctrl)
		neterr.EXPECT().Temporary().Return(false)
		acceptCall := mockListener.EXPECT().Accept().Return(mockConnection, nil)
		acceptCall.Do(func() {
			acceptCall = mockListener.EXPECT().Accept()
			acceptCall.Return(nil, neterr)
		})

		mockDecoderRunner := ith.Decoder.(*pipelinemock.MockDecoderRunner)
		mockDecoderRunner.EXPECT().InChan().Return(ith.DecodeChan)
		ith.MockInputRunner.EXPECT().InChan().Return(ith.PackSupply)
		enccall := ith.MockHelper.EXPECT().DecoderRunner("ProtobufDecoder", "TcpInput-123-ProtobufDecoder").AnyTimes()
		enccall.Return(ith.Decoder, true)
		ith.MockHelper.EXPECT().StopDecoderRunner(ith.Decoder)

		cleanup := func() {
			mockListener.EXPECT().Close()
			tcpInput.Stop()
			tcpInput.wg.Wait()
		}

		c.Specify("reads a message from its connection", func() {
			hbytes, _ := proto.Marshal(header)
			buflen := 3 + len(hbytes) + len(mbytes)
			readCall.Return(buflen, nil)
			readCall.Do(getPayloadBytes(hbytes, mbytes))
			go tcpInput.Run(ith.MockInputRunner, ith.MockHelper)
			defer cleanup()
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

			go tcpInput.Run(ith.MockInputRunner, ith.MockHelper)
			defer cleanup()
			ith.PackSupply <- ith.Pack
			timeout := make(chan bool, 1)
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

			go tcpInput.Run(ith.MockInputRunner, ith.MockHelper)
			defer cleanup()
			ith.PackSupply <- ith.Pack
			timeout := make(chan bool, 1)
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

			go tcpInput.Run(ith.MockInputRunner, ith.MockHelper)
			defer cleanup()
			ith.PackSupply <- ith.Pack
			timeout := make(chan bool, 1)
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

			go tcpInput.Run(ith.MockInputRunner, ith.MockHelper)
			defer cleanup()
			ith.PackSupply <- ith.Pack
			timeout := make(chan bool, 1)
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
		ith.MockInputRunner.EXPECT().Name().Return("TcpInput")

		config := &TcpInputConfig{
			Net:        "tcp",
			Address:    ith.AddrStr,
			Decoder:    "RegexpDecoder",
			ParserType: "regexp",
		}

		tcpInput := TcpInput{}
		err := tcpInput.Init(config)
		c.Assume(err, gs.IsNil)
		realListener := tcpInput.listener
		c.Expect(realListener.Addr().String(), gs.Equals, ith.ResolvedAddrStr)
		realListener.Close()

		mockConnection := pipeline_ts.NewMockConn(ctrl)
		mockListener := pipeline_ts.NewMockListener(ctrl)
		tcpInput.listener = mockListener

		addr := new(address)
		addr.str = "123"
		mockConnection.EXPECT().RemoteAddr().Return(addr).Times(2)
		mbytes := []byte("this is a test message\n")
		err = errors.New("connection closed") // used in the read return(s)
		readCall := mockConnection.EXPECT().Read(gomock.Any())
		readEnd := mockConnection.EXPECT().Read(gomock.Any()).After(readCall)
		readEnd.Return(0, err)
		mockConnection.EXPECT().SetReadDeadline(gomock.Any()).Return(nil).AnyTimes()
		mockConnection.EXPECT().Close()

		neterr := pipeline_ts.NewMockError(ctrl)
		neterr.EXPECT().Temporary().Return(false)
		acceptCall := mockListener.EXPECT().Accept().Return(mockConnection, nil)
		acceptCall.Do(func() {
			acceptCall = mockListener.EXPECT().Accept()
			acceptCall.Return(nil, neterr)
		})

		mockDecoderRunner := ith.Decoder.(*pipelinemock.MockDecoderRunner)
		mockDecoderRunner.EXPECT().InChan().Return(ith.DecodeChan)
		ith.MockInputRunner.EXPECT().InChan().Return(ith.PackSupply)
		ith.MockInputRunner.EXPECT().Name().Return("logger")
		enccall := ith.MockHelper.EXPECT().DecoderRunner("RegexpDecoder", "TcpInput-123-RegexpDecoder").AnyTimes()
		enccall.Return(ith.Decoder, true)
		ith.MockHelper.EXPECT().StopDecoderRunner(ith.Decoder)

		c.Specify("reads a message from its connection", func() {
			readCall.Return(len(mbytes), nil)
			readCall.Do(getPayloadText(mbytes))
			go tcpInput.Run(ith.MockInputRunner, ith.MockHelper)
			defer func() {
				mockListener.EXPECT().Close()
				tcpInput.Stop()
				tcpInput.wg.Wait()
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
		ith.MockInputRunner.EXPECT().Name().Return("TcpInput")
		tcpInput := TcpInput{}
		err := tcpInput.Init(&TcpInputConfig{Net: "tcp", Address: ith.AddrStr,
			Decoder:    "TokenDecoder",
			ParserType: "token",
			Delimiter:  "\n"})
		c.Assume(err, gs.IsNil)
		realListener := tcpInput.listener
		c.Expect(realListener.Addr().String(), gs.Equals, ith.ResolvedAddrStr)
		realListener.Close()

		mockConnection := pipeline_ts.NewMockConn(ctrl)
		mockListener := pipeline_ts.NewMockListener(ctrl)
		tcpInput.listener = mockListener

		addr := new(address)
		addr.str = "123"
		mockConnection.EXPECT().RemoteAddr().Return(addr).Times(2)
		mbytes := []byte("this is a test message\n")
		err = errors.New("connection closed") // used in the read return(s)
		readCall := mockConnection.EXPECT().Read(gomock.Any())
		readEnd := mockConnection.EXPECT().Read(gomock.Any()).After(readCall)
		readEnd.Return(0, err)
		mockConnection.EXPECT().SetReadDeadline(gomock.Any()).Return(nil).AnyTimes()
		mockConnection.EXPECT().Close()

		neterr := pipeline_ts.NewMockError(ctrl)
		neterr.EXPECT().Temporary().Return(false)
		acceptCall := mockListener.EXPECT().Accept().Return(mockConnection, nil)
		acceptCall.Do(func() {
			acceptCall = mockListener.EXPECT().Accept()
			acceptCall.Return(nil, neterr)
		})

		mockDecoderRunner := ith.Decoder.(*pipelinemock.MockDecoderRunner)
		mockDecoderRunner.EXPECT().InChan().Return(ith.DecodeChan)
		ith.MockInputRunner.EXPECT().InChan().Return(ith.PackSupply)
		ith.MockInputRunner.EXPECT().Name().Return("logger")
		enccall := ith.MockHelper.EXPECT().DecoderRunner("TokenDecoder", "TcpInput-123-TokenDecoder").AnyTimes()
		enccall.Return(ith.Decoder, true)
		ith.MockHelper.EXPECT().StopDecoderRunner(ith.Decoder)

		c.Specify("reads a message from its connection", func() {
			readCall.Return(len(mbytes), nil)
			readCall.Do(getPayloadText(mbytes))
			go tcpInput.Run(ith.MockInputRunner, ith.MockHelper)
			defer func() {
				mockListener.EXPECT().Close()
				tcpInput.Stop()
				tcpInput.wg.Wait()
			}()
			ith.PackSupply <- ith.Pack
			packRef := <-ith.DecodeChan
			c.Expect(ith.Pack, gs.Equals, packRef)
			c.Expect(ith.Pack.Message.GetPayload(), gs.Equals, string(mbytes))
			c.Expect(ith.Pack.Message.GetLogger(), gs.Equals, "logger")
			c.Expect(ith.Pack.Message.GetHostname(), gs.Equals, "123")
		})
	})

	c.Specify("A TcpInput using TLS", func() {
		tcpInput := TcpInput{}
		config := &TcpInputConfig{
			Net:        "tcp",
			Address:    ith.AddrStr,
			ParserType: "token",
			UseTls:     true,
		}

		c.Specify("fails to init w/ missing key or cert file", func() {
			config.Tls = TlsConfig{}
			err := tcpInput.Init(config)
			c.Expect(err, gs.Not(gs.IsNil))
		})

		c.Specify("accepts TLS client connections", func() {
			ith.MockInputRunner.EXPECT().Name().Return("TcpInput")
			config.Tls = TlsConfig{
				CertFile: "./testsupport/cert.pem",
				KeyFile:  "./testsupport/key.pem",
			}
			err := tcpInput.Init(config)
			c.Expect(err, gs.IsNil)
			go tcpInput.Run(ith.MockInputRunner, ith.MockHelper)
			defer func() {
				tcpInput.Stop()
				tcpInput.wg.Wait()
			}()

			clientConfig := new(tls.Config)
			clientConfig.InsecureSkipVerify = true
			conn, err := tls.Dial("tcp", ith.AddrStr, clientConfig)
			c.Expect(err, gs.IsNil)
			defer conn.Close()
			conn.SetWriteDeadline(time.Now().Add(time.Duration(10000)))
			n, err := conn.Write([]byte("This is a test."))
			c.Expect(err, gs.IsNil)
			c.Expect(n, gs.Equals, len("This is a test."))
		})

		c.Specify("doesn't accept connections below specified min TLS version", func() {
			ith.MockInputRunner.EXPECT().Name().Return("TcpInput")
			config.Tls = TlsConfig{
				CertFile:   "./testsupport/cert.pem",
				KeyFile:    "./testsupport/key.pem",
				MinVersion: "TLS12",
			}
			err := tcpInput.Init(config)
			c.Expect(err, gs.IsNil)
			go tcpInput.Run(ith.MockInputRunner, ith.MockHelper)
			defer func() {
				tcpInput.Stop()
				tcpInput.wg.Wait()
				time.Sleep(time.Duration(1000))
			}()

			clientConfig := &tls.Config{
				InsecureSkipVerify: true,
				MaxVersion:         tls.VersionTLS11,
			}
			conn, err := tls.Dial("tcp", ith.AddrStr, clientConfig)
			c.Expect(conn, gs.IsNil)
			c.Expect(err, gs.Not(gs.IsNil))
		})
	})
}

func TcpInputSpecFailure(c gs.Context) {
	tcpInput := TcpInput{}
	err := tcpInput.Init(&TcpInputConfig{Net: "udp", Address: "localhost:55565",
		Decoder:    "ProtobufDecoder",
		ParserType: "message.proto"})
	c.Assume(err, gs.Not(gs.IsNil))
	c.Assume(err.Error(), gs.Equals, "ListenTCP failed: unknown network udp\n")

}
