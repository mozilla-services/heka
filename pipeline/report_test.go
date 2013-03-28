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
	"github.com/mozilla-services/heka/message"
	ts "github.com/mozilla-services/heka/testsupport"
	gs "github.com/rafrombrc/gospec/src/gospec"
)

var (
	f0, _ = message.NewField("test0", 0, message.Field_RAW)
	f1, _ = message.NewField("test1", "one", message.Field_RAW)
)

func (f *CounterFilter) ReportMsg(msg *message.Message) (err error) {
	msg.AddField(f0)
	msg.AddField(f1)
	return
}

func (i *UdpInput) ReportMsg(msg *message.Message) (err error) {
	msg.AddField(f0)
	msg.AddField(f1)
	return
}

func ReportSpec(c gs.Context) {
	t := new(ts.SimpleT)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	c.Specify("`PopulateReportMsg`", func() {
		msg := getTestMessage()

		c.Specify("w/ a filter", func() {
			name := "counter"
			filter := new(CounterFilter)
			runner := NewFORunner(name, filter)
			err := PopulateReportMsg(runner, msg)
			c.Assume(err, gs.IsNil)

			c.Specify("invokes `ReportMsg` on the filter", func() {
				f0Val, _ := msg.GetFieldValue(f0.GetName())
				f1Val, _ := msg.GetFieldValue(f1.GetName())
				c.Expect(f0Val.(int64), gs.Equals, f0.GetValue().(int64))
				c.Expect(f1Val.(string), gs.Equals, f1.GetValue().(string))
			})

			c.Specify("adds the channel data", func() {
				capVal, _ := msg.GetFieldValue("InChanCapacity")
				lenVal, _ := msg.GetFieldValue("InChanLength")
				c.Expect(capVal.(int64), gs.Equals, int64(PIPECHAN_BUFSIZE))
				c.Expect(lenVal.(int64), gs.Equals, int64(0))
			})
		})

		c.Specify("w/ an input", func() {
			name := "udp"
			input := new(UdpInput)
			runner := NewInputRunner(name, input)
			err := PopulateReportMsg(runner, msg)
			c.Assume(err, gs.IsNil)

			c.Specify("invokes `ReportMsg` on the input", func() {
				f0Val, ok := msg.GetFieldValue(f0.GetName())
				c.Expect(ok, gs.IsTrue)
				c.Expect(f0Val.(int64), gs.Equals, f0.GetValue().(int64))
				f1Val, ok := msg.GetFieldValue(f1.GetName())
				c.Expect(ok, gs.IsTrue)
				c.Expect(f1Val.(string), gs.Equals, f1.GetValue().(string))
			})

			c.Specify("doesn't add any channel data", func() {
				capVal, ok := msg.GetFieldValue("InChanCapacity")
				c.Expect(capVal, gs.IsNil)
				c.Expect(ok, gs.IsFalse)
				lenVal, ok := msg.GetFieldValue("InChanCapacity")
				c.Expect(lenVal, gs.IsNil)
				c.Expect(ok, gs.IsFalse)
			})
		})
	})
}
