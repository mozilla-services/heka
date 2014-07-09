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

package pipeline

import (
	"code.google.com/p/go-uuid/uuid"
	"code.google.com/p/goprotobuf/proto"
	"crypto/hmac"
	"crypto/md5"
	"crypto/sha1"
	"crypto/subtle"
	"fmt"
	. "github.com/mozilla-services/heka/message"
	"hash"
	"io"
	"log"
	"net"
	"time"
)

const NEWLINE byte = 10

type NetworkParseFunction func(conn net.Conn,
	parser StreamParser,
	ir InputRunner,
	signers map[string]Signer,
	dr DecoderRunner) (err error)

// Standard text log file parser
func NetworkPayloadParser(conn net.Conn,
	parser StreamParser,
	ir InputRunner,
	signers map[string]Signer,
	dr DecoderRunner) (err error) {
	var (
		pack   *PipelinePack
		record []byte
	)

	for true {
		_, record, err = parser.Parse(conn)
		if err != nil {
			if err == io.ErrShortBuffer {
				ir.LogError(fmt.Errorf("record exceeded MAX_RECORD_SIZE %d", MAX_RECORD_SIZE))
				err = nil // non-fatal
			}
		}
		if len(record) == 0 {
			break
		}
		pack = <-ir.InChan()
		pack.Message.SetUuid(uuid.NewRandom())
		pack.Message.SetTimestamp(time.Now().UnixNano())
		pack.Message.SetType("NetworkInput")
		// Only TCP packets have a remote address.
		if remoteAddr := conn.RemoteAddr(); remoteAddr != nil {
			pack.Message.SetHostname(remoteAddr.String())
		}
		pack.Message.SetLogger(ir.Name())
		pack.Message.SetPayload(string(record))
		if dr == nil {
			ir.Inject(pack)
		} else {
			dr.InChan() <- pack
		}
	}
	return
}

// Framed protobuf message parser
func NetworkMessageProtoParser(conn net.Conn,
	parser StreamParser,
	ir InputRunner,
	signers map[string]Signer,
	dr DecoderRunner) (err error) {
	var (
		pack   *PipelinePack
		record []byte
	)
	for true {
		_, record, err = parser.Parse(conn)
		if err != nil {
			if err == io.ErrShortBuffer {
				ir.LogError(fmt.Errorf("record exceeded MAX_RECORD_SIZE %d", MAX_RECORD_SIZE))
				err = nil // non-fatal
			}
		}
		if len(record) == 0 {
			break
		}
		pack = <-ir.InChan()
		headerLen := int(record[1]) + HEADER_FRAMING_SIZE
		messageLen := len(record) - headerLen
		if headerLen > UUID_SIZE {
			header := new(Header)
			DecodeHeader(record[2:headerLen], header)
			if authenticateMessage(signers, header, record[headerLen:]) {
				pack.Signer = header.GetHmacSigner()
			} else {
				pack.Recycle()
				return
			}
		}
		if messageLen > cap(pack.MsgBytes) {
			pack.MsgBytes = make([]byte, messageLen)
		}
		pack.MsgBytes = pack.MsgBytes[:messageLen]
		copy(pack.MsgBytes, record[headerLen:])
		dr.InChan() <- pack
	}
	return
}

// Heka Message signer object.
type Signer struct {
	HmacKey string `toml:"hmac_key"`
}

// Decodes provided byte slice into a Heka protocol header object.
func DecodeHeader(buf []byte, header *Header) bool {
	if buf[len(buf)-1] != UNIT_SEPARATOR {
		log.Println("missing unit separator")
		return false
	}
	err := proto.Unmarshal(buf[0:len(buf)-1], header)
	if err != nil {
		log.Println("error unmarshaling header:", err)
		return false
	}
	if header.GetMessageLength() > MAX_MESSAGE_SIZE {
		log.Printf("message exceeds the maximum length (bytes): %d", MAX_MESSAGE_SIZE)
		return false
	}
	return true
}

// Returns true if the provided message is unsigned or has a valid signature
// from one of the provided signers.
func authenticateMessage(signers map[string]Signer, header *Header, msg []byte) bool {
	digest := header.GetHmac()
	if digest != nil {
		var key string
		signer := fmt.Sprintf("%s_%d", header.GetHmacSigner(),
			header.GetHmacKeyVersion())
		if s, ok := signers[signer]; ok {
			key = s.HmacKey
		} else {
			return false
		}

		var hm hash.Hash
		switch header.GetHmacHashFunction() {
		case Header_MD5:
			hm = hmac.New(md5.New, []byte(key))
		case Header_SHA1:
			hm = hmac.New(sha1.New, []byte(key))
		}
		hm.Write(msg)
		expectedDigest := hm.Sum(nil)
		if subtle.ConstantTimeCompare(digest, expectedDigest) != 1 {
			return false
		}
	}
	return true
}
