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
		dRunner := NewDecoderRunner("panic", decoder, nil)
		pack := NewPipelinePack(config.RecycleChan)
		var wg sync.WaitGroup
		wg.Add(1)
		Stopping = true
		dRunner.Start(config, &wg)
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
	config := NewPipelineConfig(1)
	pack := NewPipelinePack(config.RecycleChan)
	decoder := new(ProtobufDecoder)
	pack.MsgBytes = encoded
	b.StartTimer()
	for i := 0; i < b.N; i++ {
		decoder.Decode(pack)
	}
}
