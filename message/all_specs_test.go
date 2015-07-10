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
#   Mike Trinkala (trink@mozilla.com)
#
# ***** END LICENSE BLOCK *****/
package message

import (
	"bytes"
	"os"
	"testing"
	"time"

	"github.com/pborman/uuid"
	"github.com/rafrombrc/gospec/src/gospec"
	gs "github.com/rafrombrc/gospec/src/gospec"
)

func TestAllSpecs(t *testing.T) {
	r := gospec.NewRunner()
	r.AddSpec(MessageFieldsSpec)
	r.AddSpec(MessageEqualsSpec)
	r.AddSpec(MatcherSpecificationSpec)
	gospec.MainGoTest(r, t)
}

func getTestMessage() *Message {
	hostname, _ := os.Hostname()
	field, _ := NewField("foo", "bar", "")
	field1, _ := NewField("number", 64, "")
	msg := &Message{}
	msg.SetType("TEST")
	msg.SetTimestamp(time.Now().UnixNano())
	msg.SetUuid(uuid.NewRandom())
	msg.SetLogger("GoSpec")
	msg.SetSeverity(int32(6))
	msg.SetPayload("Test Payload")
	msg.SetEnvVersion("0.8")
	msg.SetPid(int32(os.Getpid()))
	msg.SetHostname(hostname)
	msg.AddField(field)
	msg.AddField(field1)

	return msg
}

func MessageFieldsSpec(c gospec.Context) {
	c.Specify("No Fields", func() {
		msg := &Message{}
		f := msg.FindFirstField("test")
		c.Expect(f, gs.IsNil)
		fa := msg.FindAllFields("test")
		c.Expect(len(fa), gs.Equals, 0)
		v, ok := msg.GetFieldValue("test")
		c.Expect(ok, gs.IsFalse)
		c.Expect(v, gs.IsNil)
	})

	c.Specify("Fields present but none match", func() {
		msg := &Message{}
		f, _ := NewField("foo", "bar", "")
		msg.AddField(f)
		ff := msg.FindFirstField("test")
		c.Expect(ff, gs.IsNil)
		fa := msg.FindAllFields("test")
		c.Expect(len(fa), gs.Equals, 0)
		v, ok := msg.GetFieldValue("test")
		c.Expect(ok, gs.IsFalse)
		c.Expect(v, gs.IsNil)
	})

	c.Specify("Fields match", func() {
		msg := &Message{}
		f, _ := NewField("foo", "bar", "")
		f1, _ := NewField("other", "value", "")
		f2, _ := NewField("foo", "bar1", "")
		msg.AddField(f)
		msg.AddField(f1)
		msg.AddField(f2)
		ff := msg.FindFirstField("foo")
		c.Expect(ff.ValueString[0], gs.Equals, "bar")
		v, ok := msg.GetFieldValue("foo")
		c.Expect(ok, gs.IsTrue)
		c.Expect(v, gs.Equals, "bar")
		fa := msg.FindAllFields("foo")
		c.Expect(len(fa), gs.Equals, 2)
		fa[0].ValueString[0] = "bar"
		fa[1].ValueString[0] = "bar1"
	})

	c.Specify("Add Bytes Field", func() {
		msg := &Message{}
		b := make([]byte, 2)
		b[0] = 'a'
		b[1] = 'b'
		f, _ := NewField("foo", b, "")
		msg.AddField(f)
		ff := msg.FindFirstField("foo")
		c.Expect(bytes.Equal(ff.ValueBytes[0], b), gs.IsTrue)
		v, ok := msg.GetFieldValue("foo")
		c.Expect(ok, gs.IsTrue)
		c.Expect(bytes.Equal(v.([]byte), b), gs.IsTrue)
	})

	c.Specify("Add Integer Field", func() {
		representation := "ns"
		msg := &Message{}
		f, _ := NewField("foo", 1, representation)
		msg.AddField(f)
		ff := msg.FindFirstField("foo")
		c.Expect(ff.GetRepresentation(), gs.Equals, representation)
		c.Expect(ff.ValueInteger[0], gs.Equals, int64(1))
		v, ok := msg.GetFieldValue("foo")
		c.Expect(ok, gs.IsTrue)
		c.Expect(v, gs.Equals, int64(1))
	})

	c.Specify("Add Double Field", func() {
		msg := &Message{}
		f, _ := NewField("foo", 1e9, "")
		msg.AddField(f)
		ff := msg.FindFirstField("foo")
		c.Expect(ff.ValueDouble[0], gs.Equals, 1e9)
		v, ok := msg.GetFieldValue("foo")
		c.Expect(ok, gs.IsTrue)
		c.Expect(v, gs.Equals, 1e9)
	})

	c.Specify("Add Bool Field", func() {
		msg := &Message{}
		f, _ := NewField("foo", true, "")
		msg.AddField(f)
		ff := msg.FindFirstField("foo")
		c.Expect(ff.ValueBool[0], gs.IsTrue)
		v, ok := msg.GetFieldValue("foo")
		c.Expect(ok, gs.IsTrue)
		c.Expect(v, gs.IsTrue)
	})

	c.Specify("Copy with nil field attributes", func() {
		msg := &Message{}
		field, _ := NewField("foo", "bar", "")
		msg.AddField(field)
		msg.Fields[0].Representation = nil
		cmsg := CopyMessage(msg)
		v1, _ := msg.GetFieldValue("foo")
		v2, _ := cmsg.GetFieldValue("foo")
		c.Expect(v1, gs.Equals, v2)
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
		f, _ := NewField("sna", "foo", "")
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
		msg0 = &Message{}
		f, _ := NewField("foo", "bar", "")
		f1, _ := NewField("foo", "bar1", "")
		msg0.AddField(f)
		msg0.AddField(f1)
		msg1 := CopyMessage(msg0)
		c.Expect(msg0, gs.Equals, msg1)
		foos := msg0.FindAllFields("foo")
		foos[1].ValueString[0] = "bar2"
		c.Expect(msg0, gs.Not(gs.Equals), msg1)
	})
}

func BenchmarkMessageCreation(b *testing.B) {
	for i := 0; i < b.N; i++ {
		msg := getTestMessage()
		msg.SetPid(999)
	}
}
