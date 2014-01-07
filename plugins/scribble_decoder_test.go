/***** BEGIN LICENSE BLOCK *****
# This Source Code Form is subject to the terms of the Mozilla Public
# License, v. 2.0. If a copy of the MPL was not distributed with this file,
# You can obtain one at http://mozilla.org/MPL/2.0/.
#
# The Initial Developer of the Original Code is the Mozilla Foundation.
# Portions created by the Initial Developer are Copyright (C) 2014
# the Initial Developer. All Rights Reserved.
#
# Contributor(s):
#   Rob Miller (rmiller@mozilla.com)
#
# ***** END LICENSE BLOCK *****/

package plugins

import (
	"code.google.com/p/gomock/gomock"
	. "github.com/mozilla-services/heka/pipeline"
	pipeline_ts "github.com/mozilla-services/heka/pipeline/testsupport"
	gs "github.com/rafrombrc/gospec/src/gospec"
)

func ScribbleDecoderSpec(c gs.Context) {
	t := &pipeline_ts.SimpleT{}
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	c.Specify("A ScribbleDecoder", func() {
		decoder := new(ScribbleDecoder)
		config := decoder.ConfigStruct().(*ScribbleDecoderConfig)
		myType := "myType"
		myPayload := "myPayload"
		config.MessageFields = MessageTemplate{"Type": myType, "Payload": myPayload}
		supply := make(chan *PipelinePack, 1)
		pack := NewPipelinePack(supply)

		c.Specify("sets basic values correctly", func() {
			decoder.Init(config)
			packs, err := decoder.Decode(pack)
			c.Expect(err, gs.IsNil)
			c.Expect(len(packs), gs.Equals, 1)
			c.Expect(pack.Message.GetType(), gs.Equals, myType)
			c.Expect(pack.Message.GetPayload(), gs.Equals, myPayload)
		})

		c.Specify("sets timestamp w/ default format", func() {
			config.MessageFields["Timestamp"] = "2008-09-08T22:47:31-07:00"
			decoder.Init(config)
			packs, err := decoder.Decode(pack)
			c.Expect(err, gs.IsNil)
			c.Expect(len(packs), gs.Equals, 1)
			c.Expect(pack.Message.GetTimestamp(), gs.Equals, int64(1220939251000000000))
		})

		c.Specify("sets timestamp w/ specified format", func() {
			config.MessageFields["Timestamp"] = "09/08 10:47:31PM '08 -0700"
			config.TimestampLayout = "01/02 03:04:05PM '06 -0700"
			decoder.Init(config)
			packs, err := decoder.Decode(pack)
			c.Expect(err, gs.IsNil)
			c.Expect(len(packs), gs.Equals, 1)
			c.Expect(pack.Message.GetTimestamp(), gs.Equals, int64(1220939251000000000))
		})

		c.Specify("sets UUID from bytes string", func() {
			byteStr := "1234567812345678"
			config.MessageFields["Uuid"] = byteStr
			decoder.Init(config)
			packs, err := decoder.Decode(pack)
			c.Expect(err, gs.IsNil)
			c.Expect(len(packs), gs.Equals, 1)
			c.Expect(string(pack.Message.GetUuid()), gs.Equals, byteStr)
		})

		c.Specify("sets UUID from UUID string", func() {
			byteStr := "1234567812345678"
			config.MessageFields["Uuid"] = "31323334-3536-3738-3132-333435363738"
			decoder.Init(config)
			packs, err := decoder.Decode(pack)
			c.Expect(err, gs.IsNil)
			c.Expect(len(packs), gs.Equals, 1)
			c.Expect(string(pack.Message.GetUuid()), gs.Equals, byteStr)
		})
	})
}
