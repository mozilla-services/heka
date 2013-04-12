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
	EncodeMessageStream(msg *message.Message, outBytes *[]byte) error
}

type JsonEncoder struct {
	signer *message.MessageSigningConfig
}

func NewJsonEncoder(signer *message.MessageSigningConfig) *JsonEncoder {
	return &JsonEncoder{signer}
}

func (self *JsonEncoder) EncodeMessage(msg *message.Message) ([]byte, error) {
	return json.Marshal(msg)
}

func (self *JsonEncoder) EncodeMessageStream(msg *message.Message, outBytes *[]byte) (err error) {
	msgBytes, err := self.EncodeMessage(msg)
	if err == nil {
		err = createStream(msgBytes, message.Header_JSON, outBytes, self.signer)
	}
	return
}

type ProtobufEncoder struct {
	signer *message.MessageSigningConfig
}

func NewProtobufEncoder(signer *message.MessageSigningConfig) *ProtobufEncoder {
	return &ProtobufEncoder{signer}
}

func (self *ProtobufEncoder) EncodeMessage(msg *message.Message) ([]byte, error) {
	return proto.Marshal(msg)
}

func (self *ProtobufEncoder) EncodeMessageStream(msg *message.Message, outBytes *[]byte) (err error) {
	msgBytes, err := self.EncodeMessage(msg) // TODO if we compute the size of the header first this can be marshaled directly to outBytes
	if err == nil {
		err = createStream(msgBytes, message.Header_PROTOCOL_BUFFER, outBytes, self.signer)
	}
	return
}

func createStream(msgBytes []byte, encoding message.Header_MessageEncoding,
	outBytes *[]byte, msc *message.MessageSigningConfig) error {
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
	if cap(*outBytes) < requiredSize {
		*outBytes = make([]byte, requiredSize, requiredSize+len(msgBytes))
	} else {
		*outBytes = (*outBytes)[:requiredSize]
	}
	(*outBytes)[0] = message.RECORD_SEPARATOR
	(*outBytes)[1] = uint8(headerSize)
	pbuf := proto.NewBuffer((*outBytes)[2:2])
	err := pbuf.Marshal(h)
	if err != nil {
		return err
	}
	(*outBytes)[headerSize+2] = message.UNIT_SEPARATOR
	*outBytes = append(*outBytes, msgBytes...)
	return nil
}
