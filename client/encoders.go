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
package hekaclient

import (
	"bytes"
	"encoding/json"
	"encoding/gob"
)

type JsonEncoder struct {
}

func (self *JsonEncoder) EncodeMessage(msg *Message) (*[]byte, error) {
	result, err := json.Marshal(msg)
	return &result, err
}

type GobEncoder struct {
	encoder *gob.Encoder
	buffer *bytes.Buffer
}

func NewGobEncoder() *GobEncoder {
	buffer := new(bytes.Buffer)
	encoder := gob.NewEncoder(buffer)
	return &(GobEncoder{encoder, buffer})
}

func (self *GobEncoder) EncodeMessage(msg *Message) (*[]byte, error) {
	err := self.encoder.Encode(msg)
	var result []byte
	if err == nil {
		result = self.buffer.Bytes()
	}
	return &result, err
}