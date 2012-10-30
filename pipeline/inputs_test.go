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
	"encoding/json"
	"fmt"
	"github.com/orfjackal/gospec/src/gospec"
	gs "github.com/orfjackal/gospec/src/gospec"
	"heka/testsupport"
	"net"
	"time"
)

func InputsSpec(c gospec.Context) {
	t := &testsupport.SimpleT{}
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	msg := getTestMessage()
	msgJson, _ := json.Marshal(msg)
	pipelinePack := getTestPipelinePack()

	putMsgJsonInBytes := func(msgBytes []byte) {
		copy(msgBytes, msgJson)
	}

	c.Specify("A UdpInput", func() {
		mockListener := testsupport.NewMockConn(ctrl)
		// Specify localhost, but we're not really going to send packets out
		addrStr := "localhost:5565"
		resolvedAddrStr := "127.0.0.1:5565"
		udpInput := NewUdpInput(addrStr, nil)
		realListener := (udpInput.Listener).(*net.UDPConn)
		c.Expect(realListener.LocalAddr().String(), gs.Equals, resolvedAddrStr)

		// Replace the listener object w/ a mock listener
		udpInput.Listener = mockListener

		c.Specify("reads a message from its listener", func() {
			mockListener.EXPECT().SetReadDeadline(gomock.Any())
			readCall := mockListener.EXPECT().Read(pipelinePack.MsgBytes)
			readCall.Return(len(msgJson), nil)
			readCall.Do(putMsgJsonInBytes)
			second := time.Second
			err := udpInput.Read(pipelinePack, &second)
			c.Expect(err, gs.IsNil)
			c.Expect(pipelinePack.Decoded, gs.IsFalse)
			fmt.Println(msgJson)
			fmt.Println(pipelinePack.MsgBytes)
			c.Expect(string(pipelinePack.MsgBytes), gs.Equals, string(msgJson))
		})
	})
}
