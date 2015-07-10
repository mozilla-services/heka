/***** BEGIN LICENSE BLOCK *****
# This Source Code Form is subject to the terms of the Mozilla Public
# License, v. 2.0. If a copy of the MPL was not distributed with this file,
# You can obtain one at http://mozilla.org/MPL/2.0/.
#
# The Initial Developer of the Original Code is the Mozilla Foundation.
# Portions created by the Initial Developer are Copyright (C) 2012-2014
# the Initial Developer. All Rights Reserved.
#
# Contributor(s):
#   Rob Miller (rmiller@mozilla.com)
#   Mike Trinkala (trink@mozilla.com)
#
# ***** END LICENSE BLOCK *****/

package client

import (
	"crypto/hmac"
	"crypto/md5"
	"crypto/sha1"
	"fmt"
	"hash"

	"github.com/gogo/protobuf/proto"
	"github.com/mozilla-services/heka/message"
)

type Encoder interface {
	EncodeMessage(msg *message.Message) ([]byte, error)
}

type StreamEncoder interface {
	Encoder
	EncodeMessageStream(msg *message.Message, outBytes *[]byte) error
}

type ProtobufEncoder struct {
	Signer *message.MessageSigningConfig
}

func NewProtobufEncoder(signer *message.MessageSigningConfig) *ProtobufEncoder {
	return &ProtobufEncoder{signer}
}

func (p *ProtobufEncoder) EncodeMessage(msg *message.Message) ([]byte, error) {
	return proto.Marshal(msg)
}

func (p *ProtobufEncoder) EncodeMessageStream(msg *message.Message,
	outBytes *[]byte) (err error) {

	msgBytes, err := p.EncodeMessage(msg)
	// TODO if we compute the size of the header first this can be marshaled
	// directly to outBytes.
	if err == nil {
		err = CreateHekaStream(msgBytes, outBytes, p.Signer)
	}
	return
}

func CreateHekaStream(msgBytes []byte, outBytes *[]byte,
	msc *message.MessageSigningConfig) error {

	msgSize := uint32(len(msgBytes))
	if msgSize > message.MAX_MESSAGE_SIZE {
		return fmt.Errorf("Message too big, requires %d (MAX_MESSAGE_SIZE = %d)",
			len(msgBytes), message.MAX_MESSAGE_SIZE)
	}

	h := &message.Header{}
	h.SetMessageLength(msgSize)
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
	if headerSize > message.MAX_HEADER_SIZE {
		return fmt.Errorf("Message header too big, requires %d (MAX_HEADER_SIZE = %d)",
			headerSize, message.MAX_HEADER_SIZE)
	}

	requiredSize := message.HEADER_FRAMING_SIZE + headerSize + len(msgBytes)
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
