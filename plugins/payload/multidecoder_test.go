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
#   Victor Ng (vng@mozilla.com)
#
# ***** END LICENSE BLOCK *****/

// These tests would ideally be in the pipeline package, but for historical
// reasons they're using a PayloadRegexDecoder which is defined in the payload
// package, so we put them here to avoid circular imports.

package payload

import (
	"code.google.com/p/gomock/gomock"
	"errors"
	"fmt"
	. "github.com/mozilla-services/heka/pipeline"
	pipeline_ts "github.com/mozilla-services/heka/pipeline/testsupport"
	"github.com/mozilla-services/heka/pipelinemock"
	"github.com/rafrombrc/gospec/src/gospec"
	gs "github.com/rafrombrc/gospec/src/gospec"
)

func MultiDecoderSpec(c gospec.Context) {
	t := &pipeline_ts.SimpleT{}
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	NewPipelineConfig(nil) // initializes Globals()

	c.Specify("A MultiDecoder", func() {
		decoder := new(MultiDecoder)
		decoder.SetName("MyMultiDecoder")
		conf := decoder.ConfigStruct().(*MultiDecoderConfig)

		supply := make(chan *PipelinePack, 1)
		pack := NewPipelinePack(supply)

		conf.Subs = make(map[string]interface{}, 0)

		conf.Subs["StartsWithM"] = make(map[string]interface{}, 0)
		withM := conf.Subs["StartsWithM"].(map[string]interface{})
		withM["type"] = "PayloadRegexDecoder"
		withM["match_regex"] = "^(?P<TheData>m.*)"
		withMFields := make(map[string]interface{}, 0)
		withMFields["StartsWithM"] = "%TheData%"
		withM["message_fields"] = withMFields

		conf.Order = []string{"StartsWithM"}

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

			c.Expect(pack.Message.GetType(), gs.Equals, "heka.MyMultiDecoder")
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
			sub := decoder.Decoders["StartsWithM"]
			subRunner := sub.(*PayloadRegexDecoder).dRunner
			c.Expect(subRunner.Name(), gs.Equals,
				fmt.Sprintf("%s-StartsWithM", decoder.Name))
			c.Expect(subRunner.Decoder(), gs.Equals, sub)
		})

		c.Specify("fails to init with no order set", func() {
			conf.Order = []string{}
			err := decoder.Init(conf)
			c.Expect(err.Error(), gs.Equals, "An order must be specified.")

			dRunner.LogError(errors.New("foo"))
		})

		c.Specify("with multiple registered decoders", func() {
			conf.Subs["StartsWithS"] = make(map[string]interface{}, 0)
			withS := conf.Subs["StartsWithS"].(map[string]interface{})
			withS["type"] = "PayloadRegexDecoder"
			withS["match_regex"] = "^(?P<TheData>s.*)"
			withSFields := make(map[string]interface{}, 0)
			withSFields["StartsWithS"] = "%TheData%"
			withS["message_fields"] = withSFields

			conf.Subs["StartsWithM2"] = make(map[string]interface{}, 0)
			withM2 := conf.Subs["StartsWithM2"].(map[string]interface{})
			withM2["type"] = "PayloadRegexDecoder"
			withM2["match_regex"] = "^(?P<TheData>m.*)"
			withM2Fields := make(map[string]interface{}, 0)
			withM2Fields["StartsWithM2"] = "%TheData%"
			withM2["message_fields"] = withM2Fields

			conf.Order = append(conf.Order, "StartsWithS", "StartsWithM2")

			var ok bool

			// Two more subdecoders means two more LogError calls.
			dRunner.EXPECT().LogError(gomock.Any()).Times(2)

			c.Specify("defaults to `first-wins` cascading", func() {
				err := decoder.Init(conf)
				c.Assume(err, gs.IsNil)
				decoder.SetDecoderRunner(dRunner)

				c.Specify("on a first match condition", func() {
					regexData := "match first"
					pack.Message.SetPayload(regexData)
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
					regexData := "second match"
					pack.Message.SetPayload(regexData)
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
					regexData := "won't match"
					pack.Message.SetPayload(regexData)
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
					regexData := "matches twice"
					pack.Message.SetPayload(regexData)
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
					regexData := "second match"
					pack.Message.SetPayload(regexData)
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
					regexData := "won't match"
					pack.Message.SetPayload(regexData)
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
}
