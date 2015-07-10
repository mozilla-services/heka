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

package pipeline

import (
	"testing"

	"github.com/gogo/protobuf/proto"
	ts "github.com/mozilla-services/heka/pipeline/testsupport"
	"github.com/rafrombrc/gomock/gomock"
	"github.com/rafrombrc/gospec/src/gospec"
	gs "github.com/rafrombrc/gospec/src/gospec"
)

// Attach an `Init` method to MockDecoders so they'll work w/ PluginWrappers
func (d *MockDecoder) Init(config interface{}) (err error) {
	return
}

func ProtobufDecoderSpec(c gospec.Context) {
	t := &ts.SimpleT{}
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	msg := ts.GetTestMessage()
	config := NewPipelineConfig(nil) // Initializes globals.

	c.Specify("A ProtobufDecoder", func() {
		encoded, err := proto.Marshal(msg)
		c.Assume(err, gs.IsNil)
		pack := NewPipelinePack(config.inputRecycleChan)
		decoder := new(ProtobufDecoder)
		decoder.sampleDenominator = 1000 // Since we don't call decoder.Init().

		c.Specify("decodes a protobuf message", func() {
			pack.MsgBytes = encoded
			_, err := decoder.Decode(pack)
			c.Expect(err, gs.IsNil)
			c.Expect(pack.Message, gs.Equals, msg)
			v, ok := pack.Message.GetFieldValue("foo")
			c.Expect(ok, gs.IsTrue)
			c.Expect(v, gs.Equals, "bar")
		})

		c.Specify("returns an error for bunk encoding", func() {
			bunk := append([]byte{0, 0, 0}, encoded...)
			pack.MsgBytes = bunk
			_, err := decoder.Decode(pack)
			c.Expect(err, gs.Not(gs.IsNil))
		})
	})
}

func BenchmarkEncodeProtobuf(b *testing.B) {
	b.StopTimer()
	msg := ts.GetTestMessage()
	b.StartTimer()
	for i := 0; i < b.N; i++ {
		proto.Marshal(msg)
	}
}

func BenchmarkDecodeProtobuf(b *testing.B) {
	b.StopTimer()
	msg := ts.GetTestMessage()
	msg.SetPayload("This is a test")
	encoded, _ := proto.Marshal(msg)
	config := NewPipelineConfig(nil)
	pack := NewPipelinePack(config.inputRecycleChan)
	decoder := new(ProtobufDecoder)
	decoder.SetPipelineConfig(config)
	decoder.Init(nil)
	pack.MsgBytes = encoded
	b.StartTimer()
	for i := 0; i < b.N; i++ {
		decoder.Decode(pack)
	}
}
