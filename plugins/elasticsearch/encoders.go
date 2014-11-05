/***** BEGIN LICENSE BLOCK *****
# This Source Code Form is subject to the terms of the Mozilla Public
# License, v. 2.0. If a copy of the MPL was not distributed with this file,
# You can obtain one at http://mozilla.org/MPL/2.0/.
#
# The Initial Developer of the Original Code is the Mozilla Foundation.
# Portions created by the Initial Developer are Copyright (C) 2013-2014
# the Initial Developer. All Rights Reserved.
#
# Contributor(s):
#   Tanguy Leroux (tlrx.dev@gmail.com)
#   Rob Miller (rmiller@mozilla.com)
#
# ***** END LICENSE BLOCK *****/

package elasticsearch

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"github.com/mozilla-services/heka/message"
	. "github.com/mozilla-services/heka/pipeline"
	"strconv"
	"strings"
	"time"
)

const lowerhex = "0123456789abcdef"

func writeUTF16Escape(b *bytes.Buffer, c rune) {
	b.WriteString(`\u`)
	b.WriteByte(lowerhex[(c>>12)&0xF])
	b.WriteByte(lowerhex[(c>>8)&0xF])
	b.WriteByte(lowerhex[(c>>4)&0xF])
	b.WriteByte(lowerhex[c&0xF])
}

// Go json encoder will blow up on invalid utf8 so we use this custom json
// encoder. Also, go json encoder generates these funny \U escapes which I
// don't think are valid json.

// Also note that invalid utf-8 sequences get encoded as U+FFFD this is a
// feature. :)

func writeQuotedString(b *bytes.Buffer, str string) {
	b.WriteString(`"`)

	// string = quotation-mark *char quotation-mark

	// char = unescaped /
	//        escape (
	//            %x22 /          ; "    quotation mark  U+0022
	//            %x5C /          ; \    reverse solidus U+005C
	//            %x2F /          ; /    solidus         U+002F
	//            %x62 /          ; b    backspace       U+0008
	//            %x66 /          ; f    form feed       U+000C
	//            %x6E /          ; n    line feed       U+000A
	//            %x72 /          ; r    carriage return U+000D
	//            %x74 /          ; t    tab             U+0009
	//            %x75 4HEXDIG )  ; uXXXX                U+XXXX

	// escape = %x5C              ; \

	// quotation-mark = %x22      ; "

	// unescaped = %x20-21 / %x23-5B / %x5D-10FFFF

	for _, c := range str {
		if c == 0x20 || c == 0x21 || (c >= 0x23 && c <= 0x5B) || (c >= 0x5D) {
			b.WriteRune(c)
		} else {

			// All runes should be < 16 bits because of the (c >= 0x5D) guard
			// above. However, runes are int32 so it is possible to have
			// negative values that won't be correctly outputted. However,
			// afaik these values are not part of the unicode standard.
			writeUTF16Escape(b, c)
		}

	}
	b.WriteString(`"`)

}

func writeField(first bool, b *bytes.Buffer, f *message.Field, raw bool) {
	if !first {
		b.WriteString(`,`)
	}

	writeQuotedString(b, f.GetName())
	b.WriteString(`:`)

	switch f.GetValueType() {
	case message.Field_STRING:
		values := f.GetValueString()
		if len(values) > 1 {
			b.WriteString(`[`)
			for i, value := range values {
				if raw {
					b.WriteString(value)
				} else {
					writeQuotedString(b, value)
				}
				if i < len(values) - 1 {
					b.WriteString(`,`)
				}
			}
			b.WriteString(`]`)
		} else {
			if raw {
				b.WriteString(values[0])
			} else {
				writeQuotedString(b, values[0])
			}
		}
	case message.Field_BYTES:
		values := f.GetValueBytes()
		if len(values) > 1 {
			b.WriteString(`[`)
			for i, value := range values {
				if raw {
					b.WriteString(string(value))
				} else {
					writeQuotedString(b, base64.StdEncoding.EncodeToString(value))
				}
				if i < len(values) - 1 {
					b.WriteString(`,`)
				}
			}
			b.WriteString(`]`)
		} else {
			if raw {
				b.WriteString(string(values[0]))
			} else {
				writeQuotedString(b, string(values[0]))
			}
		}
	case message.Field_INTEGER:
		values := f.GetValueInteger()
		if len(values) > 1 {
			b.WriteString(`[`)
			for i, value := range values {
				b.WriteString(strconv.FormatInt(value, 10))
				if i < len(values) - 1 {
					b.WriteString(`,`)
				}
			}
			b.WriteString(`]`)
		} else {
			b.WriteString(strconv.FormatInt(values[0], 10))
		}
	case message.Field_DOUBLE:
		values := f.GetValueDouble()
		if len(values) > 1 {
			b.WriteString(`[`)
			for i, value := range values {
				b.WriteString(strconv.FormatFloat(value, 'g', -1, 64))
				if i < len(values) - 1 {
					b.WriteString(`,`)
				}
			}
			b.WriteString(`]`)
		} else {
			b.WriteString(strconv.FormatFloat(values[0], 'g', -1, 64))
		}
	case message.Field_BOOL:
		values := f.GetValueBool()
		if len(values) > 1 {
			b.WriteString(`[`)
			for i, value := range values {
				b.WriteString(strconv.FormatBool(value))
				if i < len(values) - 1 {
					b.WriteString(`,`)
				}
			}
			b.WriteString(`]`)
		} else {
			b.WriteString(strconv.FormatBool(values[0]))
		}
	}
}

func writeStringField(first bool, b *bytes.Buffer, name string, value string) {
	if !first {
		b.WriteString(`,`)
	}

	writeQuotedString(b, name)
	b.WriteString(`:`)
	writeQuotedString(b, value)
}

func writeIntField(first bool, b *bytes.Buffer, name string, value int32) {
	if !first {
		b.WriteString(`,`)
	}
	writeQuotedString(b, name)
	b.WriteString(`:`)
	b.WriteString(strconv.Itoa(int(value)))
}

// Manually encodes the Heka message into an ElasticSearch friendly way.
type ESJsonEncoder struct {
	// Field names to include in ElasticSearch document for "clean" format.
	fields          []string
	timestampFormat string
	rawBytesFields  []string
	coord           *ElasticSearchCoordinates
}

type ESJsonEncoderConfig struct {
	// Name of the index in which the messages will be indexed. Defaults
	// to "heka-%{2006.01.02}".
	Index string
	// Name of the document type of the messages. Defaults to "message".
	TypeName string `toml:"type_name"`
	// Field names to include in ElasticSearch document.
	Fields []string
	// Timestamp format. Defaults to "2006-01-02T15:04:05.000Z"
	Timestamp string
	// When formating the Index use the Timestamp from the Message instead of
	// time of processing. Defaults to false.
	ESIndexFromTimestamp bool `toml:"es_index_from_timestamp"`
	// Document ID to use. Defaults to "".
	Id string
	// Fields to which formatting will not be applied.
	RawBytesFields []string `toml:"raw_bytes_fields"`
}

func (e *ESJsonEncoder) ConfigStruct() interface{} {
	config := &ESJsonEncoderConfig{
		Index:                "heka-%{2006.01.02}",
		TypeName:             "message",
		Timestamp:            "2006-01-02T15:04:05.000Z",
		ESIndexFromTimestamp: false,
		Id:                   "",
	}

	config.Fields = []string{
		"Uuid",
		"Timestamp",
		"Type",
		"Logger",
		"Severity",
		"Payload",
		"EnvVersion",
		"Pid",
		"Hostname",
		"Fields",
	}

	return config
}

func (e *ESJsonEncoder) Init(config interface{}) (err error) {
	conf := config.(*ESJsonEncoderConfig)
	e.fields = conf.Fields
	e.timestampFormat = conf.Timestamp
	e.rawBytesFields = conf.RawBytesFields
	e.coord = &ElasticSearchCoordinates{
		Index:                conf.Index,
		Type:                 conf.TypeName,
		ESIndexFromTimestamp: conf.ESIndexFromTimestamp,
		Id:                   conf.Id,
	}
	return
}

func (e *ESJsonEncoder) Encode(pack *PipelinePack) (output []byte, err error) {
	m := pack.Message
	buf := bytes.Buffer{}
	e.coord.PopulateBuffer(pack.Message, &buf)
	buf.WriteByte(NEWLINE)
	buf.WriteString(`{`)
	first := true
	for _, f := range e.fields {
		switch strings.ToLower(f) {
		case "uuid":
			writeStringField(first, &buf, f, m.GetUuidString())
		case "timestamp":
			t := time.Unix(0, m.GetTimestamp()).UTC()
			writeStringField(first, &buf, f, t.Format(e.timestampFormat))
		case "type":
			writeStringField(first, &buf, f, m.GetType())
		case "logger":
			writeStringField(first, &buf, f, m.GetLogger())
		case "severity":
			writeIntField(first, &buf, f, m.GetSeverity())
		case "payload":
			writeStringField(first, &buf, f, m.GetPayload())
		case "envversion":
			writeStringField(first, &buf, f, m.GetEnvVersion())
		case "pid":
			writeIntField(first, &buf, f, m.GetPid())
		case "hostname":
			writeStringField(first, &buf, f, m.GetHostname())
		case "fields":
			for _, field := range m.Fields {
				raw := false
				if len(e.rawBytesFields) > 0 {
					for _, raw_field_name := range e.rawBytesFields {
						if *field.Name == raw_field_name {
							raw = true
						}
					}
				}
				writeField(first, &buf, field, raw)
		 		first = false
			}
		default:
			err = fmt.Errorf("Unable to find field: %s", f)
			return
		}
		first = false
	}
	buf.WriteString(`}`)
	buf.WriteByte(NEWLINE)
	return buf.Bytes(), err
}

// Manually encodes messages into JSON structure matching Logstash's "version
// 1" schema, for compatibility with Kibana's out-of-box Logstash dashboards.
type ESLogstashV0Encoder struct {
	rawBytesFields []string
	coord          *ElasticSearchCoordinates
	// Field names to include in ElasticSearch document for "clean" format.
	fields         []string
	useMessageType bool
}

type ESLogstashV0EncoderConfig struct {
	// Name of the index in which the messages will be indexed. Defaults
	// to "logstash-%{2006.01.02}".
	Index string
	// Name of the document type of the messages. Defaults to "message".
	TypeName string `toml:"type_name"`
	// Should the @type field match the index _type. Defaults to false.
	UseMessageType bool `toml:"use_message_type"`
	// Field names to include in ElasticSearch document.
	Fields []string
	// When formating the Index use the Timestamp from the Message instead of
	// time of processing. Defaults to false.
	ESIndexFromTimestamp bool `toml:"es_index_from_timestamp"`
	// Document ID to use. Defaults to "".
	Id string
	// Fields to which formatting will not be applied.
	RawBytesFields []string `toml:"raw_bytes_fields"`
}

func (e *ESLogstashV0Encoder) ConfigStruct() interface{} {
	config := &ESLogstashV0EncoderConfig{
		Index:                "logstash-%{2006.01.02}",
		TypeName:             "message",
		UseMessageType:       false,
		ESIndexFromTimestamp: false,
		Id:                   "",
	}

	config.Fields = []string{
		"Uuid",
		"Timestamp",
		"Type",
		"Logger",
		"Severity",
		"Payload",
		"EnvVersion",
		"Pid",
		"Hostname",
		"Fields",
	}

	return config
}

func (e *ESLogstashV0Encoder) Init(config interface{}) (err error) {
	conf := config.(*ESLogstashV0EncoderConfig)
	e.rawBytesFields = conf.RawBytesFields
	e.fields = conf.Fields
	e.useMessageType = conf.UseMessageType
	e.coord = &ElasticSearchCoordinates{
		Index:                conf.Index,
		Type:                 conf.TypeName,
		ESIndexFromTimestamp: conf.ESIndexFromTimestamp,
		Id:                   conf.Id,
	}
	return
}

func (e *ESLogstashV0Encoder) Encode(pack *PipelinePack) (output []byte, err error) {
	m := pack.Message
	buf := bytes.Buffer{}
	e.coord.PopulateBuffer(pack.Message, &buf)
	buf.WriteByte(NEWLINE)
	buf.WriteString(`{`)

	first := true
	for _, f := range e.fields {
		switch strings.ToLower(f) {
		case "uuid":
			writeStringField(first, &buf, `@uuid`, m.GetUuidString())
		case "timestamp":
			t := time.Unix(0, m.GetTimestamp()).UTC()
			writeStringField(first, &buf, `@timestamp`, t.Format("2006-01-02T15:04:05.000Z"))
		case "type":
			if e.useMessageType || len(e.coord.Type) < 1 {
				writeStringField(first, &buf, `@type`, m.GetType())
			} else {
				var interpType string
				interpType, err = interpolateFlag(e.coord, m, e.coord.Type)
				if len(interpType) > 0 && err == nil {
					writeStringField(first, &buf, `@type`, interpType)
				} else {
					// fall back on writing the uninterpolated string
					writeStringField(first, &buf, `@type`, e.coord.Type)
				}
			}
		case "logger":
			writeStringField(first, &buf, `@logger`, m.GetLogger())
		case "severity":
			writeIntField(first, &buf, `@severity`, m.GetSeverity())
		case "payload":
			writeStringField(first, &buf, `@message`, m.GetPayload())
		case "envversion":
			writeStringField(first, &buf, `@envversion`, m.GetEnvVersion())
		case "pid":
			writeIntField(first, &buf, `@pid`, m.GetPid())
		case "hostname":
			writeStringField(first, &buf, `@source_host`, m.GetHostname())
		case "fields":
			if !first {
				buf.WriteString(`,`)
			}
			buf.WriteString(`"@fields":{`)
			firstfield := true
			for _, field := range m.Fields {
				raw := false
				if len(e.rawBytesFields) > 0 {
					for _, raw_field_name := range e.rawBytesFields {
						if *field.Name == raw_field_name {
							raw = true
						}
					}
				}
				writeField(firstfield, &buf, field, raw)
				firstfield = false
			}
			buf.WriteString(`}`) // end of fields
		default:
			err = fmt.Errorf("Unable to find field: %s", f)
			return
		}
		first = false
	}
	buf.WriteString(`}`)
	buf.WriteByte(NEWLINE)
	return buf.Bytes(), err
}

func init() {
	RegisterPlugin("ESJsonEncoder", func() interface{} {
		return new(ESJsonEncoder)
	})
	RegisterPlugin("ESLogstashV0Encoder", func() interface{} {
		return new(ESLogstashV0Encoder)
	})
}
