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
	"fmt"
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
		err = createStream(msgBytes, outBytes, self.signer)
	}
	return
}

type ProtobufEncoder struct {
	signer *message.MessageSigningConfig
}

func NewProtobufEncoder(signer *message.MessageSigningConfig) *ProtobufEncoder {
	return &ProtobufEncoder{signer}
}

func (p *ProtobufEncoder) EncodeMessage(msg *message.Message) ([]byte, error) {
	return proto.Marshal(msg)
}

func (p *ProtobufEncoder) EncodeMessageStream(msg *message.Message, outBytes *[]byte) (err error) {
	msgBytes, err := p.EncodeMessage(msg) // TODO if we compute the size of the header first this can be marshaled directly to outBytes
	if err == nil {
		err = createStream(msgBytes, outBytes, p.signer)
	}
	return
}

func createStream(msgBytes []byte, outBytes *[]byte, msc *message.MessageSigningConfig) error {
	h := &message.Header{}
	h.SetMessageLength(uint32(len(msgBytes)))
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
	headerSize := proto.Size(h)
	requiredSize := message.HEADER_FRAMING_SIZE + headerSize + len(msgBytes)
	if requiredSize > message.MAX_RECORD_SIZE {
		return fmt.Errorf("Message too big, requires %d (MAX_RECORD_SIZE = %d)",
			message.MAX_RECORD_SIZE, requiredSize)
	}
	if cap(*outBytes) < requiredSize {
		*outBytes = make([]byte, requiredSize)
	} else {
		*outBytes = (*outBytes)[:requiredSize]
	}
	(*outBytes)[0] = message.RECORD_SEPARATOR
	(*outBytes)[1] = uint8(headerSize)
	// This looks odd but is correct; it effectively "seeks" the initial write
	// position for the protobuf output to be at the
	// `(*outBytes)[message.HEADER_DELIMITER_SIZE]` position.
	pbuf := proto.NewBuffer((*outBytes)[message.HEADER_DELIMITER_SIZE:message.HEADER_DELIMITER_SIZE])
	if err := pbuf.Marshal(h); err != nil {
		return err
	}
	(*outBytes)[headerSize+message.HEADER_DELIMITER_SIZE] = message.UNIT_SEPARATOR
	copy((*outBytes)[message.HEADER_FRAMING_SIZE+headerSize:], msgBytes)
	return nil
}
