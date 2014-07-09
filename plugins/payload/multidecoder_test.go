/***** BEGIN LICENSE BLOCK *****
# This Source Code Form is subject to the terms of the Mozilla Public
# License, v. 2.0. If a copy of the MPL was not distributed with this file,
# You can obtain one at http://mozilla.org/MPL/2.0/.
#
# The Initial Developer of the Original Code is the Mozilla Foundation.
# Portions created by the Initial Developer are Copyright (C) 2012-2014
# the Initial Developer. All Rights Reserved.
#
# Contributor(s):
#   Rob Miller (rmiller@mozilla.com)
#   Mike Trinkala (trink@mozilla.com)
#   Victor Ng (vng@mozilla.com)
#
# ***** END LICENSE BLOCK *****/

// These tests would ideally be in the pipeline package, but for historical
// reasons they're using a PayloadRegexDecoder which is defined in the payload
// package, so we put them here to avoid circular imports.

package payload

import (
	"code.google.com/p/gomock/gomock"
	"code.google.com/p/goprotobuf/proto"
	"errors"
	"fmt"
	"github.com/mozilla-services/heka/message"
	. "github.com/mozilla-services/heka/pipeline"
	pipeline_ts "github.com/mozilla-services/heka/pipeline/testsupport"
	"github.com/mozilla-services/heka/pipelinemock"
	"github.com/rafrombrc/gospec/src/gospec"
	gs "github.com/rafrombrc/gospec/src/gospec"
	"regexp"
	"testing"
)

type MultiOutputDecoder struct {
	recycleChan chan *PipelinePack
}

func (m *MultiOutputDecoder) Decode(pack *PipelinePack) (packs []*PipelinePack, err error) {
	if _, ok := pack.Message.GetFieldValue("skipit"); ok {
		return nil, errors.New("Skipped")
	}
	pack2 := NewPipelinePack(m.recycleChan)
	pack3 := NewPipelinePack(m.recycleChan)
	pack2.Message = new(message.Message)
	pack3.Message = new(message.Message)
	pack.Message.Copy(pack2.Message)
	pack.Message.Copy(pack3.Message)
	message.NewStringField(pack2.Message, "skipit", "skipit")
	packs = append(packs, pack, pack2, pack3)
	return
}

func MultiDecoderSpec(c gospec.Context) {
	t := &pipeline_ts.SimpleT{}
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	pConfig := NewPipelineConfig(nil) // initializes Globals()

	c.Specify("A MultiDecoder", func() {
		decoder := new(MultiDecoder)
		decoder.SetName("MyMultiDecoder")
		conf := decoder.ConfigStruct().(*MultiDecoderConfig)

		supply := make(chan *PipelinePack, 1)
		pack := NewPipelinePack(supply)

		rDecoder0 := new(PayloadRegexDecoder)
		rDecoder0.Match, _ = regexp.Compile("^(?P<TheData>m.*)")
		rDecoder0.MessageFields = MessageTemplate{
			"StartsWithM": "%TheData%",
		}
		rDecoder0.logErrors = true
		wrapper0 := NewPluginWrapper("StartsWithM")
		wrapper0.CreateWithError = func() (interface{}, error) {
			return rDecoder0, nil
		}
		pConfig.DecoderWrappers["StartsWithM"] = wrapper0

		conf.Subs = []string{"StartsWithM"}
		errMsg := "All subdecoders failed."

		dRunner := pipelinemock.NewMockDecoderRunner(ctrl)
		// An error will be spit out b/c there's no real *dRunner in there;
		// doesn't impact the tests.
		dRunner.EXPECT().LogError(gomock.Any())

		c.Specify("decodes simple messages", func() {
			err := decoder.Init(conf)
			c.Assume(err, gs.IsNil)

			decoder.SetDecoderRunner(dRunner)
			regex_data := "matching text"
			pack.Message.SetPayload(regex_data)
			_, err = decoder.Decode(pack)
			c.Assume(err, gs.IsNil)

			value, ok := pack.Message.GetFieldValue("StartsWithM")
			c.Assume(ok, gs.IsTrue)
			c.Expect(value, gs.Equals, regex_data)
		})

		c.Specify("returns an error if all decoders fail", func() {
			err := decoder.Init(conf)
			c.Assume(err, gs.IsNil)

			decoder.SetDecoderRunner(dRunner)
			regex_data := "non-matching text"
			pack.Message.SetPayload(regex_data)
			packs, err := decoder.Decode(pack)
			c.Expect(len(packs), gs.Equals, 0)
			c.Expect(err.Error(), gs.Equals, errMsg)
		})

		c.Specify("logs subdecoder failures when configured to do so", func() {
			conf.LogSubErrors = true
			err := decoder.Init(conf)
			c.Assume(err, gs.IsNil)

			decoder.SetDecoderRunner(dRunner)
			regex_data := "non-matching text"
			pack.Message.SetPayload(regex_data)

			// Expect that we log an error for undecoded message.
			dRunner.EXPECT().LogError(fmt.Errorf(
				"Subdecoder 'StartsWithM' decode error: No match: %s", regex_data))

			packs, err := decoder.Decode(pack)
			c.Expect(len(packs), gs.Equals, 0)
			c.Expect(err.Error(), gs.Equals, errMsg)
		})

		c.Specify("sets subdecoder runner correctly", func() {
			err := decoder.Init(conf)
			c.Assume(err, gs.IsNil)

			// Call LogError to appease the angry gomock gods.
			dRunner.LogError(errors.New("foo"))

			// Now create a real *dRunner, pass it in, make sure a wrapper
			// gets handed to the subdecoder.
			dr := NewDecoderRunner(decoder.Name, decoder, new(PluginGlobals))
			decoder.SetDecoderRunner(dr)
			sub := decoder.Decoders[0]
			subRunner := sub.(*PayloadRegexDecoder).dRunner
			c.Expect(subRunner.Name(), gs.Equals,
				fmt.Sprintf("%s-StartsWithM", decoder.Name))
			c.Expect(subRunner.Decoder(), gs.Equals, sub)
		})

		c.Specify("with multiple registered decoders", func() {
			rDecoder1 := new(PayloadRegexDecoder)
			rDecoder1.Match, _ = regexp.Compile("^(?P<TheData>s.*)")
			rDecoder1.MessageFields = MessageTemplate{
				"StartsWithS": "%TheData%",
			}
			wrapper1 := NewPluginWrapper("StartsWithS")
			wrapper1.CreateWithError = func() (interface{}, error) {
				return rDecoder1, nil
			}
			pConfig.DecoderWrappers["StartsWithS"] = wrapper1

			rDecoder2 := new(PayloadRegexDecoder)
			rDecoder2.Match, _ = regexp.Compile("^(?P<TheData>m.*)")
			rDecoder2.MessageFields = MessageTemplate{
				"StartsWithM2": "%TheData%",
			}
			wrapper2 := NewPluginWrapper("StartsWithM2")
			wrapper2.CreateWithError = func() (interface{}, error) {
				return rDecoder2, nil
			}
			pConfig.DecoderWrappers["StartsWithM2"] = wrapper2

			conf.Subs = append(conf.Subs, "StartsWithS", "StartsWithM2")

			var ok bool

			// Two more subdecoders means two more LogError calls.
			dRunner.EXPECT().LogError(gomock.Any()).Times(2)

			c.Specify("defaults to `first-wins` cascading", func() {
				err := decoder.Init(conf)
				c.Assume(err, gs.IsNil)
				decoder.SetDecoderRunner(dRunner)

				c.Specify("on a first match condition", func() {
					pack.Message.SetPayload("match first")
					_, err = decoder.Decode(pack)
					c.Expect(err, gs.IsNil)
					_, ok = pack.Message.GetFieldValue("StartsWithM")
					c.Expect(ok, gs.IsTrue)
					_, ok = pack.Message.GetFieldValue("StartsWithS")
					c.Expect(ok, gs.IsFalse)
					_, ok = pack.Message.GetFieldValue("StartsWithM2")
					c.Expect(ok, gs.IsFalse)
				})

				c.Specify("and a second match condition", func() {
					pack.Message.SetPayload("second match")
					_, err = decoder.Decode(pack)
					c.Expect(err, gs.IsNil)
					_, ok = pack.Message.GetFieldValue("StartsWithM")
					c.Expect(ok, gs.IsFalse)
					_, ok = pack.Message.GetFieldValue("StartsWithS")
					c.Expect(ok, gs.IsTrue)
					_, ok = pack.Message.GetFieldValue("StartsWithM2")
					c.Expect(ok, gs.IsFalse)
				})

				c.Specify("returning an error if they all fail", func() {
					pack.Message.SetPayload("won't match")
					packs, err := decoder.Decode(pack)
					c.Expect(len(packs), gs.Equals, 0)
					c.Expect(err.Error(), gs.Equals, errMsg)
					_, ok = pack.Message.GetFieldValue("StartsWithM")
					c.Expect(ok, gs.IsFalse)
					_, ok = pack.Message.GetFieldValue("StartsWithS")
					c.Expect(ok, gs.IsFalse)
					_, ok = pack.Message.GetFieldValue("StartsWithM2")
					c.Expect(ok, gs.IsFalse)
				})
			})

			c.Specify("and using `all` cascading", func() {
				conf.CascadeStrategy = "all"
				err := decoder.Init(conf)
				c.Assume(err, gs.IsNil)
				decoder.SetDecoderRunner(dRunner)

				c.Specify("matches multiples when appropriate", func() {
					pack.Message.SetPayload("matches twice")
					_, err = decoder.Decode(pack)
					c.Expect(err, gs.IsNil)
					_, ok = pack.Message.GetFieldValue("StartsWithM")
					c.Expect(ok, gs.IsTrue)
					_, ok = pack.Message.GetFieldValue("StartsWithS")
					c.Expect(ok, gs.IsFalse)
					_, ok = pack.Message.GetFieldValue("StartsWithM2")
					c.Expect(ok, gs.IsTrue)
				})

				c.Specify("matches singles when appropriate", func() {
					pack.Message.SetPayload("second match")
					_, err = decoder.Decode(pack)
					c.Expect(err, gs.IsNil)
					_, ok = pack.Message.GetFieldValue("StartsWithM")
					c.Expect(ok, gs.IsFalse)
					_, ok = pack.Message.GetFieldValue("StartsWithS")
					c.Expect(ok, gs.IsTrue)
					_, ok = pack.Message.GetFieldValue("StartsWithM2")
					c.Expect(ok, gs.IsFalse)
				})

				c.Specify("returns an error if they all fail", func() {
					pack.Message.SetPayload("won't match")
					packs, err := decoder.Decode(pack)
					c.Expect(len(packs), gs.Equals, 0)
					c.Expect(err.Error(), gs.Equals, errMsg)
					_, ok = pack.Message.GetFieldValue("StartsWithM")
					c.Expect(ok, gs.IsFalse)
					_, ok = pack.Message.GetFieldValue("StartsWithS")
					c.Expect(ok, gs.IsFalse)
					_, ok = pack.Message.GetFieldValue("StartsWithM2")
					c.Expect(ok, gs.IsFalse)
				})
			})
		})
	})

	c.Specify("A MultiDecoder w/ MultiOutput", func() {
		decoder := new(MultiDecoder)
		decoder.SetName("MyMultiDecoder")
		conf := decoder.ConfigStruct().(*MultiDecoderConfig)
		conf.CascadeStrategy = "all"

		supply := make(chan *PipelinePack, 10)
		pack := NewPipelinePack(supply)

		sub0 := &MultiOutputDecoder{supply}
		sub1 := &MultiOutputDecoder{supply}
		sub2 := &MultiOutputDecoder{supply}

		wrapper0 := NewPluginWrapper("sub0")
		wrapper0.CreateWithError = func() (interface{}, error) {
			return sub0, nil
		}
		pConfig.DecoderWrappers["sub0"] = wrapper0

		wrapper1 := NewPluginWrapper("sub1")
		wrapper1.CreateWithError = func() (interface{}, error) {
			return sub1, nil
		}
		pConfig.DecoderWrappers["sub1"] = wrapper1

		wrapper2 := NewPluginWrapper("sub2")
		wrapper2.CreateWithError = func() (interface{}, error) {
			return sub2, nil
		}
		pConfig.DecoderWrappers["sub2"] = wrapper2

		conf.Subs = []string{"sub0", "sub1", "sub2"}
		dRunner := pipelinemock.NewMockDecoderRunner(ctrl)

		c.Specify("always tries to decode all packs", func() {
			err := decoder.Init(conf)
			c.Assume(err, gs.IsNil)
			decoder.SetDecoderRunner(dRunner)

			packs, err := decoder.Decode(pack)
			c.Expect(err, gs.IsNil)
			c.Expect(len(packs), gs.Equals, 15)
		})
	})
}

func BenchmarkMultiDecodeProtobuf(b *testing.B) {
	b.StopTimer()
	pConfig := NewPipelineConfig(nil) // initializes Globals()
	msg := pipeline_ts.GetTestMessage()
	msg.SetPayload("This is a test")
	pack := NewPipelinePack(pConfig.InputRecycleChan())
	pack.MsgBytes, _ = proto.Marshal(msg)
	decoder := new(MultiDecoder)
	conf := decoder.ConfigStruct().(*MultiDecoderConfig)
	sub := new(ProtobufDecoder)
	sub.Init(nil)
	wrapper0 := NewPluginWrapper("sub")
	wrapper0.CreateWithError = func() (interface{}, error) {
		return sub, nil
	}
	pConfig.DecoderWrappers["sub"] = wrapper0
	conf.CascadeStrategy = "first-wins"
	conf.Subs = []string{"sub"}
	decoder.Init(conf)

	b.StartTimer()
	for i := 0; i < b.N; i++ {
		decoder.Decode(pack)
	}
}
