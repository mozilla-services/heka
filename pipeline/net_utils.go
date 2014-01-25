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

package pipeline

import (
	"code.google.com/p/go-uuid/uuid"
	"code.google.com/p/goprotobuf/proto"
	"crypto/hmac"
	"crypto/md5"
	"crypto/sha1"
	"crypto/subtle"
	"fmt"
	"github.com/mozilla-services/heka/client"
	. "github.com/mozilla-services/heka/message"
	"hash"
	"io"
	"log"
	"net"
	"time"
)

const NEWLINE byte = 10

// Create a protocol buffers stream for the given message, put it in the
// provided byte slice.
func ProtobufEncodeMessage(pack *PipelinePack, outBytes *[]byte) (err error) {
	enc := client.NewProtobufEncoder(nil)
	err = enc.EncodeMessageStream(pack.Message, outBytes)
	return
}

// ConfigStruct for NetworkInput plugins.
type NetworkInputConfig struct {
	// Network type (e.g. tcp, tcp4, tcp6, udp, udp4, udp6). Needs to match the input type.
	Net string
	// String representation of the address of the network connection on which
	// the listener should be listening (e.g. "127.0.0.1:5565").
	Address string
	// Set of message signer objects, keyed by signer id string.
	Signers map[string]Signer `toml:"signer"`
	// Name of configured decoder to receive the input
	Decoder string
	// Type of parser used to break the stream up into messages
	ParserType string `toml:"parser_type"`
	// Delimiter used to split the stream into messages
	Delimiter string
	// String indicating if the delimiter is at the start or end of the line,
	// only used for regexp delimiters
	DelimiterLocation string `toml:"delimiter_location"`
}

type NetworkParseFunction func(conn net.Conn,
	parser StreamParser,
	ir InputRunner,
	config *NetworkInputConfig,
	dr DecoderRunner) (err error)

// Standard text log file parser
func NetworkPayloadParser(conn net.Conn,
	parser StreamParser,
	ir InputRunner,
	config *NetworkInputConfig,
	dr DecoderRunner) (err error) {
	var (
		pack   *PipelinePack
		record []byte
	)
	_, record, err = parser.Parse(conn)
	if len(record) > 0 {
		pack = <-ir.InChan()
		pack.Message.SetUuid(uuid.NewRandom())
		pack.Message.SetTimestamp(time.Now().UnixNano())
		pack.Message.SetType("NetworkInput")
		pack.Message.SetSeverity(int32(0))
		pack.Message.SetEnvVersion("0.8")
		pack.Message.SetPid(0)
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
	config *NetworkInputConfig,
	dr DecoderRunner) (err error) {
	var (
		pack   *PipelinePack
		record []byte
	)
	_, record, err = parser.Parse(conn)
	if err != nil {
		if err == io.ErrShortBuffer {
			ir.LogError(fmt.Errorf("record exceeded MAX_RECORD_SIZE %d", MAX_RECORD_SIZE))
			err = nil // non-fatal, keep going
		}
	}
	if len(record) > 0 {
		pack = <-ir.InChan()
		headerLen := int(record[1]) + HEADER_FRAMING_SIZE
		messageLen := len(record) - headerLen
		if headerLen > UUID_SIZE {
			header := new(Header)
			DecodeHeader(record[2:headerLen], header)
			if authenticateMessage(config.Signers, header, record[headerLen:]) {
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
