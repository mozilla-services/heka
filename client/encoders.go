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
#   Mike Trinkala (trink@mozilla.com)
#
# ***** END LICENSE BLOCK *****/
package client

import (
	"code.google.com/p/goprotobuf/proto"
	"encoding/json"
	"github.com/mozilla-services/heka/message"
	"github.com/mozilla-services/heka/pipeline"
)

type Encoder interface {
	EncodeMessage(msg *message.Message) ([]byte, error)
}

type JsonEncoder struct {
}

type ProtobufEncoder struct {
	buffer *proto.Buffer
}

func (self *JsonEncoder) EncodeMessage(msg *message.Message) ([]byte, error) {
	result, err := json.Marshal(msg)
	return result, err
}

func (self *ProtobufEncoder) EncodeMessage(msg *message.Message) ([]byte, error) {
	if self.buffer == nil {
		buf := make([]byte, pipeline.MAX_MESSAGE_SIZE)
		self.buffer = proto.NewBuffer(buf)
	}
	self.buffer.Reset()
	err := self.buffer.Marshal(msg)
	return self.buffer.Bytes(), err
}
