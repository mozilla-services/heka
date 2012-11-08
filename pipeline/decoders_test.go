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
#
# ***** END LICENSE BLOCK *****/
package pipeline

import (
	"encoding/json"
	"fmt"
	"github.com/orfjackal/gospec/src/gospec"
	gs "github.com/orfjackal/gospec/src/gospec"
	"github.com/ugorji/go-msgpack"
	hekatime "heka/time"
)

func DecodersSpec(c gospec.Context) {

	msg := getTestMessage()

	c.Specify("A JsonDecoder", func() {
		var fmtString = `{"type":"%s","timestamp":%s,"logger":"%s","severity":%d,"payload":"%s","fields":%s,"env_version":"%s","metlog_pid":%d,"metlog_hostname":"%s"}`
		utctime := msg.Timestamp
		timestampJson, err := json.Marshal(utctime.Format(hekatime.TimeFormat))
		fieldsJson, err := json.Marshal(msg.Fields)
		c.Assume(err, gs.IsNil)
		jsonString := fmt.Sprintf(fmtString, msg.Type,
			timestampJson, msg.Logger, msg.Severity, msg.Payload,
			fieldsJson, msg.Env_version, msg.Pid, msg.Hostname)

		pipelinePack := getTestPipelinePack()
		pipelinePack.MsgBytes = []byte(jsonString)
		jsonDecoder := new(JsonDecoder)

		c.Specify("can decode a JSON message", func() {
			err := jsonDecoder.Decode(pipelinePack)
			c.Expect(pipelinePack.Message, gs.Equals, msg)
			c.Expect(err, gs.IsNil)
		})

		c.Specify("returns `fields` as a map", func() {
			jsonDecoder.Decode(pipelinePack)
			c.Expect(pipelinePack.Message.Fields["foo"], gs.Equals, "bar")
		})

		c.Specify("returns an error for bogus JSON", func() {
			badJson := fmt.Sprint("{{", jsonString)
			pipelinePack.MsgBytes = []byte(badJson)
			err := jsonDecoder.Decode(pipelinePack)
			c.Expect(err, gs.Not(gs.IsNil))
			c.Expect(pipelinePack.Decoded, gs.IsFalse)
			c.Expect(pipelinePack.Message.Timestamp.IsZero(), gs.IsTrue)
		})
	})

	c.Specify("A MsgPackDecoder", func() {
		msg := getTestMessage()
		encoded, err := msgpack.Marshal(msg)
		c.Assume(err, gs.IsNil)

		decoder := new(MsgPackDecoder)
		decoder.Init(nil)
		pack := getTestPipelinePack()

		c.Specify("decodes a msgpack message", func() {
			pack.MsgBytes = encoded
			err := decoder.Decode(pack)
			c.Expect(err, gs.IsNil)
			c.Expect(pack.Message, gs.Equals, msg)
			c.Expect(pack.Decoded, gs.IsTrue)
		})

		c.Specify("returns an error for bunk encoding", func() {
			bunk := append([]byte{0, 0, 0}, encoded...)
			pack.MsgBytes = bunk
			err := decoder.Decode(pack)
			c.Expect(err, gs.Not(gs.IsNil))
			c.Expect(pack.Decoded, gs.IsFalse)
		})
	})
}
