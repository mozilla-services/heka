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
	"github.com/orfjackal/gospec/src/gospec"
	gs "github.com/orfjackal/gospec/src/gospec"
	"heka/testsupport"
	"net"
)

func InputsSpec(c gospec.Context) {
	t := testsupport.TestingT()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	// msg := getTestMessage()

	// putMsgInBytes := func([]byte) {
	// }

	c.Specify("A UdpInput", func() {
		mockListener := testsupport.NewMockConn(ctrl)
		// Specify localhost, but we're not really going to send packets out
		addrStr := "localhost:5565"
		resolvedAddrStr := "127.0.0.1:5565"
		udpInput := NewUdpInput(addrStr, nil)
		realListener := (udpInput.Listener).(*net.UDPConn)
		c.Expect(realListener.LocalAddr().String(), gs.Equals, resolvedAddrStr)
		udpInput.Listener = mockListener

		c.Specify("", func() {
		})
	})
}
