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
#   Mike Trinkala (trink@mozilla.com)
#
# ***** END LICENSE BLOCK *****/
package message

import (
	"bytes"
	"code.google.com/p/go-uuid/uuid"
	"github.com/rafrombrc/gospec/src/gospec"
	gs "github.com/rafrombrc/gospec/src/gospec"
	"os"
	"testing"
	"time"
)

func TestAllSpecs(t *testing.T) {
	r := gospec.NewRunner()
	r.AddSpec(MessageFieldsSpec)
	r.AddSpec(MessageEqualsSpec)
	gospec.MainGoTest(r, t)
}

func getTestMessage() *Message {
	hostname, _ := os.Hostname()
	field, _ := NewField("foo", "bar", Field_RAW)
	msg := NewMessage()
	*msg.Type = "TEST"
	*msg.Timestamp = time.Now().UnixNano()
	u := uuid.NewRandom()
	copy(msg.Uuid, u)
	*msg.Logger = "GoSpec"
	*msg.Severity = int32(6)
	*msg.Payload = "Test Payload"
	*msg.EnvVersion = "0.8"
	*msg.Pid = int32(os.Getpid())
	*msg.Hostname = hostname
	msg.AddField(field)

	return msg
}

func MessageFieldsSpec(c gospec.Context) {
	c.Specify("No Fields", func() {
		msg := NewMessage()
		f := msg.FindFirstField("test")
		c.Expect(f, gs.IsNil)
		fa := msg.FindAllFields("test")
		c.Expect(len(fa), gs.Equals, 0)
	})

	c.Specify("Fields present but none match", func() {
		msg := NewMessage()
		f, _ := NewField("foo", "bar", Field_RAW)
		msg.AddField(f)
		ff := msg.FindFirstField("test")
		c.Expect(ff, gs.IsNil)
		fa := msg.FindAllFields("test")
		c.Expect(len(fa), gs.Equals, 0)
	})

	c.Specify("Fields match", func() {
		msg := NewMessage()
		f, _ := NewField("foo", "bar", Field_RAW)
		f1, _ := NewField("other", "value", Field_RAW)
		f2, _ := NewField("foo", "bar1", Field_RAW)
		msg.AddField(f)
		msg.AddField(f1)
		msg.AddField(f2)
		ff := msg.FindFirstField("foo")
		c.Expect(ff.ValueString[0], gs.Equals, "bar")
		fa := msg.FindAllFields("foo")
		c.Expect(len(fa), gs.Equals, 2)
		fa[0].ValueString[0] = "bar"
		fa[1].ValueString[0] = "bar1"
	})

	c.Specify("Add Bytes Field", func() {
		msg := NewMessage()
		b := make([]byte, 2)
		b[0] = 'a'
		b[1] = 'b'
		f, _ := NewField("foo", b, Field_RAW)
		msg.AddField(f)
		ff := msg.FindFirstField("foo")
		c.Expect(bytes.Equal(ff.ValueBytes[0], b), gs.IsTrue)
	})

	c.Specify("Add Integer Field", func() {
		msg := NewMessage()
		f, _ := NewField("foo", 1, Field_RAW)
		msg.AddField(f)
		ff := msg.FindFirstField("foo")
		c.Expect(ff.ValueInteger[0] == 1, gs.IsTrue)
	})

	c.Specify("Add Double Field", func() {
		msg := NewMessage()
		f, _ := NewField("foo", 5e9, Field_RAW)
		msg.AddField(f)
		ff := msg.FindFirstField("foo")
		c.Expect(ff.ValueDouble[0], gs.Equals, 5e9)
	})

	c.Specify("Add Bool Field", func() {
		msg := NewMessage()
		f, _ := NewField("foo", true, Field_RAW)
		msg.AddField(f)
		ff := msg.FindFirstField("foo")
		c.Expect(ff.ValueBool[0], gs.IsTrue)
	})

}

func MessageEqualsSpec(c gospec.Context) {
	msg0 := getTestMessage()

	c.Specify("Messages are equal", func() {
		msg1 := CopyMessage(msg0)
		c.Expect(msg0, gs.Equals, msg1)
	})

	c.Specify("Messages w/ diff severity", func() {
		msg1 := CopyMessage(msg0)
		*msg1.Severity--
		c.Expect(msg0, gs.Not(gs.Equals), msg1)
	})

	c.Specify("Messages w/ diff uuid", func() {
		msg1 := CopyMessage(msg0)
		u := uuid.NewRandom()
		copy(msg1.Uuid, u)
		c.Expect(msg0, gs.Not(gs.Equals), msg1)
	})

	c.Specify("Messages w/ diff payload", func() {
		msg1 := CopyMessage(msg0)
		*msg1.Payload = "Something completely different"
		c.Expect(msg0, gs.Not(gs.Equals), msg1)
	})

	c.Specify("Messages w/ diff number of fields", func() {
		msg1 := CopyMessage(msg0)
		f, _ := NewField("sna", "foo", Field_RAW)
		msg1.AddField(f)
		c.Expect(msg0, gs.Not(gs.Equals), msg1)
	})

	c.Specify("Messages w/ diff number of field values in a key", func() {
		msg1 := CopyMessage(msg0)
		f := msg1.FindFirstField("foo")
		f.AddValue("foo1")
		c.Expect(msg0, gs.Not(gs.Equals), msg1)
	})

	c.Specify("Messages w/ diff value in a field", func() {
		msg1 := CopyMessage(msg0)
		f := msg1.FindFirstField("foo")
		f.ValueString[0] = "bah"
		c.Expect(msg0, gs.Not(gs.Equals), msg1)
	})

	c.Specify("Messages w/ diff field key", func() {
		msg1 := CopyMessage(msg0)
		f := msg1.FindFirstField("foo")
		*f.Name = "widget"
		c.Expect(msg0, gs.Not(gs.Equals), msg1)
	})

	c.Specify("Messages w/ recurring keys", func() {
		msg0 = NewMessage()
		f, _ := NewField("foo", "bar", Field_RAW)
		f1, _ := NewField("foo", "bar1", Field_RAW)
		msg0.AddField(f)
		msg0.AddField(f1)
		msg1 := CopyMessage(msg0)
		c.Expect(msg0, gs.Equals, msg1)
		foos := msg0.FindAllFields("foo")
		foos[1].ValueString[0] = "bar2"
		c.Expect(msg0, gs.Not(gs.Equals), msg1)
	})
}
