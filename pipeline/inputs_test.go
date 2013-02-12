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
	"encoding/json"
	"errors"
	"github.com/mozilla-services/heka/message"
	ts "github.com/mozilla-services/heka/testsupport"
	gs "github.com/rafrombrc/gospec/src/gospec"
	"net"
	"sync"
	"time"
)

func InputsSpec(c gs.Context) {
	t := &ts.SimpleT{}
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	msg := getTestMessage()
	pipelinePack := getTestPipelinePack()

	// Specify localhost, but we're not really going to use the network
	addrStr := "localhost:55565"
	resolvedAddrStr := "127.0.0.1:55565"

	c.Specify("A UdpInput", func() {
		udpInput := UdpInput{}
		err := udpInput.Init(&UdpInputConfig{addrStr, "JsonDecoder"})
		c.Assume(err, gs.IsNil)
		realListener := (udpInput.listener).(*net.UDPConn)
		c.Expect(realListener.LocalAddr().String(), gs.Equals, resolvedAddrStr)
		realListener.Close()

		// Replace the listener object w/ a mock listener
		mockListener := ts.NewMockConn(ctrl)
		udpInput.listener = mockListener
		config = NewPipelineConfig(0)
		var wg sync.WaitGroup
		udpInput.Start(config, wg)

		msgJson, _ := json.Marshal(msg)
		putMsgJsonInBytes := func(msgBytes []byte) {
			copy(msgBytes, msgJson)
		}

		c.Specify("reads a message from its listener", func() {
			mockListener.EXPECT().SetReadDeadline(gomock.Any())
			readCall := mockListener.EXPECT().Read(pipelinePack.MsgBytes)
			readCall.Return(len(msgJson), nil)
			readCall.Do(putMsgJsonInBytes)
			second := time.Second
			err := udpInput.Read(pipelinePack, &second)
			c.Expect(err, gs.IsNil)
			c.Expect(pipelinePack.Decoded, gs.IsFalse)
			c.Expect(string(pipelinePack.MsgBytes), gs.Equals, string(msgJson))
		})
	})

	c.Specify("A TcpInput", func() {
		tcpInput := TcpInput{}
		err := tcpInput.Init(&TcpInputConfig{addrStr})
		c.Assume(err, gs.IsNil)
		mockConnection := ts.NewMockConn(ctrl)

		/// @todo use the msg encoder
		mbytes, _ := proto.Marshal(msg)
		header := &message.Header{}
		header.SetMessageLength(uint32(len(mbytes)))
		hbytes, _ := proto.Marshal(header)
		buflen := 3 + len(hbytes) + len(mbytes)
		putPayloadInBytes := func(msgBytes []byte) {
			msgBytes[0] = message.RECORD_SEPARATOR
			msgBytes[1] = uint8(len(hbytes))
			copy(msgBytes[2:], hbytes)
			pos := 2 + len(hbytes)
			msgBytes[pos] = message.UNIT_SEPARATOR
			copy(msgBytes[pos+1:], mbytes)
		}

		c.Specify("reads a message from its connection", func() {
			buf := make([]byte, message.MAX_MESSAGE_SIZE+message.MAX_HEADER_SIZE)
			err = errors.New("connection closed")
			closeCall := mockConnection.EXPECT().Close()
			closeCall.Do(func() {})
			readCall := mockConnection.EXPECT().Read(buf)
			readCall.Return(buflen, err)
			readCall.Do(putPayloadInBytes)
			second := time.Second
			tcpInput.handleConnection(mockConnection)
			err = tcpInput.Read(pipelinePack, &second)
			c.Expect(err, gs.IsNil)
			c.Expect(pipelinePack.Decoded, gs.IsTrue)
			v, ok := pipelinePack.Message.GetFieldValue("foo")
			c.Expect(ok, gs.IsTrue)
			c.Expect(v, gs.Equals, "bar")
		})
	})
}
