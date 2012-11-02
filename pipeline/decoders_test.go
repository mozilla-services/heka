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
)

func DecodersSpec(c gospec.Context) {

	msg := getTestMessage()

	c.Specify("A JsonDecoder", func() {
		var fmtString = `{"type":"%s","timestamp":%s,"logger":"%s","severity":%d,"payload":"%s","fields":%s,"env_version":"%s","metlog_pid":%d,"metlog_hostname":"%s"}`
		timestampJson, err := json.Marshal(msg.Timestamp)
		fieldsJson, err := json.Marshal(msg.Fields)
		c.Assume(err, gs.IsNil)
		jsonString := fmt.Sprintf(fmtString, msg.Type,
			timestampJson, msg.Logger, msg.Severity, msg.Payload,
			fieldsJson, msg.Env_version, msg.Pid, msg.Hostname)

		pipelinePack := getTestPipelinePack()
		pipelinePack.MsgBytes = []byte(jsonString)
		jsonDecoder := &JsonDecoder{}

		c.Specify("can decode a JSON message", func() {
			jsonDecoder.Decode(pipelinePack)
			c.Expect(pipelinePack.Message, gs.Equals, msg)
		})

		c.Specify("returns `fields` as a map", func() {
			jsonDecoder.Decode(pipelinePack)
			c.Expect(pipelinePack.Message.Fields["foo"], gs.Equals, "bar")
		})

		c.Specify("returns nil for bogus JSON", func() {
			badJson := fmt.Sprint("{{", jsonString)
			pipelinePack.MsgBytes = []byte(badJson)
			jsonDecoder.Decode(pipelinePack)
			c.Expect(pipelinePack.Decoded, gs.IsFalse)
			c.Expect(pipelinePack.Message.Timestamp.IsZero(), gs.IsTrue)
		})
	})
}
