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
	"bytes"
	"code.google.com/p/goprotobuf/proto"
	"encoding/json"
	"github.com/mozilla-services/heka/message"
)

type Encoder interface {
	EncodeMessage(msg *message.Message) ([]byte, error)
}

type JsonEncoder struct {
}

type ProtobufEncoder struct {
}

func (self *JsonEncoder) EncodeMessage(msg *message.Message) ([]byte, error) {
	result, err := json.Marshal(msg)
	return result, err
}

func (self *ProtobufEncoder) EncodeMessage(msg *message.Message) ([]byte, error) {
	result, err := proto.Marshal(msg)
	return result, err
}
