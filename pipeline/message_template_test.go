/***** BEGIN LICENSE BLOCK *****
# This Source Code Form is subject to the terms of the Mozilla Public
# License, v. 2.0. If a copy of the MPL was not distributed with this file,
# You can obtain one at http://mozilla.org/MPL/2.0/.
#
# The Initial Developer of the Original Code is the Mozilla Foundation.
# Portions created by the Initial Developer are Copyright (C) 2014
# the Initial Developer. All Rights Reserved.
#
# Contributor(s):
#   Rob Miller (rmiller@mozilla.com)
#
# ***** END LICENSE BLOCK *****/

package pipeline

import (
	ts "github.com/mozilla-services/heka/pipeline/testsupport"
	gs "github.com/rafrombrc/gospec/src/gospec"
)

func MessageTemplateSpec(c gs.Context) {
	c.Specify("A message template", func() {
		mt := make(MessageTemplate)
		mt["Logger"] = "test logger"
		mt["Payload"] = "test payload"
		mt["Hostname"] = "host.example.com"
		mt["Pid"] = "123.456"
		mt["Type"] = "test type"
		mt["Severity"] = "23"
		mt["tmplTest|baz"] = "bar"
		msg := ts.GetTestMessage()

		c.Specify("replaces message values", func() {
			err := mt.PopulateMessage(msg, nil)
			c.Assume(err, gs.IsNil)
			c.Expect(msg.GetLogger(), gs.Equals, mt["Logger"])
			c.Expect(msg.GetPayload(), gs.Equals, mt["Payload"])
			c.Expect(msg.GetHostname(), gs.Equals, mt["Hostname"])
			c.Expect(msg.GetPid(), gs.Equals, int32(123))
			c.Expect(msg.GetType(), gs.Equals, mt["Type"])
			c.Expect(msg.GetSeverity(), gs.Equals, int32(23))
			fields := msg.FindAllFields("tmplTest")
			c.Expect(len(fields), gs.Equals, 1)
			field := fields[0]
			value := field.GetValueString()
			c.Expect(len(value), gs.Equals, 1)
			c.Expect(value[0], gs.Equals, "bar")
			c.Expect(field.GetRepresentation(), gs.Equals, "baz")
		})

		c.Specify("honors substitutions", func() {
			mt["Payload"] = "this is %substitution%"
			mt["Hostname"] = "%host%.example.com"
			mt["Type"] = "another %substitution%"
			mt["tmplTest|baz"] = "%fieldvalue%"
			subs := make(map[string]string)
			subs["host"] = "otherhost"
			subs["substitution"] = "a test"
			subs["fieldvalue"] = "wakajawaka"
			err := mt.PopulateMessage(msg, subs)
			c.Assume(err, gs.IsNil)
			c.Expect(msg.GetLogger(), gs.Equals, mt["Logger"])
			c.Expect(msg.GetPayload(), gs.Equals, "this is a test")
			c.Expect(msg.GetHostname(), gs.Equals, "otherhost.example.com")
			c.Expect(msg.GetPid(), gs.Equals, int32(123))
			c.Expect(msg.GetType(), gs.Equals, "another a test")
			c.Expect(msg.GetSeverity(), gs.Equals, int32(23))
			fields := msg.FindAllFields("tmplTest")
			c.Expect(len(fields), gs.Equals, 1)
			field := fields[0]
			value := field.GetValueString()
			c.Expect(len(value), gs.Equals, 1)
			c.Expect(value[0], gs.Equals, subs["fieldvalue"])
			c.Expect(field.GetRepresentation(), gs.Equals, "baz")
		})
	})
}
