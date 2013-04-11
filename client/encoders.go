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
	"crypto/hmac"
	"crypto/md5"
	"crypto/sha1"
	"encoding/json"
	"github.com/mozilla-services/heka/message"
	"hash"
)

type Encoder interface {
	EncodeMessage(msg *message.Message) ([]byte, error)
	Encoding() message.Header_MessageEncoding
}

type JsonEncoder struct {
}

type ProtobufEncoder struct {
}

func (self *JsonEncoder) EncodeMessage(msg *message.Message) ([]byte, error) {
	return json.Marshal(msg)
}

func (self *JsonEncoder) Encoding() message.Header_MessageEncoding {
	return message.Header_JSON
}

func (self *ProtobufEncoder) EncodeMessage(msg *message.Message) ([]byte, error) {
	return proto.Marshal(msg)
}

func (self *ProtobufEncoder) Encoding() message.Header_MessageEncoding {
	return message.Header_PROTOCOL_BUFFER
}

func EncodeStreamHeader(msgBytes []byte,
	encoding message.Header_MessageEncoding,
	headerBytes *[]byte, msc *message.MessageSigningConfig) error {
	h := &message.Header{}
	h.SetMessageLength(uint32(len(msgBytes)))
	if encoding != message.Default_Header_MessageEncoding {
		h.SetMessageEncoding(encoding)
	}
	if msc != nil {
		h.SetHmacSigner(msc.Name)
		h.SetHmacKeyVersion(msc.Version)
		var hm hash.Hash
		switch msc.Hash {
		case "sha1":
			hm = hmac.New(sha1.New, []byte(msc.Key))
			h.SetHmacHashFunction(message.Header_SHA1)
		default:
			hm = hmac.New(md5.New, []byte(msc.Key))
		}

		hm.Write(msgBytes)
		h.SetHmac(hm.Sum(nil))
	}
	headerSize := uint8(proto.Size(h))
	requiredSize := int(3 + headerSize)
	if cap(*headerBytes) < requiredSize {
		*headerBytes = make([]byte, requiredSize)
	} else {
		*headerBytes = (*headerBytes)[:requiredSize]
	}
	(*headerBytes)[0] = message.RECORD_SEPARATOR
	(*headerBytes)[1] = uint8(headerSize)
	pbuf := proto.NewBuffer((*headerBytes)[2:2])
	err := pbuf.Marshal(h)
	if err != nil {
		return err
	}
	(*headerBytes)[headerSize+2] = message.UNIT_SEPARATOR
	return nil
}
