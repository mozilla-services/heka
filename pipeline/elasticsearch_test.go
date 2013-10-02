package pipeline

import (
	"bytes"
	"code.google.com/p/go-uuid/uuid"
	"encoding/json"
	. "github.com/mozilla-services/heka/message"
	gs "github.com/rafrombrc/gospec/src/gospec"
	"time"
)

func getTestMessageWithFunnyFields() *Message {
	field, _ := NewField(`"foo`, "bar\n", "")
	field1, _ := NewField(`"number`, 64, "")
	field2, _ := NewField("\xa3", "\xa3", "")

	msg := &Message{}
	msg.SetType("TEST")
	t, _ := time.Parse("2006-01-02T15:04:05.000Z", "2013-07-16T15:49:05.070Z")
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

	return msg
}

func ElasticSearchOutputSpec(c gs.Context) {
	c.Specify("Should properly encode special characters in json", func() {
		buf := bytes.Buffer{}
		writeStringField(true, &buf, `hello"bar`, "world\nfoo\\")
		c.Expect(buf.String(), gs.Equals, `"hello\u0022bar":"world\u000afoo\u005c"`)
	})

	c.Specify("Should replace invalid utf8 with replacement character", func() {
		buf := bytes.Buffer{}
		writeStringField(true, &buf, "\xa3", "\xa3")
		c.Expect(buf.String(), gs.Equals, "\"\xEF\xBF\xBD\":\"\xEF\xBF\xBD\"")
	})

	c.Specify("Should properly encode message using kibana formatter", func() {
		formatter := KibanaFormatter{}
		b, _ := formatter.Format(getTestMessageWithFunnyFields())

		decoded := make(map[string]interface{})

		err := json.Unmarshal(b, &decoded)

		c.Expect(err, gs.IsNil)
		c.Expect(decoded["@fields"].(map[string]interface{})[`"foo`], gs.Equals, "bar\n")
		c.Expect(decoded["@fields"].(map[string]interface{})[`"number`], gs.Equals, 64.0)
		c.Expect(decoded["@fields"].(map[string]interface{})["\xEF\xBF\xBD"], gs.Equals, "\xEF\xBF\xBD")

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

	c.Specify("Should properly encode message using payload formatter", func() {
		formatter := PayloadFormatter{}
		msg := getTestMessageWithFunnyFields()
		jsonPayload := `{"this": "is", "a": "test"}
{"of": "the", "payload": "formatter"}
`
		msg.SetPayload(jsonPayload)
		b, err := formatter.Format(msg)
		c.Expect(err, gs.IsNil)
		c.Expect(string(b), gs.Equals, jsonPayload)
	})

        c.Specify("Should interpolate fields and message attributes for index and type names", func() {
                interpolatedIndex := interpolateFlag(&ElasticSearchCoordinates{}, getTestMessageWithFunnyFields(), "heka-%{Pid}-%{\"foo}-%{2006.01.02}")
                interpolatedType := interpolateFlag(&ElasticSearchCoordinates{}, getTestMessageWithFunnyFields(), "%{Type}")
                t := time.Now()

                c.Expect(interpolatedIndex, gs.Equals, "heka-14098-bar\n-" + t.Format("2006.01.02"))
                c.Expect(interpolatedType, gs.Equals, "TEST")
        })

}
