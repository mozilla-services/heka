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
	ts "github.com/mozilla-services/heka/testsupport"
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

		c.Specify("returns an error for invalid field type", func() {
			badJson := `{"uuid":"2ae75e3a-7a70-4686-a2ea-ac7a02db9542","type":"TEST","timestamp":"2013-01-23T08:00:47.797575607-08:00","logger":"GoSpec","severity":6,"payload":"Test Payload","fields":[{"name":"foo","value_type":"BOGUS","value_format":"RAW","value_string":["bar"]}],"env_version":"0.8","metlog_pid":10569,"metlog_hostname":"trink-x230"}`
			pipelinePack.MsgBytes = []byte(badJson)
			err := jsonDecoder.Decode(pipelinePack)
			c.Expect(err.Error(), ts.StringContains, "invalid value type")
		})

		c.Specify("returns an error for invalid field format", func() {
			badJson := `{"uuid":"2ae75e3a-7a70-4686-a2ea-ac7a02db9542","type":"TEST","timestamp":"2013-01-23T08:00:47.797575607-08:00","logger":"GoSpec","severity":6,"payload":"Test Payload","fields":[{"name":"foo","value_type":"STRING","value_format":"BOGUS", "value_string":["bar"]}],"env_version":"0.8","metlog_pid":10569,"metlog_hostname":"trink-x230"}`
			pipelinePack.MsgBytes = []byte(badJson)
			err := jsonDecoder.Decode(pipelinePack)
			fmt.Println(err)
			c.Expect(err.Error(), ts.StringContains, "invalid value format")
		})

		c.Specify("returns an error for missing field value array", func() {
			badJson := `{"uuid":"2ae75e3a-7a70-4686-a2ea-ac7a02db9542","type":"TEST","timestamp":"2013-01-23T08:00:47.797575607-08:00","logger":"GoSpec","severity":6,"payload":"Test Payload","fields":[{"name":"foo","value_type":"STRING","value_format":"RAW"}],"env_version":"0.8","metlog_pid":10569,"metlog_hostname":"trink-x230"}`
			pipelinePack.MsgBytes = []byte(badJson)
			err := jsonDecoder.Decode(pipelinePack)
			c.Expect(err.Error(), ts.StringContains, "invalid value array")
		})

		c.Specify("returns an error for value array type mismatch", func() {
			badJson := `{"uuid":"2ae75e3a-7a70-4686-a2ea-ac7a02db9542","type":"TEST","timestamp":"2013-01-23T08:00:47.797575607-08:00","logger":"GoSpec","severity":6,"payload":"Test Payload","fields":[{"name":"foo","value_type":"STRING","value_format":"RAW", "value_string":[1]}],"env_version":"0.8","metlog_pid":10569,"metlog_hostname":"trink-x230"}`
			pipelinePack.MsgBytes = []byte(badJson)
			err := jsonDecoder.Decode(pipelinePack)
			fmt.Println(err)
			c.Expect(err.Error(), ts.StringContains, "The field contains: STRING; attempted to add DOUBLE")
		})
	})
}
