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
	"bytes"
	"encoding/base64"
	"fmt"
	"github.com/mozilla-services/heka/message"
	"github.com/mozilla-services/heka/pipeline"
	"strconv"
	"strings"
	"time"
)

// RstEncoder generates a restructured text rendering of a Heka message,
// useful for debugging.
type RstEncoder struct {
	typeNames []string
}

func (re *RstEncoder) Init(config interface{}) (err error) {
	re.typeNames = make([]string, len(message.Field_ValueType_name))
	for i, typeName := range message.Field_ValueType_name {
		re.typeNames[i] = strings.ToLower(typeName)
	}
	return
}

func (re *RstEncoder) writeAttr(buf *bytes.Buffer, name, value string) {
	buf.WriteString(fmt.Sprintf(":%s: %s\n", name, value))
}

func (re *RstEncoder) writeField(buf *bytes.Buffer, name, typeName, repr string,
	values []string) {

	isString := typeName == "string"
	buf.WriteString(fmt.Sprintf("    | name:\"%s\" type:%s value:", name, typeName))
	var value string
	if len(values) == 1 {
		value = values[0]
		if isString {
			value = fmt.Sprintf("\"%s\"", value)
		}
		buf.WriteString(value)
	} else {
		var i int
		buf.WriteString("[")
		for i, value = range values {
			if i > 0 {
				buf.WriteString(",")
			}
			if isString {
				value = fmt.Sprintf("\"%s\"", value)
			}
			buf.WriteString(value)
		}
		buf.WriteString("]")
	}
	if repr != "" {
		buf.WriteString(fmt.Sprintf(" representation:\"%s\"", repr))
	}
	buf.WriteString("\n")
}

func (re *RstEncoder) Encode(pack *pipeline.PipelinePack) (output []byte, err error) {
	// Writing out the message attributes is easy.
	buf := new(bytes.Buffer)
	buf.WriteString("\n")
	timestamp := time.Unix(0, pack.Message.GetTimestamp()).UTC()
	re.writeAttr(buf, "Timestamp", timestamp.String())
	re.writeAttr(buf, "Type", pack.Message.GetType())
	re.writeAttr(buf, "Hostname", pack.Message.GetHostname())
	re.writeAttr(buf, "Pid", strconv.Itoa(int(pack.Message.GetPid())))
	re.writeAttr(buf, "Uuid", pack.Message.GetUuidString())
	re.writeAttr(buf, "Logger", pack.Message.GetLogger())
	re.writeAttr(buf, "Payload", pack.Message.GetPayload())
	re.writeAttr(buf, "EnvVersion", pack.Message.GetEnvVersion())
	re.writeAttr(buf, "Severity", strconv.Itoa(int(pack.Message.GetSeverity())))

	// Writing out the dynamic message fields is a bit of a PITA.
	fields := pack.Message.GetFields()
	if len(fields) > 0 {
		buf.WriteString(":Fields:\n")
		for _, field := range fields {
			valueType := field.GetValueType()
			typeName := re.typeNames[valueType]
			var values []string
			switch valueType {
			case message.Field_STRING:
				values = field.GetValueString()
			case message.Field_BYTES:
				vBytes := field.GetValueBytes()
				values = make([]string, len(vBytes))
				for i, v := range vBytes {
					values[i] = base64.StdEncoding.EncodeToString(v)
				}
			case message.Field_DOUBLE:
				vDoubles := field.GetValueDouble()
				values = make([]string, len(vDoubles))
				for i, v := range vDoubles {
					values[i] = strconv.FormatFloat(v, 'g', -1, 64)
				}
			case message.Field_INTEGER:
				vInts := field.GetValueInteger()
				values = make([]string, len(vInts))
				for i, v := range vInts {
					values[i] = strconv.FormatInt(v, 10)
				}
			case message.Field_BOOL:
				vBools := field.GetValueBool()
				values = make([]string, len(vBools))
				for i, v := range vBools {
					values[i] = strconv.FormatBool(v)
				}
			}
			re.writeField(buf, field.GetName(), typeName, field.GetRepresentation(),
				values)
		}
	}
	buf.WriteString("\n")
	return buf.Bytes(), nil
}

func init() {
	pipeline.RegisterPlugin("RstEncoder", func() interface{} {
		return new(RstEncoder)
	})
}
