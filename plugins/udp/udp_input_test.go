/***** BEGIN LICENSE BLOCK *****
# This Source Code Form is subject to the terms of the Mozilla Public
# License, v. 2.0. If a copy of the MPL was not distributed with this file,
# You can obtain one at http://mozilla.org/MPL/2.0/.
#
# The Initial Developer of the Original Code is the Mozilla Foundation.
# Portions created by the Initial Developer are Copyright (C) 2012-2014
# the Initial Developer. All Rights Reserved.
#
# Contributor(s):
#   Rob Miller (rmiller@mozilla.com)
#   Victor Ng (vng@mozilla.com)
#
# ***** END LICENSE BLOCK *****/

package udp

import (
	"code.google.com/p/gomock/gomock"
	"code.google.com/p/goprotobuf/proto"
	"github.com/mozilla-services/heka/message"
	. "github.com/mozilla-services/heka/pipeline"
	pipeline_ts "github.com/mozilla-services/heka/pipeline/testsupport"
	"github.com/mozilla-services/heka/pipelinemock"
	plugins_ts "github.com/mozilla-services/heka/plugins/testsupport"
	gs "github.com/rafrombrc/gospec/src/gospec"
	"io/ioutil"
	"net"
	"path/filepath"
	"runtime"
	"testing"
)

func TestAllSpecs(t *testing.T) {
	r := gs.NewRunner()
	r.Parallel = false

	r.AddSpec(UdpInputSpec)
	r.AddSpec(UdpInputSpecFailure)

	gs.MainGoTest(r, t)
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

func UdpInputSpec(c gs.Context) {
	t := &pipeline_ts.SimpleT{}
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	config := NewPipelineConfig(nil)
	ith := new(plugins_ts.InputTestHelper)
	ith.Msg = pipeline_ts.GetTestMessage()
	ith.Pack = NewPipelinePack(config.InputRecycleChan())

	// set up mock helper, decoder set, and packSupply channel
	ith.MockHelper = pipelinemock.NewMockPluginHelper(ctrl)
	ith.MockInputRunner = pipelinemock.NewMockInputRunner(ctrl)
	ith.Decoder = pipelinemock.NewMockDecoderRunner(ctrl)
	ith.PackSupply = make(chan *PipelinePack, 1)
	ith.DecodeChan = make(chan *PipelinePack)

	c.Specify("A UdpInput", func() {
		udpInput := UdpInput{}
		config := &UdpInputConfig{
			Decoder:    "ProtobufDecoder",
			ParserType: "message.proto",
		}

		mbytes, _ := proto.Marshal(ith.Msg)
		header := &message.Header{}
		header.SetMessageLength(uint32(len(mbytes)))
		hbytes, _ := proto.Marshal(header)
		buf := encodeMessage(hbytes, mbytes)

		mockDecoderRunner := ith.Decoder.(*pipelinemock.MockDecoderRunner)
		mockDecoderRunner.EXPECT().InChan().Return(ith.DecodeChan)
		ith.MockInputRunner.EXPECT().InChan().Return(ith.PackSupply)
		ith.MockInputRunner.EXPECT().Name().Return("UdpInput")
		encCall := ith.MockHelper.EXPECT().DecoderRunner("ProtobufDecoder",
			"UdpInput-ProtobufDecoder")
		encCall.Return(ith.Decoder, true)

		c.Specify("using a udp address", func() {
			ith.AddrStr = "localhost:55565"
			ith.ResolvedAddrStr = "127.0.0.1:55565"
			config.Net = "udp"
			config.Address = ith.AddrStr

			err := udpInput.Init(config)
			c.Assume(err, gs.IsNil)
			realListener := (udpInput.listener).(*net.UDPConn)
			c.Expect(realListener.LocalAddr().String(), gs.Equals, ith.ResolvedAddrStr)

			c.Specify("reads a message from the connection and passes it to the decoder", func() {
				go func() {
					udpInput.Run(ith.MockInputRunner, ith.MockHelper)
				}()
				conn, err := net.Dial("udp", ith.AddrStr) // a mock connection will not work here since the mock read cannot block
				c.Assume(err, gs.IsNil)
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

		if runtime.GOOS != "windows" {
			c.Specify("using a unix datagram socket", func() {
				tmpDir, err := ioutil.TempDir("", "heka-socket")
				c.Assume(err, gs.IsNil)
				unixPath := filepath.Join(tmpDir, "unixgram-socket")
				ith.AddrStr = unixPath
				config.Net = "unixgram"
				config.Address = ith.AddrStr

				err = udpInput.Init(config)
				c.Assume(err, gs.IsNil)
				realListener := (udpInput.listener).(*net.UnixConn)
				c.Expect(realListener.LocalAddr().String(), gs.Equals, unixPath)

				c.Specify("reads a message from the socket and passes it to the decoder", func() {
					go func() {
						udpInput.Run(ith.MockInputRunner, ith.MockHelper)
					}()
					unixAddr, err := net.ResolveUnixAddr("unixgram", unixPath)
					c.Assume(err, gs.IsNil)
					conn, err := net.DialUnix("unixgram", nil, unixAddr)
					c.Assume(err, gs.IsNil)
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
		}
	})

	c.Specify("A UdpInput Multiline input", func() {
		ith.AddrStr = "localhost:55566"
		ith.ResolvedAddrStr = "127.0.0.1:55566"
		udpInput := UdpInput{}
		err := udpInput.Init(&UdpInputConfig{Net: "udp", Address: ith.AddrStr,
			Decoder:    "test",
			ParserType: "token"})
		c.Assume(err, gs.IsNil)
		realListener := (udpInput.listener).(*net.UDPConn)
		c.Expect(realListener.LocalAddr().String(), gs.Equals, ith.ResolvedAddrStr)

		mockDecoderRunner := ith.Decoder.(*pipelinemock.MockDecoderRunner)
		mockDecoderRunner.EXPECT().InChan().Return(ith.DecodeChan).Times(2)
		ith.MockInputRunner.EXPECT().InChan().Return(ith.PackSupply).Times(2)
		ith.MockInputRunner.EXPECT().Name().Return("UdpInput").AnyTimes()
		encCall := ith.MockHelper.EXPECT().DecoderRunner("test", "UdpInput-test")
		encCall.Return(ith.Decoder, true)

		c.Specify("reads two messages from a packet and passes them to the decoder", func() {
			go func() {
				udpInput.Run(ith.MockInputRunner, ith.MockHelper)
			}()
			conn, err := net.Dial("udp", ith.AddrStr) // a mock connection will not work here since the mock read cannot block
			c.Assume(err, gs.IsNil)
			_, err = conn.Write([]byte("message1\nmessage2\n"))
			c.Assume(err, gs.IsNil)
			ith.PackSupply <- ith.Pack
			packRef := <-ith.DecodeChan
			c.Expect(string(packRef.Message.GetPayload()), gs.Equals, "message1\n")
			ith.PackSupply <- ith.Pack
			packRef = <-ith.DecodeChan
			c.Expect(string(packRef.Message.GetPayload()), gs.Equals, "message2\n")
			udpInput.Stop()
		})
	})
}

func UdpInputSpecFailure(c gs.Context) {
	udpInput := UdpInput{}
	err := udpInput.Init(&UdpInputConfig{Net: "tcp", Address: "localhost:55565",
		Decoder:    "ProtobufDecoder",
		ParserType: "message.proto"})
	c.Assume(err, gs.Not(gs.IsNil))
	c.Assume(err.Error(), gs.Equals, "ResolveUDPAddr failed: unknown network tcp\n")

}
