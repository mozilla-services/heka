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
package client

import (
	"bytes"
	"encoding/gob"
	"encoding/json"
	"fmt"
)

type Encoder interface {
	EncodeMessage(msg *Message) ([]byte, error)
}

type JsonEncoder struct {
}

func (self *JsonEncoder) EncodeMessage(msg *Message) ([]byte, error) {
	result, err := json.Marshal(msg)
	return result, err
}

var fmtString = `{"type":"%s","timestamp":%s,"logger":"%s","severity":%d,"payload":"%s","fields":%s,"env_version":"%s","metlog_pid":%d,"metlog_hostname":"%s"}`

var hex = "0123456789abcdef"

func escapeStr(inStr string) string {
	result := new(bytes.Buffer)
	for i := 0; i < len(inStr); i++ {
		b := inStr[i]
		if 0x20 <= b && b != '\\' && b != '"' && b != '<' && b != '>' {
			result.WriteByte(b)
			continue
		}
		switch b {
		case '\\', '"':
			result.WriteByte('\\')
			result.WriteByte(b)
		case '\n':
			result.WriteByte('\\')
			result.WriteByte('n')
		case '\r':
			result.WriteByte('\\')
			result.WriteByte('r')
		default:
			result.WriteString(`\u00`)
			result.WriteByte(hex[b>>4])
			result.WriteByte(hex[b&0xF])
		}
	}
	resultStr := result.String()
	return resultStr
}

func (self *Message) MarshalJSON() ([]byte, error) {
	fieldsJson, err := json.Marshal(self.Fields)
	if err != nil {
		return nil, err
	}
	timestampJson, err := json.Marshal(self.Timestamp)
	if err != nil {
		return nil, err
	}
	result := fmt.Sprintf(fmtString, escapeStr(self.Type),
		string(timestampJson), escapeStr(self.Logger),
		self.Severity, escapeStr(self.Payload),
		string(fieldsJson), escapeStr(self.Env_version), self.Pid,
		escapeStr(self.Hostname))
	return []byte(result), nil
}

type GobEncoder struct {
	encoder *gob.Encoder
	buffer  *bytes.Buffer
}

func NewGobEncoder() *GobEncoder {
	buffer := new(bytes.Buffer)
	encoder := gob.NewEncoder(buffer)
	return &(GobEncoder{encoder, buffer})
}

func (self *GobEncoder) EncodeMessage(msg *Message) ([]byte, error) {
	err := self.encoder.Encode(msg)
	var result []byte
	if err == nil {
		result = self.buffer.Bytes()
	}
	return result, err
}
