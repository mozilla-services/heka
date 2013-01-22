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
	"github.com/mozilla-services/heka/message"
	"github.com/rafrombrc/gospec/src/gospec"
	gs "github.com/rafrombrc/gospec/src/gospec"
	"time"
)

func DecodersSpec(c gospec.Context) {
	msg := getTestMessage()

	c.Specify("A JsonDecoder", func() {
		var fmtString = `{"uuid":"%s","type":"%s","timestamp":%s,"logger":"%s","severity":%d,"payload":"%s","fields":%s,"env_version":"%s","metlog_pid":%d,"metlog_hostname":"%s"}`
		timestampJson, err := json.Marshal(time.Unix(*msg.Timestamp/1e9, *msg.Timestamp%1e9))
		fieldsJson, err := json.Marshal(msg.Fields)
		c.Assume(err, gs.IsNil)
		uuid := fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", msg.Uuid[:4], msg.Uuid[4:6], msg.Uuid[6:8], msg.Uuid[8:10], msg.Uuid[10:])
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
			c.Expect(*pipelinePack.Message.Timestamp == int64(0), gs.IsTrue)
		})
	})
}
