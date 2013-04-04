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
	"code.google.com/p/gomock/gomock"
	"code.google.com/p/goprotobuf/proto"
	"encoding/json"
	"fmt"
	"github.com/mozilla-services/heka/message"
	ts "github.com/mozilla-services/heka/testsupport"
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

func DecoderMgrSpec(c gospec.Context) {
	t := &ts.SimpleT{}
	ctrl := gomock.NewController(t)
	origPoolSize := PoolSize
	origDecodersByEncoding := DecodersByEncoding
	origTopHeaderMessageEncoding := topHeaderMessageEncoding
	defer func() {
		PoolSize = origPoolSize
		DecodersByEncoding = origDecodersByEncoding
		topHeaderMessageEncoding = origTopHeaderMessageEncoding
		ctrl.Finish()
	}()

	fakeDecoderWrappers := func(count int) (wrappers map[string]*PluginWrapper) {
		wrappers = make(map[string]*PluginWrapper)
		var w *PluginWrapper
		for i := 0; i < count; i++ {
			w = &PluginWrapper{
				name: fmt.Sprintf("mock%d", i),
			}

			w.configCreator = func() interface{} {
				return nil
			}

			w.pluginCreator = func() interface{} {
				return NewMockDecoder(ctrl)
			}

			wrappers[w.name] = w
			DecodersByEncoding[message.Header_MessageEncoding(i)] = w.name
		}
		topHeaderMessageEncoding = message.Header_MessageEncoding(count - 1)
		return
	}

	c.Specify("decoderManager instances", func() {

		config := NewPipelineConfig(10)
		count := 5
		wrappers := fakeDecoderWrappers(count)
		config.DecoderWrappers = wrappers
		name := "test"
		dm := newDecoderManager(config, name)
		c.Assume(len(dm.decoders), gs.Equals, 0)
		defer config.decodersWg.Wait()

		c.Specify("add decoders to the registry when NewDecoders is called", func() {
			decoders := dm.NewDecoders()
			defer func() {
				for _, d := range decoders {
					close(d.InChan())
				}
			}()
			c.Expect(len(decoders), gs.Equals, count)
			c.Expect(len(dm.decoders), gs.Equals, count)
		})

		c.Specify("when NewDecodersByEncoding is called", func() {
			decoders := dm.NewDecodersByEncoding()
			defer func() {
				for _, d := range decoders {
					close(d.InChan())
				}
			}()
			c.Expect(len(decoders), gs.Equals, count)

			c.Specify("adds decoders to the registry", func() {
				for i := 0; i < count; i++ {
					dRunner := decoders[message.Header_MessageEncoding(i)]
					nameStarts := fmt.Sprintf("%s-%s%d", name, "mock", i)
					c.Expect(dRunner.Name()[:len(nameStarts)], gs.Equals, nameStarts)
				}
			})

			c.Specify("reuses stopped decoders", func() {
				for _, d := range decoders {
					close(d.InChan())
				}
				config.decodersWg.Wait()
				c.Expect(len(dm.stopped), gs.Equals, count)
				decoders2 := dm.NewDecodersByEncoding()
				c.Expect(len(decoders2), gs.Equals, count)
				c.Expect(len(dm.decoders), gs.Equals, count)
				c.Expect(len(dm.stopped), gs.Equals, 0)
				for _, d := range decoders {
					_, ok := dm.decoders[d.UUID()]
					c.Expect(ok, gs.IsTrue)
				}
			})
		})
	})
}

func DecodersSpec(c gospec.Context) {
	msg := getTestMessage()
	config := NewPipelineConfig(1)

	c.Specify("A JsonDecoder", func() {
		var fmtString = `{"uuid":"%s","type":"%s","timestamp":%s,"logger":"%s","severity":%d,"payload":"%s","fields":%s,"env_version":"%s","metlog_pid":%d,"metlog_hostname":"%s"}`
		timestampJson, err := json.Marshal(time.Unix(*msg.Timestamp/1e9, *msg.Timestamp%1e9))
		fieldsJson := `{"foo":"bar"}`
		c.Assume(err, gs.IsNil)
		uuid := msg.GetUuidString()
		jsonString := fmt.Sprintf(fmtString, uuid, *msg.Type,
			timestampJson, *msg.Logger, *msg.Severity, *msg.Payload,
			fieldsJson, *msg.EnvVersion, *msg.Pid, *msg.Hostname)

		pipelinePack := NewPipelinePack(config)
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
		pack := NewPipelinePack(config)
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
		dRunner := NewDecoderRunner("panic", decoder, nil)
		pack := NewPipelinePack(config)
		var wg sync.WaitGroup
		wg.Add(1)
		Stopping = true
		dRunner.Start(&wg)
		dRunner.InChan() <- pack // No panic ==> success
		wg.Wait()
		Stopping = false
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

	config := NewPipelineConfig(1)
	pipelinePack := NewPipelinePack(config)
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
	config := NewPipelineConfig(1)
	pack := NewPipelinePack(config)
	decoder := new(ProtobufDecoder)
	pack.MsgBytes = encoded
	b.StartTimer()
	for i := 0; i < b.N; i++ {
		decoder.Decode(pack)
	}
}
