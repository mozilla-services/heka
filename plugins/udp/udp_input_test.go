/***** BEGIN LICENSE BLOCK *****
# This Source Code Form is subject to the terms of the Mozilla Public
# License, v. 2.0. If a copy of the MPL was not distributed with this file,
# You can obtain one at http://mozilla.org/MPL/2.0/.
#
# The Initial Developer of the Original Code is the Mozilla Foundation.
# Portions created by the Initial Developer are Copyright (C) 2012-2015
# the Initial Developer. All Rights Reserved.
#
# Contributor(s):
#   Rob Miller (rmiller@mozilla.com)
#   Victor Ng (vng@mozilla.com)
#
# ***** END LICENSE BLOCK *****/

package udp

import (
	"io/ioutil"
	"net"
	"path/filepath"
	"runtime"

	"github.com/gogo/protobuf/proto"
	"github.com/mozilla-services/heka/message"
	. "github.com/mozilla-services/heka/pipeline"
	pipeline_ts "github.com/mozilla-services/heka/pipeline/testsupport"
	"github.com/mozilla-services/heka/pipelinemock"
	plugins_ts "github.com/mozilla-services/heka/plugins/testsupport"
	"github.com/rafrombrc/gomock/gomock"
	gs "github.com/rafrombrc/gospec/src/gospec"
)

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
	ith.MockSplitterRunner = pipelinemock.NewMockSplitterRunner(ctrl)

	c.Specify("A UdpInput", func() {
		udpInput := UdpInput{}
		config := &UdpInputConfig{}

		mbytes, _ := proto.Marshal(ith.Msg)
		header := &message.Header{}
		header.SetMessageLength(uint32(len(mbytes)))
		hbytes, _ := proto.Marshal(header)
		buf := encodeMessage(hbytes, mbytes)

		bytesChan := make(chan []byte, 1)

		ith.MockInputRunner.EXPECT().Name().Return("mock_name")
		ith.MockInputRunner.EXPECT().NewSplitterRunner("").Return(ith.MockSplitterRunner)
		ith.MockSplitterRunner.EXPECT().Done().AnyTimes()
		ith.MockSplitterRunner.EXPECT().GetRemainingData().AnyTimes()
		ith.MockSplitterRunner.EXPECT().UseMsgBytes().Return(false)
		ith.MockSplitterRunner.EXPECT().SetPackDecorator(gomock.Any())

		splitCall := ith.MockSplitterRunner.EXPECT().SplitStream(gomock.Any(),
			nil).AnyTimes()
		splitCall.Do(func(conn net.Conn, del Deliverer) {
			recd := make([]byte, 65536)
			n, _ := conn.Read(recd)
			recd = recd[:n]
			bytesChan <- recd
		})

		c.Specify("using a udp address", func() {
			ith.AddrStr = "localhost:55565"
			ith.ResolvedAddrStr = "127.0.0.1:55565"
			config.Net = "udp"
			config.Address = ith.AddrStr

			err := udpInput.Init(config)
			c.Assume(err, gs.IsNil)
			realListener := (udpInput.listener).(*net.UDPConn)
			c.Expect(realListener.LocalAddr().String(), gs.Equals, ith.ResolvedAddrStr)

			c.Specify("passes the connection to SplitStream", func() {
				go udpInput.Run(ith.MockInputRunner, ith.MockHelper)

				conn, err := net.Dial("udp", ith.AddrStr)
				c.Assume(err, gs.IsNil)
				_, err = conn.Write(buf)
				c.Assume(err, gs.IsNil)
				conn.Close()

				recd := <-bytesChan
				c.Expect(string(recd), gs.Equals, string(buf))
				udpInput.Stop()
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

				c.Specify("passes the socket to SplitStream", func() {
					go udpInput.Run(ith.MockInputRunner, ith.MockHelper)

					unixAddr, err := net.ResolveUnixAddr("unixgram", unixPath)
					c.Assume(err, gs.IsNil)
					conn, err := net.DialUnix("unixgram", nil, unixAddr)
					c.Assume(err, gs.IsNil)
					_, err = conn.Write(buf)
					c.Assume(err, gs.IsNil)
					conn.Close()

					recd := <-bytesChan
					c.Expect(string(recd), gs.Equals, string(buf))
					udpInput.Stop()
				})
			})
		}

		if runtime.GOOS == "linux" {
			c.Specify("using an abstract unix domain socket", func() {
				unixPath := "@unixgram-socket"
				ith.AddrStr = unixPath
				config.Net = "unixgram"
				config.Address = ith.AddrStr

				err := udpInput.Init(config)
				c.Assume(err, gs.IsNil)
				realListener := (udpInput.listener).(*net.UnixConn)
				c.Expect(realListener.LocalAddr().String(), gs.Equals, unixPath)

				c.Specify("passes the socket to SplitStream", func() {
					go udpInput.Run(ith.MockInputRunner, ith.MockHelper)

					unixAddr, err := net.ResolveUnixAddr("unixgram", unixPath)
					c.Assume(err, gs.IsNil)
					conn, err := net.DialUnix("unixgram", nil, unixAddr)
					c.Assume(err, gs.IsNil)
					_, err = conn.Write(buf)
					c.Assume(err, gs.IsNil)
					conn.Close()

					recd := <-bytesChan
					c.Expect(string(recd), gs.Equals, string(buf))
					udpInput.Stop()
				})
			})
		}
	})
}

func UdpInputSpecFailure(c gs.Context) {
	udpInput := UdpInput{}
	err := udpInput.Init(&UdpInputConfig{
		Net:     "tcp",
		Address: "localhost:55565",
	})
	c.Assume(err, gs.Not(gs.IsNil))
	c.Assume(err.Error(), gs.Equals, "ResolveUDPAddr failed: unknown network tcp\n")

}
