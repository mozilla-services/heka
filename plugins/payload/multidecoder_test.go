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
	"errors"
	"fmt"
	"testing"

	"github.com/bbangert/toml"
	"github.com/gogo/protobuf/proto"
	"github.com/mozilla-services/heka/message"
	. "github.com/mozilla-services/heka/pipeline"
	pipeline_ts "github.com/mozilla-services/heka/pipeline/testsupport"
	"github.com/mozilla-services/heka/pipelinemock"
	"github.com/rafrombrc/gomock/gomock"
	"github.com/rafrombrc/gospec/src/gospec"
	gs "github.com/rafrombrc/gospec/src/gospec"
)

type MultiOutputDecoder struct {
	recycleChan chan *PipelinePack
}

func (m *MultiOutputDecoder) Init(config interface{}) error {
	return nil
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
		subsTOML := `[StartsWithM]
		type = "PayloadRegexDecoder"
		match_regex = '^(?P<TheData>m.*)'
		log_errors = true
		[StartsWithM.message_fields]
		StartsWithM = "%TheData%"

		[StartsWithS]
		type = "PayloadRegexDecoder"
		match_regex = '^(?P<TheData>s.*)'
		log_errors = true
		[StartsWithS.message_fields]
		StartsWithS = "%TheData%"

		[StartsWithM2]
		type = "PayloadRegexDecoder"
		match_regex = '^(?P<TheData>m.*)'
		log_errors = true
		[StartsWithM2.message_fields]
		StartsWithM2 = "%TheData%"
		`

		RegisterPlugin("PayloadRegexDecoder", func() interface{} {
			return &PayloadRegexDecoder{}
		})
		defer delete(AvailablePlugins, "PayloadRegexDecoder")

		var configFile ConfigFile
		_, err := toml.Decode(subsTOML, &configFile)
		c.Assume(err, gs.IsNil)

		decoder := new(MultiDecoder)
		decoder.SetName("MyMultiDecoder")
		decoder.SetPipelineConfig(pConfig)
		conf := decoder.ConfigStruct().(*MultiDecoderConfig)

		supply := make(chan *PipelinePack, 1)
		pack := NewPipelinePack(supply)

		mSection, ok := configFile["StartsWithM"]
		c.Assume(ok, gs.IsTrue)
		mMaker, err := NewPluginMaker("StartsWithM", pConfig, mSection)
		c.Assume(err, gs.IsNil)
		pConfig.DecoderMakers["StartsWithM"] = mMaker

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
				"Subdecoder 'StartsWithM' decode error: No match: %s", regex_data)).AnyTimes()

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
			dr := NewDecoderRunner(decoder.Name, decoder, 10)
			decoder.SetDecoderRunner(dr)
			sub := decoder.Decoders[0]
			subRunner := sub.(*PayloadRegexDecoder).dRunner
			c.Expect(subRunner.Name(), gs.Equals,
				fmt.Sprintf("%s-StartsWithM", decoder.Name))
			c.Expect(subRunner.Decoder(), gs.Equals, sub)
		})

		c.Specify("with multiple registered decoders", func() {
			sSection, ok := configFile["StartsWithS"]
			c.Assume(ok, gs.IsTrue)
			sMaker, err := NewPluginMaker("StartsWithS", pConfig, sSection)
			c.Assume(err, gs.IsNil)
			pConfig.DecoderMakers["StartsWithS"] = sMaker

			m2Section, ok := configFile["StartsWithM2"]
			c.Assume(ok, gs.IsTrue)
			m2Maker, err := NewPluginMaker("StartsWithM2", pConfig, m2Section)
			c.Assume(err, gs.IsNil)
			pConfig.DecoderMakers["StartsWithM2"] = m2Maker

			conf.Subs = append(conf.Subs, "StartsWithS", "StartsWithM2")

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
		subsTOML := `[sub0]
		type = "MultiOutputDecoder"

		[sub1]
		type = "MultiOutputDecoder"

		[sub2]
		type = "MultiOutputDecoder"
		`

		RegisterPlugin("MultiOutputDecoder", func() interface{} {
			return &MultiOutputDecoder{}
		})
		defer delete(AvailablePlugins, "MultiOutputDecoder")

		var configFile ConfigFile
		_, err := toml.Decode(subsTOML, &configFile)

		decoder := new(MultiDecoder)
		decoder.SetName("MyMultiDecoder")
		decoder.SetPipelineConfig(pConfig)
		conf := decoder.ConfigStruct().(*MultiDecoderConfig)
		conf.CascadeStrategy = "all"

		supply := make(chan *PipelinePack, 10)
		pack := NewPipelinePack(supply)

		sub0Section, ok := configFile["sub0"]
		c.Assume(ok, gs.IsTrue)
		sub0Maker, err := NewPluginMaker("sub0", pConfig, sub0Section)
		c.Assume(err, gs.IsNil)
		pConfig.DecoderMakers["sub0"] = sub0Maker

		sub1Section, ok := configFile["sub1"]
		c.Assume(ok, gs.IsTrue)
		sub1Maker, err := NewPluginMaker("sub1", pConfig, sub1Section)
		c.Assume(err, gs.IsNil)
		pConfig.DecoderMakers["sub1"] = sub1Maker

		sub2Section, ok := configFile["sub2"]
		c.Assume(ok, gs.IsTrue)
		sub2Maker, err := NewPluginMaker("sub2", pConfig, sub2Section)
		c.Assume(err, gs.IsNil)
		pConfig.DecoderMakers["sub2"] = sub2Maker

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
	pConfig := NewPipelineConfig(nil) // initializes Globals
	msg := pipeline_ts.GetTestMessage()
	msg.SetPayload("This is a test")
	pack := NewPipelinePack(pConfig.InputRecycleChan())
	pack.MsgBytes, _ = proto.Marshal(msg)
	decoder := new(MultiDecoder)
	decoder.SetPipelineConfig(pConfig)
	conf := decoder.ConfigStruct().(*MultiDecoderConfig)

	RegisterPlugin("ProtobufDecoder", func() interface{} {
		return &ProtobufDecoder{}
	})
	defer delete(AvailablePlugins, "ProtobufDecoder")
	var section PluginConfig
	_, err := toml.Decode("", &section)
	if err != nil {
		b.Fatalf("Error decoding empty TOML: %s", err.Error())
	}
	maker, err := NewPluginMaker("ProtobufDecoder", pConfig, section)
	if err != nil {
		b.Fatalf("Error decoding empty TOML: %s", err.Error())
	}
	pConfig.DecoderMakers["ProtobufDecoder"] = maker

	conf.CascadeStrategy = "first-wins"
	conf.Subs = []string{"sub"}
	decoder.Init(conf)

	b.StartTimer()
	for i := 0; i < b.N; i++ {
		decoder.Decode(pack)
	}
}
