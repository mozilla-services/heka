/***** BEGIN LICENSE BLOCK *****
# This Source Code Form is subject to the terms of the Mozilla Public
# License, v. 2.0. If a copy of the MPL was not distributed with this file,
# You can obtain one at http://mozilla.org/MPL/2.0/.
#
# The Initial Developer of the Original Code is the Mozilla Foundation.
# Portions created by the Initial Developer are Copyright (C) 2013
# the Initial Developer. All Rights Reserved.
#
# Contributor(s):
#   Tanguy Leroux (tlrx.dev@gmail.com)
#
# ***** END LICENSE BLOCK *****/

package elasticsearch

import (
	"bytes"
	"code.google.com/p/go-uuid/uuid"
	"encoding/json"
	"fmt"
	"github.com/mozilla-services/heka/message"
	. "github.com/mozilla-services/heka/pipeline"
	gs "github.com/rafrombrc/gospec/src/gospec"
	"strings"
	"testing"
	"time"
)

func TestAllSpecs(t *testing.T) {
	r := gs.NewRunner()
	r.Parallel = false

	r.AddSpec(ESEncodersSpec)

	gs.MainGoTest(r, t)
}

func getTestMessageWithFunnyFields() *message.Message {
	field, _ := message.NewField(`"foo`, "bar\n", "")
	field1, _ := message.NewField(`"number`, 64, "")
	field2, _ := message.NewField("\xa3", "\xa3", "")
	field3, _ := message.NewField("idField", "1234", "")

	msg := &message.Message{}
	msg.SetType("TEST")
	loc, _ := time.LoadLocation("UTC")
	t, _ := time.ParseInLocation("2006-01-02T15:04:05.000Z", "2013-07-16T15:49:05.070Z",
		loc)
	msg.SetTimestamp(t.UnixNano())
	msg.SetUuid(uuid.Parse("87cf1ac2-e810-4ddf-a02d-a5ce44d13a85"))
	msg.SetLogger("GoSpec")
	msg.SetSeverity(int32(6))
	msg.SetPayload("Test Payload")
	msg.SetEnvVersion("0.8")
	msg.SetPid(14098)
	msg.SetHostname("hostname")
	msg.AddField(field)
	msg.AddField(field1)
	msg.AddField(field2)
	msg.AddField(field3)

	return msg
}

func ESEncodersSpec(c gs.Context) {
	recycleChan := make(chan *PipelinePack, 1)
	pack := NewPipelinePack(recycleChan)
	pack.Message = getTestMessageWithFunnyFields()

	c.Specify("writeStringField", func() {
		buf := bytes.Buffer{}
		c.Specify("should properly encode special characters in json", func() {
			writeStringField(true, &buf, `hello"bar`, "world\nfoo\\")
			c.Expect(buf.String(), gs.Equals, `"hello\u0022bar":"world\u000afoo\u005c"`)
		})
		c.Specify("Should replace invalid utf8 with replacement character", func() {
			writeStringField(true, &buf, "\xa3", "\xa3")
			c.Expect(buf.String(), gs.Equals, "\"\xEF\xBF\xBD\":\"\xEF\xBF\xBD\"")
		})
	})

	c.Specify("interpolateFlag", func() {
		c.Specify("should interpolate for index and type names", func() {
			interpolatedIndex, err := interpolateFlag(&ElasticSearchCoordinates{},
				pack.Message, "heka-%{Pid}-%{\"foo}-%{2006.01.02}")
			c.Expect(err, gs.IsNil)
			t := time.Now().UTC()
			c.Expect(interpolatedIndex, gs.Equals, "heka-14098-bar\n-"+t.Format("2006.01.02"))

			interpolatedType, err := interpolateFlag(&ElasticSearchCoordinates{},
				pack.Message, "%{Type}")
			c.Expect(err, gs.IsNil)
			c.Expect(interpolatedType, gs.Equals, "TEST")
		})

		c.Specify("should interpolate from message field", func() {
			id := "%{idField}"
			interpolatedId, err := interpolateFlag(&ElasticSearchCoordinates{},
				pack.Message, id)
			c.Expect(err, gs.IsNil)
			c.Expect(interpolatedId, gs.Equals, "1234")
		})

		c.Specify("should fail for nonexistent message field", func() {
			id := "%{idFail}"
			unInterpolatedId, err := interpolateFlag(&ElasticSearchCoordinates{},
				pack.Message, id)

			c.Expect(strings.Contains(err.Error(),
				"Could not interpolate field from config: %{idFail}"), gs.IsTrue)
			c.Expect(unInterpolatedId, gs.Equals, "idFail")
		})
	})

	c.Specify("ESLogstashV0Encoder", func() {
		encoder := new(ESLogstashV0Encoder)
		config := encoder.ConfigStruct()

		c.Specify("Should properly encode a message", func() {
			err := encoder.Init(config)
			c.Expect(err, gs.IsNil)
			b, err := encoder.Encode(pack)
			c.Expect(err, gs.IsNil)

			output := string(b)
			lines := strings.Split(output, string(NEWLINE))

			decoded := make(map[string]interface{})
			err = json.Unmarshal([]byte(lines[0]), &decoded)
			c.Expect(err, gs.IsNil)
			sub := decoded["index"].(map[string]interface{})
			t := time.Now().UTC()
			c.Expect(sub["_index"], gs.Equals, "logstash-"+t.Format("2006.01.02"))
			c.Expect(sub["_type"], gs.Equals, "message")

			fmt.Println(lines[1])
			decoded = make(map[string]interface{})
			err = json.Unmarshal([]byte(lines[1]), &decoded)
			c.Expect(err, gs.IsNil)
			c.Expect(decoded["@fields"].(map[string]interface{})[`"foo`], gs.Equals, "bar\n")
			c.Expect(decoded["@fields"].(map[string]interface{})[`"number`], gs.Equals, 64.0)
			c.Expect(decoded["@fields"].(map[string]interface{})["\xEF\xBF\xBD"], gs.Equals,
				"\xEF\xBF\xBD")
			c.Expect(decoded["@uuid"], gs.Equals, "87cf1ac2-e810-4ddf-a02d-a5ce44d13a85")
			c.Expect(decoded["@timestamp"], gs.Equals, "2013-07-16T15:49:05.070Z")
			c.Expect(decoded["@type"], gs.Equals, "TEST")
			c.Expect(decoded["@logger"], gs.Equals, "GoSpec")
			c.Expect(decoded["@severity"], gs.Equals, 6.0)
			c.Expect(decoded["@message"], gs.Equals, "Test Payload")
			c.Expect(decoded["@envversion"], gs.Equals, "0.8")
			c.Expect(decoded["@pid"], gs.Equals, 14098.0)
			c.Expect(decoded["@source_host"], gs.Equals, "hostname")
		})
	})

	c.Specify("ESJsonEncoder", func() {
		encoder := new(ESJsonEncoder)
		config := encoder.ConfigStruct()

		c.Specify("Should properly encode a message", func() {
			err := encoder.Init(config)
			c.Expect(err, gs.IsNil)
			b, err := encoder.Encode(pack)
			c.Expect(err, gs.IsNil)

			output := string(b)
			lines := strings.Split(output, string(NEWLINE))

			decoded := make(map[string]interface{})
			err = json.Unmarshal([]byte(lines[0]), &decoded)
			c.Expect(err, gs.IsNil)
			sub := decoded["index"].(map[string]interface{})
			t := time.Now().UTC()
			c.Expect(sub["_index"], gs.Equals, "heka-"+t.Format("2006.01.02"))
			c.Expect(sub["_type"], gs.Equals, "message")

			fmt.Println(lines[1])
			decoded = make(map[string]interface{})
			err = json.Unmarshal([]byte(lines[1]), &decoded)
			c.Expect(err, gs.IsNil)
			c.Expect(decoded[`"foo`], gs.Equals, "bar\n")
			c.Expect(decoded[`"number`], gs.Equals, 64.0)
			c.Expect(decoded["\xEF\xBF\xBD"], gs.Equals, "\xEF\xBF\xBD")
			c.Expect(decoded["Uuid"], gs.Equals, "87cf1ac2-e810-4ddf-a02d-a5ce44d13a85")
			c.Expect(decoded["Timestamp"], gs.Equals, "2013-07-16T15:49:05.070Z")
			c.Expect(decoded["Type"], gs.Equals, "TEST")
			c.Expect(decoded["Logger"], gs.Equals, "GoSpec")
			c.Expect(decoded["Severity"], gs.Equals, 6.0)
			c.Expect(decoded["Payload"], gs.Equals, "Test Payload")
			c.Expect(decoded["EnvVersion"], gs.Equals, "0.8")
			c.Expect(decoded["Pid"], gs.Equals, 14098.0)
			c.Expect(decoded["Hostname"], gs.Equals, "hostname")
		})
	})
}
