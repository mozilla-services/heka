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
	gs "github.com/rafrombrc/gospec/src/gospec"
	"github.com/rafrombrc/gospec/src/gospec"
	"sync"
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

// Attach an `Init` method to MockDecoders so they'll work w/ PluginWrappers
func (d *MockDecoder) Init(config interface{}) (err error) {
	return
}

func DecodersSpec(c gospec.Context) {
	msg := getTestMessage()
	config := NewPipelineConfig(nil)

	c.Specify("A JsonDecoder", func() {
		encoded, err := json.Marshal(msg)
		c.Assume(err, gs.IsNil)
		pack := NewPipelinePack(config.RecycleChan)
		decoder := new(JsonDecoder)

		c.Specify("decodes a json message", func() {
			pack.MsgBytes = encoded
			err := decoder.Decode(pack)
			c.Expect(err, gs.IsNil)
			c.Expect(pack.Message, gs.Equals, msg)
			v, ok := pack.Message.GetFieldValue("foo")
			c.Expect(ok, gs.IsTrue)
			c.Expect(v, gs.Equals, "bar")
		})

		c.Specify("returns an error for bunk encoding", func() {
			bunk := append([]byte{'}'}, encoded...)
			pack.MsgBytes = bunk
			err := decoder.Decode(pack)
			c.Expect(err, gs.Not(gs.IsNil))
		})
	})

	c.Specify("A ProtobufDecoder", func() {
		encoded, err := proto.Marshal(msg)
		c.Assume(err, gs.IsNil)
		pack := NewPipelinePack(config.RecycleChan)
		decoder := new(ProtobufDecoder)

		c.Specify("decodes a protobuf message", func() {
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
		pack := NewPipelinePack(config.RecycleChan)
		var wg sync.WaitGroup
		wg.Add(1)
		Globals().Stopping = true
		dRunner.Start(config, &wg)
		dRunner.InChan() <- pack // No panic ==> success
		wg.Wait()
		Globals().Stopping = false
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

	config := NewPipelineConfig(nil)
	pipelinePack := NewPipelinePack(config.RecycleChan)
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
	config := NewPipelineConfig(nil)
	pack := NewPipelinePack(config.RecycleChan)
	decoder := new(ProtobufDecoder)
	pack.MsgBytes = encoded
	b.StartTimer()
	for i := 0; i < b.N; i++ {
		decoder.Decode(pack)
	}
}
