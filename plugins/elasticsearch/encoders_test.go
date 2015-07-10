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
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/mozilla-services/heka/message"
	. "github.com/mozilla-services/heka/pipeline"
	"github.com/pborman/uuid"
	gs "github.com/rafrombrc/gospec/src/gospec"
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
	field4 := message.NewFieldInit("test_raw_field_bytes_array", message.Field_BYTES, "")
	field4.AddValue([]byte("{\"asdf\":123}"))
	field4.AddValue([]byte("{\"jkl;\":123}"))
	field5 := message.NewFieldInit("byteArray", message.Field_BYTES, "")
	field5.AddValue([]byte("asdf"))
	field5.AddValue([]byte("jkl;"))
	field6 := message.NewFieldInit("integerArray", message.Field_INTEGER, "")
	field6.AddValue(22)
	field6.AddValue(80)
	field6.AddValue(3000)
	field7 := message.NewFieldInit("doubleArray", message.Field_DOUBLE, "")
	field7.AddValue(42.0)
	field7.AddValue(19101.3)
	field8 := message.NewFieldInit("boolArray", message.Field_BOOL, "")
	field8.AddValue(true)
	field8.AddValue(false)
	field8.AddValue(false)
	field8.AddValue(false)
	field8.AddValue(true)
	field9 := message.NewFieldInit("test_raw_field_string", message.Field_STRING, "")
	field9.AddValue("{\"asdf\":123}")
	field10 := message.NewFieldInit("test_raw_field_bytes", message.Field_BYTES, "")
	field10.AddValue([]byte("{\"asdf\":123}"))
	field11 := message.NewFieldInit("test_raw_field_string_array", message.Field_STRING, "")
	field11.AddValue("{\"asdf\":123}")
	field11.AddValue("{\"jkl;\":123}")
	field12 := message.NewFieldInit("stringArray", message.Field_STRING, "")
	field12.AddValue("asdf")
	field12.AddValue("jkl;")
	field12.AddValue("push")
	field12.AddValue("pull")

	msg := &message.Message{}
	msg.SetType("TEST")
	loc, _ := time.LoadLocation("UTC")
	t, _ := time.ParseInLocation("2006-01-02T15:04:05", "2013-07-16T15:49:05",
		loc)
	fmt.Printf("%s %v \n", t)
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
	msg.AddField(field4)
	msg.AddField(field5)
	msg.AddField(field6)
	msg.AddField(field7)
	msg.AddField(field8)
	msg.AddField(field9)
	msg.AddField(field10)
	msg.AddField(field11)
	msg.AddField(field12)

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
				pack.Message, "heka-%{Pid}-%{\"foo}-%{%Y.%m.%d}")
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
		config := encoder.ConfigStruct().(*ESLogstashV0EncoderConfig)
		config.RawBytesFields = []string{
			"test_raw_field_string",
			"test_raw_field_bytes",
			"test_raw_field_string_array",
			"test_raw_field_bytes_array",
		}

		c.Specify("Should properly encode a message", func() {
			err := encoder.Init(config)
			c.Assume(err, gs.IsNil)
			b, err := encoder.Encode(pack)
			c.Assume(err, gs.IsNil)

			output := string(b)
			lines := strings.Split(output, string(NEWLINE))

			decoded := make(map[string]interface{})
			err = json.Unmarshal([]byte(lines[0]), &decoded)
			c.Assume(err, gs.IsNil)
			sub := decoded["index"].(map[string]interface{})
			t := time.Now().UTC()
			c.Expect(sub["_index"], gs.Equals, "logstash-"+t.Format("2006.01.02"))
			c.Expect(sub["_type"], gs.Equals, "message")

			decoded = make(map[string]interface{})
			err = json.Unmarshal([]byte(lines[1]), &decoded)
			c.Assume(err, gs.IsNil)
			c.Expect(decoded["@fields"].(map[string]interface{})[`"foo`], gs.Equals, "bar\n")
			c.Expect(decoded["@fields"].(map[string]interface{})[`"number`], gs.Equals, 64.0)
			c.Expect(decoded["@fields"].(map[string]interface{})["\xEF\xBF\xBD"], gs.Equals,
				"\xEF\xBF\xBD")
			c.Expect(decoded["@uuid"], gs.Equals, "87cf1ac2-e810-4ddf-a02d-a5ce44d13a85")
			c.Expect(decoded["@timestamp"], gs.Equals, "2013-07-16T15:49:05")
			c.Expect(decoded["@type"], gs.Equals, "message")
			c.Expect(decoded["@logger"], gs.Equals, "GoSpec")
			c.Expect(decoded["@severity"], gs.Equals, 6.0)
			c.Expect(decoded["@message"], gs.Equals, "Test Payload")
			c.Expect(decoded["@envversion"], gs.Equals, "0.8")
			c.Expect(decoded["@pid"], gs.Equals, 14098.0)
			c.Expect(decoded["@source_host"], gs.Equals, "hostname")

			stringsArray := decoded["@fields"].(map[string]interface{})["stringArray"].([]interface{})
			c.Expect(len(stringsArray), gs.Equals, 4)
			c.Expect(stringsArray[0], gs.Equals, "asdf")
			c.Expect(stringsArray[1], gs.Equals, "jkl;")
			c.Expect(stringsArray[2], gs.Equals, "push")
			c.Expect(stringsArray[3], gs.Equals, "pull")

			bytesArray := decoded["@fields"].(map[string]interface{})["byteArray"].([]interface{})
			c.Expect(len(bytesArray), gs.Equals, 2)
			c.Expect(bytesArray[0], gs.Equals, base64.StdEncoding.EncodeToString([]byte("asdf")))
			c.Expect(bytesArray[1], gs.Equals, base64.StdEncoding.EncodeToString([]byte("jkl;")))

			integerArray := decoded["@fields"].(map[string]interface{})["integerArray"].([]interface{})
			c.Expect(len(integerArray), gs.Equals, 3)
			c.Expect(integerArray[0], gs.Equals, 22.0)
			c.Expect(integerArray[1], gs.Equals, 80.0)
			c.Expect(integerArray[2], gs.Equals, 3000.0)

			doubleArray := decoded["@fields"].(map[string]interface{})["doubleArray"].([]interface{})
			c.Expect(len(doubleArray), gs.Equals, 2)
			c.Expect(doubleArray[0], gs.Equals, 42.0)
			c.Expect(doubleArray[1], gs.Equals, 19101.3)

			boolArray := decoded["@fields"].(map[string]interface{})["boolArray"].([]interface{})
			c.Expect(len(boolArray), gs.Equals, 5)
			c.Expect(boolArray[0], gs.Equals, true)
			c.Expect(boolArray[1], gs.Equals, false)
			c.Expect(boolArray[2], gs.Equals, false)
			c.Expect(boolArray[3], gs.Equals, false)
			c.Expect(boolArray[4], gs.Equals, true)

			c.Expect(decoded["@fields"].(map[string]interface{})["test_raw_field_string"].(map[string]interface{})["asdf"], gs.Equals, 123.0)
			c.Expect(decoded["@fields"].(map[string]interface{})["test_raw_field_bytes"].(map[string]interface{})["asdf"], gs.Equals, 123.0)
			c.Expect(decoded["@fields"].(map[string]interface{})["test_raw_field_string_array"].([]interface{})[0].(map[string]interface{})["asdf"], gs.Equals, 123.0)
			c.Expect(decoded["@fields"].(map[string]interface{})["test_raw_field_string_array"].([]interface{})[1].(map[string]interface{})["jkl;"], gs.Equals, 123.0)
			c.Expect(decoded["@fields"].(map[string]interface{})["test_raw_field_bytes_array"].([]interface{})[0].(map[string]interface{})["asdf"], gs.Equals, 123.0)
			c.Expect(decoded["@fields"].(map[string]interface{})["test_raw_field_bytes_array"].([]interface{})[1].(map[string]interface{})["jkl;"], gs.Equals, 123.0)
		})

		c.Specify("encodes w/ a different timestamp format", func() {
			config.Timestamp = "%Y/%m/%d %H:%M:%S %z"
			err := encoder.Init(config)
			c.Expect(err, gs.IsNil)
			b, err := encoder.Encode(pack)
			c.Expect(err, gs.IsNil)

			output := string(b)
			lines := strings.Split(output, string(NEWLINE))
			decoded := make(map[string]interface{})
			err = json.Unmarshal([]byte(lines[1]), &decoded)
			c.Assume(err, gs.IsNil)

			c.Expect(decoded["@timestamp"], gs.Equals, "2013/07/16 15:49:05 +0000")
		})

		c.Specify("validates dynamic_fields against fields list", func() {
			// "DynamicFields" not listed fails init.
			config.DynamicFields = []string{"asdf", "jkl;"}
			config.Fields = []string{"Logger", "Hostname"}
			err := encoder.Init(config)
			c.Assume(err, gs.Not(gs.IsNil))
			msg := "\"DynamicFields\" must be in 'fields' list if using 'dynamic_fields'"
			c.Expect(err.Error(), gs.Equals, msg)

			// "DynamicFields" listed passes init.
			config.Fields = []string{"Logger", "Hostname", "DynamicFields"}
			err = encoder.Init(config)
			c.Expect(err, gs.IsNil)
			c.Expect(len(encoder.dynamicFields), gs.Equals, 2)

			// "Fields" works as an alias for "DynamicFields".
			config.Fields = []string{"Logger", "Hostname", "Fields"}
			err = encoder.Init(config)
			c.Expect(err, gs.IsNil)
			c.Expect(len(encoder.dynamicFields), gs.Equals, 2)
		})

		c.Specify("honors dynamic fields", func() {
			c.Specify("when dynamic_fields is empty", func() {
				err := encoder.Init(config)
				c.Expect(err, gs.IsNil)
				b, err := encoder.Encode(pack)
				c.Expect(err, gs.IsNil)

				output := string(b)
				lines := strings.Split(output, string(NEWLINE))
				decoded := make(map[string]interface{})
				err = json.Unmarshal([]byte(lines[1]), &decoded)
				c.Assume(err, gs.IsNil)
				c.Expect(len(decoded), gs.Equals, 10)
				fieldsValInterface := decoded["@fields"]
				fieldsVal := fieldsValInterface.(map[string]interface{})
				c.Expect(len(fieldsVal), gs.Equals, 13)
			})
		})
	})

	c.Specify("ESJsonEncoder", func() {
		encoder := new(ESJsonEncoder)
		config := encoder.ConfigStruct().(*ESJsonEncoderConfig)
		config.RawBytesFields = []string{
			"test_raw_field_string",
			"test_raw_field_bytes",
			"test_raw_field_string_array",
			"test_raw_field_bytes_array",
		}

		c.Specify("Should properly encode a message", func() {
			err := encoder.Init(config)
			c.Assume(err, gs.IsNil)
			b, err := encoder.Encode(pack)
			c.Assume(err, gs.IsNil)

			output := string(b)
			lines := strings.Split(output, string(NEWLINE))

			decoded := make(map[string]interface{})
			err = json.Unmarshal([]byte(lines[0]), &decoded)
			c.Assume(err, gs.IsNil)
			sub := decoded["index"].(map[string]interface{})
			t := time.Now().UTC()
			c.Expect(sub["_index"], gs.Equals, "heka-"+t.Format("2006.01.02"))
			c.Expect(sub["_type"], gs.Equals, "message")

			decoded = make(map[string]interface{})
			err = json.Unmarshal([]byte(lines[1]), &decoded)
			c.Assume(err, gs.IsNil)
			c.Expect(decoded[`"foo`], gs.Equals, "bar\n")
			c.Expect(decoded[`"number`], gs.Equals, 64.0)
			c.Expect(decoded["\xEF\xBF\xBD"], gs.Equals, "\xEF\xBF\xBD")
			c.Expect(decoded["Uuid"], gs.Equals, "87cf1ac2-e810-4ddf-a02d-a5ce44d13a85")
			c.Expect(decoded["Timestamp"], gs.Equals, "2013-07-16T15:49:05")
			c.Expect(decoded["Type"], gs.Equals, "TEST")
			c.Expect(decoded["Logger"], gs.Equals, "GoSpec")
			c.Expect(decoded["Severity"], gs.Equals, 6.0)
			c.Expect(decoded["Payload"], gs.Equals, "Test Payload")
			c.Expect(decoded["EnvVersion"], gs.Equals, "0.8")
			c.Expect(decoded["Pid"], gs.Equals, 14098.0)
			c.Expect(decoded["Hostname"], gs.Equals, "hostname")

			stringsArray := decoded["stringArray"].([]interface{})
			c.Expect(len(stringsArray), gs.Equals, 4)
			c.Expect(stringsArray[0], gs.Equals, "asdf")
			c.Expect(stringsArray[1], gs.Equals, "jkl;")
			c.Expect(stringsArray[2], gs.Equals, "push")
			c.Expect(stringsArray[3], gs.Equals, "pull")

			bytesArray := decoded["byteArray"].([]interface{})
			c.Expect(len(bytesArray), gs.Equals, 2)
			c.Expect(bytesArray[0], gs.Equals, base64.StdEncoding.EncodeToString([]byte("asdf")))
			c.Expect(bytesArray[1], gs.Equals, base64.StdEncoding.EncodeToString([]byte("jkl;")))

			integerArray := decoded["integerArray"].([]interface{})
			c.Expect(len(integerArray), gs.Equals, 3)
			c.Expect(integerArray[0], gs.Equals, 22.0)
			c.Expect(integerArray[1], gs.Equals, 80.0)
			c.Expect(integerArray[2], gs.Equals, 3000.0)

			doubleArray := decoded["doubleArray"].([]interface{})
			c.Expect(len(doubleArray), gs.Equals, 2)
			c.Expect(doubleArray[0], gs.Equals, 42.0)
			c.Expect(doubleArray[1], gs.Equals, 19101.3)

			boolArray := decoded["boolArray"].([]interface{})
			c.Expect(len(boolArray), gs.Equals, 5)
			c.Expect(boolArray[0], gs.Equals, true)
			c.Expect(boolArray[1], gs.Equals, false)
			c.Expect(boolArray[2], gs.Equals, false)
			c.Expect(boolArray[3], gs.Equals, false)
			c.Expect(boolArray[4], gs.Equals, true)

			c.Expect(decoded["test_raw_field_string"].(map[string]interface{})["asdf"], gs.Equals, 123.0)
			c.Expect(decoded["test_raw_field_bytes"].(map[string]interface{})["asdf"], gs.Equals, 123.0)
			c.Expect(decoded["test_raw_field_string_array"].([]interface{})[0].(map[string]interface{})["asdf"], gs.Equals, 123.0)
			c.Expect(decoded["test_raw_field_string_array"].([]interface{})[1].(map[string]interface{})["jkl;"], gs.Equals, 123.0)
			c.Expect(decoded["test_raw_field_bytes_array"].([]interface{})[0].(map[string]interface{})["asdf"], gs.Equals, 123.0)
			c.Expect(decoded["test_raw_field_bytes_array"].([]interface{})[1].(map[string]interface{})["jkl;"], gs.Equals, 123.0)
		})

		c.Specify("Should use field mappings", func() {
			config := encoder.ConfigStruct().(*ESJsonEncoderConfig)
			config.FieldMappings = &ESFieldMappings{
				Timestamp:  "XTimestamp",
				Uuid:       "XUuid",
				Type:       "XType",
				Logger:     "XLogger",
				Severity:   "XSeverity",
				Payload:    "XPayload",
				EnvVersion: "XEnvVersion",
				Pid:        "XPid",
				Hostname:   "XHostname",
			}
			err := encoder.Init(config)
			c.Assume(err, gs.IsNil)
			b, err := encoder.Encode(pack)
			c.Assume(err, gs.IsNil)

			output := string(b)
			lines := strings.Split(output, string(NEWLINE))

			decoded := make(map[string]interface{})
			err = json.Unmarshal([]byte(lines[1]), &decoded)
			c.Assume(err, gs.IsNil)
			c.Expect(decoded["XTimestamp"], gs.Equals, "2013-07-16T15:49:05")
			c.Expect(decoded["XUuid"], gs.Equals, "87cf1ac2-e810-4ddf-a02d-a5ce44d13a85")
			c.Expect(decoded["XType"], gs.Equals, "TEST")
			c.Expect(decoded["XLogger"], gs.Equals, "GoSpec")
			c.Expect(decoded["XSeverity"], gs.Equals, 6.0)
			c.Expect(decoded["XPayload"], gs.Equals, "Test Payload")
			c.Expect(decoded["XEnvVersion"], gs.Equals, "0.8")
			c.Expect(decoded["XPid"], gs.Equals, 14098.0)
			c.Expect(decoded["XHostname"], gs.Equals, "hostname")
		})

		c.Specify("encodes w/ a different timestamp format", func() {
			config.Timestamp = "%Y/%m/%d %H:%M:%S %z"
			err := encoder.Init(config)
			c.Expect(err, gs.IsNil)
			b, err := encoder.Encode(pack)
			c.Expect(err, gs.IsNil)

			output := string(b)
			lines := strings.Split(output, string(NEWLINE))
			decoded := make(map[string]interface{})
			err = json.Unmarshal([]byte(lines[1]), &decoded)
			c.Assume(err, gs.IsNil)

			c.Expect(decoded["Timestamp"], gs.Equals, "2013/07/16 15:49:05 +0000")
		})

		c.Specify("validates field list", func() {
			config.Fields = []string{"severity", "Hogger", "Lostname"}
			err := encoder.Init(config)
			c.Assume(err, gs.Not(gs.IsNil))
			msg := "Unsupported value \"Hogger\" in 'fields' list, must be one of"
			c.Expect(err.Error()[:len(msg)], gs.Equals, msg)
		})

		c.Specify("validates dynamic_fields against fields list", func() {
			// "DynamicFields" not listed fails init.
			config.DynamicFields = []string{"asdf", "jkl;"}
			config.Fields = []string{"Logger", "Hostname"}
			err := encoder.Init(config)
			c.Assume(err, gs.Not(gs.IsNil))
			msg := "\"DynamicFields\" must be in 'fields' list if using 'dynamic_fields'"
			c.Expect(err.Error(), gs.Equals, msg)

			// "DynamicFields" listed passes init.
			config.Fields = []string{"Logger", "Hostname", "DynamicFields"}
			err = encoder.Init(config)
			c.Expect(err, gs.IsNil)
			c.Expect(len(encoder.dynamicFields), gs.Equals, 2)

			// "Fields" works as an alias for "DynamicFields".
			config.Fields = []string{"Logger", "Hostname", "Fields"}
			err = encoder.Init(config)
			c.Expect(err, gs.IsNil)
			c.Expect(len(encoder.dynamicFields), gs.Equals, 2)
		})

		c.Specify("honors dynamic fields", func() {

			c.Specify("when dynamic_fields is empty", func() {
				err := encoder.Init(config)
				c.Assume(err, gs.IsNil)
				b, err := encoder.Encode(pack)
				c.Expect(err, gs.IsNil)

				output := string(b)
				lines := strings.Split(output, string(NEWLINE))
				decoded := make(map[string]interface{})
				err = json.Unmarshal([]byte(lines[1]), &decoded)
				c.Assume(err, gs.IsNil)
				c.Expect(len(decoded), gs.Equals, 22) // 9 base fields and 13 dynamic fields.
			})

			c.Specify("when dynamic_fields is specified", func() {
				config.DynamicFields = []string{"idField", "stringArray"}
				err := encoder.Init(config)
				c.Assume(err, gs.IsNil)
				b, err := encoder.Encode(pack)
				c.Expect(err, gs.IsNil)

				output := string(b)
				lines := strings.Split(output, string(NEWLINE))
				decoded := make(map[string]interface{})
				err = json.Unmarshal([]byte(lines[1]), &decoded)
				c.Assume(err, gs.IsNil)
				c.Expect(len(decoded), gs.Equals, 11) // 9 base fields and 2 dynamic fields.
			})
		})
	})
}
