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

package pipeline

import (
	"code.google.com/p/goprotobuf/proto"
	"encoding/json"
	"fmt"
	"github.com/mozilla-services/heka/message"
	ts "github.com/mozilla-services/heka/testsupport"
	"github.com/rafrombrc/gospec/src/gospec"
	gs "github.com/rafrombrc/gospec/src/gospec"
	"testing"
	"time"
)

type PanicDecoder struct{}

func (p *PanicDecoder) Init(config interface{}) (err error) {
	return
}

func (p *PanicDecoder) Decode(pack *PipelinePack) (err error) {
	panic("PANICDECODER")
	return
}

func DecodersSpec(c gospec.Context) {
	msg := getTestMessage()

	c.Specify("A JsonDecoder", func() {
		var fmtString = `{"uuid":"%s","type":"%s","timestamp":%s,"logger":"%s","severity":%d,"payload":"%s","fields":%s,"env_version":"%s","metlog_pid":%d,"metlog_hostname":"%s"}`
		timestampJson, err := json.Marshal(time.Unix(*msg.Timestamp/1e9, *msg.Timestamp%1e9))
		fieldsJson := `{"foo":"bar"}`
		c.Assume(err, gs.IsNil)
		uuid := msg.GetUuidString()
		jsonString := fmt.Sprintf(fmtString, uuid, *msg.Type,
			timestampJson, *msg.Logger, *msg.Severity, *msg.Payload,
			fieldsJson, *msg.EnvVersion, *msg.Pid, *msg.Hostname)

		pipelinePack := getTestPipelinePack()
		pipelinePack.MsgBytes = []byte(jsonString)
		jsonDecoder := new(JsonDecoder)

		c.Specify("can decode a JSON message", func() {
			err := jsonDecoder.Decode(pipelinePack)
			c.Expect(pipelinePack.Message, gs.Equals, msg)
			c.Expect(err, gs.IsNil)
		})

		c.Specify("returns `fields` as a array", func() {
			jsonDecoder.Decode(pipelinePack)
			f := pipelinePack.Message.FindFirstField("foo")
			c.Expect(*f.Name, gs.Equals, "foo")
			c.Expect(*f.ValueType, gs.Equals, message.Field_STRING)
			c.Expect(*f.ValueFormat, gs.Equals, message.Field_RAW)
			c.Expect(f.ValueString[0], gs.Equals, "bar")
		})

		c.Specify("returns an error for bogus JSON", func() {
			badJson := fmt.Sprint("{{", jsonString)
			pipelinePack.MsgBytes = []byte(badJson)
			err := jsonDecoder.Decode(pipelinePack)
			c.Expect(err, gs.Not(gs.IsNil))
			c.Expect(pipelinePack.Message.GetTimestamp() == int64(0), gs.IsTrue)
		})

		c.Specify("returns an error for value array type mismatch", func() {
			Json := `{"uuid":"2ae75e3a-7a70-4686-a2ea-ac7a02db9542","type":"TEST","timestamp":"2013-01-23T08:00:47.797575607-08:00","logger":"GoSpec","severity":6,"payload":"Test Payload","fields":{"foo":["bar", 1]},"env_version":"0.8","metlog_pid":10569,"metlog_hostname":"trink-x230"}`
			pipelinePack.MsgBytes = []byte(Json)
			err := jsonDecoder.Decode(pipelinePack)
			c.Expect(err.Error(), ts.StringContains, "The field contains: STRING; attempted to add DOUBLE")
		})

		c.Specify("returns foo as flattened map", func() {
			Json := `{"uuid":"2ae75e3a-7a70-4686-a2ea-ac7a02db9542","type":"TEST","timestamp":"2013-01-23T08:00:47.797575607-08:00","logger":"GoSpec","severity":6,"payload":"Test Payload","fields":{"foo":{"bar":1,"widget": {"name":"w1","price":10.10,"in_stock":true, "locations":["sfo"]}}},"env_version":"0.8","metlog_pid":10569,"metlog_hostname":"trink-x230"}`
			pipelinePack.MsgBytes = []byte(Json)
			err := jsonDecoder.Decode(pipelinePack)
			c.Expect(err, gs.IsNil)
			v, ok := pipelinePack.Message.GetFieldValue("foo.bar")
			c.Expect(ok, gs.IsTrue)
			c.Expect(v, gs.Equals, float64(1))
			v, ok = pipelinePack.Message.GetFieldValue("foo.widget.name")
			c.Expect(ok, gs.IsTrue)
			c.Expect(v, gs.Equals, "w1")
			v, ok = pipelinePack.Message.GetFieldValue("foo.widget.price")
			c.Expect(ok, gs.IsTrue)
			c.Expect(v, gs.Equals, 10.1)
			v, ok = pipelinePack.Message.GetFieldValue("foo.widget.in_stock")
			c.Expect(ok, gs.IsTrue)
			c.Expect(v, gs.IsTrue)
			f := pipelinePack.Message.FindFirstField("foo.widget.locations")
			c.Expect(f, gs.Not(gs.IsNil))
			c.Expect(len(f.ValueString), gs.Equals, 1)
			c.Expect(f.ValueString[0], gs.Equals, "sfo")
		})

		c.Specify("returns an array of objects", func() {
			Json := `{"uuid":"2ae75e3a-7a70-4686-a2ea-ac7a02db9542","type":"TEST","timestamp":"2013-01-23T08:00:47.797575607-08:00","logger":"GoSpec","severity":6,"payload":"Test Payload","fields":{"objects":[{"name":"one"},{"name":"two"}]},"env_version":"0.8","metlog_pid":10569,"metlog_hostname":"trink-x230"}`
			pipelinePack.MsgBytes = []byte(Json)
			err := jsonDecoder.Decode(pipelinePack)
			c.Expect(err, gs.IsNil)
			v, ok := pipelinePack.Message.GetFieldValue("objects.0.name")
			c.Expect(ok, gs.IsTrue)
			c.Expect(v, gs.Equals, "one")
			v, ok = pipelinePack.Message.GetFieldValue("objects.1.name")
			c.Expect(ok, gs.IsTrue)
			c.Expect(v, gs.Equals, "two")
		})
	})

	c.Specify("A ProtobufDecoder", func() {
		msg := getTestMessage()
		encoded, err := proto.Marshal(msg)
		c.Assume(err, gs.IsNil)
		pack := getTestPipelinePack()
		decoder := new(ProtobufDecoder)

		c.Specify("decodes a msgpack message", func() {
			pack.MsgBytes = encoded
			err := decoder.Decode(pack)
			c.Expect(err, gs.IsNil)
			c.Expect(pack.Message, gs.Equals, msg)
			v, ok := pack.Message.GetFieldValue("foo")
			c.Expect(ok, gs.IsTrue)
			c.Expect(v, gs.Equals, "bar")
		})

		c.Specify("returns an error for bunk encoding", func() {
			bunk := append([]byte{0, 0, 0}, encoded...)
			pack.MsgBytes = bunk
			err := decoder.Decode(pack)
			c.Expect(err, gs.Not(gs.IsNil))
		})
	})

	c.Specify("Recovers from a panic in `Decode()`", func() {
		decoder := new(PanicDecoder)
		dRunner := NewDecoderRunner("panic", decoder)
		pack := getTestPipelinePack()
		dRunner.Start()
		dRunner.InChan() <- pack // No panic ==> success
	})
}

func BenchmarkDecodeJSON(b *testing.B) {
	b.StopTimer()
	msg := getTestMessage()
	var fmtString = `{"uuid":"%s","type":"%s","timestamp":%s,"logger":"%s","severity":%d,"payload":"%s","fields":%s,"env_version":"%s","metlog_pid":%d,"metlog_hostname":"%s"}`
	timestampJson, _ := json.Marshal(time.Unix(*msg.Timestamp/1e9, *msg.Timestamp%1e9))
	fieldsJson := `{"foo":"bar"}`
	uuid := msg.GetUuidString()
	jsonString := fmt.Sprintf(fmtString, uuid, *msg.Type,
		timestampJson, *msg.Logger, *msg.Severity, *msg.Payload,
		fieldsJson, *msg.EnvVersion, *msg.Pid, *msg.Hostname)

	pipelinePack := getTestPipelinePack()
	pipelinePack.MsgBytes = []byte(jsonString)
	jsonDecoder := new(JsonDecoder)
	b.StartTimer()
	for i := 0; i < b.N; i++ {
		jsonDecoder.Decode(pipelinePack)
	}
}
func BenchmarkEncodeProtobuf(b *testing.B) {
	b.StopTimer()
	msg := getTestMessage()
	b.StartTimer()
	for i := 0; i < b.N; i++ {
		proto.Marshal(msg)
	}
}

func BenchmarkDecodeProtobuf(b *testing.B) {
	b.StopTimer()
	msg := getTestMessage()
	encoded, _ := proto.Marshal(msg)
	pack := getTestPipelinePack()
	decoder := new(ProtobufDecoder)
	pack.MsgBytes = encoded
	b.StartTimer()
	for i := 0; i < b.N; i++ {
		decoder.Decode(pack)
	}
}
