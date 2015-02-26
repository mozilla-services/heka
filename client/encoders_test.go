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
#   Mike Trinkala (trink@mozilla.com)
#
# ***** END LICENSE BLOCK *****/

package client

import (
	"bytes"
	"github.com/mozilla-services/heka/message"
	"strings"
	"testing"
)

func TestEncodeMessageStream(t *testing.T) {
	var out []byte
	msg := &message.Message{}
	msg.SetType("TEST")
	msg.SetTimestamp(1416840893000000000)
	expected := []byte{0x1e, 0x2, 0x8, 0x10, 0x1f, 0x10, 0x80, 0xc4, 0x8c, 0x94, 0x91, 0xa9, 0xe8, 0xd4, 0x13, 0x1a, 0x4, 0x54, 0x45, 0x53, 0x54}
	pe := NewProtobufEncoder(nil)

	err := pe.EncodeMessageStream(msg, &out)
	if err != nil {
		t.Errorf("EncodeMessageStream failed: %s", err)
	} else if !bytes.Equal(expected, out) {
		t.Errorf("EncodeMessageStream expected: %#v received: %#v", expected, out)
	}
}

func TestEncodeMessageStreamSigned(t *testing.T) {
	var out []byte
	sk := &message.MessageSigningConfig{Name: "test"}
	msg := &message.Message{}
	msg.SetType("TEST")
	msg.SetTimestamp(1416840893000000000)
	expected := []byte{0x1e, 0x1c, 0x8, 0x10, 0x22, 0x4, 0x74, 0x65, 0x73, 0x74, 0x28, 0x0, 0x32, 0x10, 0x78, 0x35, 0x7e, 0x36, 0x35, 0x38, 0x3f, 0x9f, 0xbc, 0x75, 0x98, 0x46, 0x1b, 0x5, 0xef, 0x2d, 0x1f, 0x10, 0x80, 0xc4, 0x8c, 0x94, 0x91, 0xa9, 0xe8, 0xd4, 0x13, 0x1a, 0x4, 0x54, 0x45, 0x53, 0x54}
	pe := NewProtobufEncoder(sk)

	err := pe.EncodeMessageStream(msg, &out)
	if err != nil {
		t.Errorf("EncodeMessageStream failed: %s", err)
	} else if !bytes.Equal(expected, out) {
		t.Errorf("EncodeMessageStream expected: %#v received: %#v", expected, out)
	}
}

func TestEncodeMessageStreamMessageLengthFailure(t *testing.T) {
	var out []byte
	msg := &message.Message{}
	msg.SetType("TEST")
	msg.SetTimestamp(1416840893000000000)
	msg.SetPayload(strings.Repeat("x", int(message.MAX_MESSAGE_SIZE)))
	pe := NewProtobufEncoder(nil)
	expected := "Message too big, requires 65556 (MAX_MESSAGE_SIZE = 65536)"

	err := pe.EncodeMessageStream(msg, &out)
	if err == nil {
		t.Errorf("EncodeMessageStream should have failed on long message")
	} else if err.Error() != expected {
		t.Errorf("EncodeMessageStream expected: %s received: %s", expected, err)
	}
}

func TestEncodeMessageStreamHeaderLengthFailure(t *testing.T) {
	var out []byte
	sk := &message.MessageSigningConfig{Name: strings.Repeat("x", message.MAX_HEADER_SIZE)}
	msg := &message.Message{}
	msg.SetType("TEST")
	msg.SetTimestamp(1416840893000000000)
	pe := NewProtobufEncoder(sk)
	expected := "Message header too big, requires 280 (MAX_HEADER_SIZE = 255)"

	err := pe.EncodeMessageStream(msg, &out)
	if err == nil {
		t.Errorf("EncodeMessageStream should have failed on long message")
	} else if err.Error() != expected {
		t.Errorf("EncodeMessageStream expected: %s received: %s", expected, err)
	}
}
