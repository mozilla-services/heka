/***** BEGIN LICENSE BLOCK *****
# This Source Code Form is subject to the terms of the Mozilla Public
# License, v. 2.0. If a copy of the MPL was not distributed with this file,
# You can obtain one at http://mozilla.org/MPL/2.0/.
#
# The Initial Developer of the Original Code is the Mozilla Foundation.
# Portions created by the Initial Developer are Copyright (C) 2012-2015
# the Initial Developer. All Rights Reserved.
#
# Contributor(s):
#   Mike Trinkala (trink@mozilla.com)
#   Rob Miller (rmiller@mozilla.com)
#
# ***** END LICENSE BLOCK *****/

package pipeline

import (
	"bytes"
	"crypto/hmac"
	"crypto/md5"
	"crypto/sha1"
	"io"

	"github.com/gogo/protobuf/proto"
	"github.com/mozilla-services/heka/message"
	ts "github.com/mozilla-services/heka/pipeline/testsupport"
	gs "github.com/rafrombrc/gospec/src/gospec"
)

func makeSplitterRunner(name string, splitter Splitter) SplitterRunner {
	srConfig := CommonSplitterConfig{}
	return NewSplitterRunner(name, splitter, srConfig)
}

func TokenSpec(c gs.Context) {
	c.Specify("A TokenSplitter", func() {
		splitter := &TokenSplitter{}
		config := splitter.ConfigStruct().(*TokenSplitterConfig)
		sRunner := makeSplitterRunner("TokenSplitter", splitter)
		buf := []byte("test1\ntest12\ntest123\npartial")

		c.Specify("using default delimiter", func() {
			err := splitter.Init(config)
			c.Assume(err, gs.IsNil)
			reader := bytes.NewReader(buf)
			n, record, err := sRunner.GetRecordFromStream(reader)
			c.Expect(n, gs.Equals, 6)
			c.Expect(err, gs.IsNil)
			c.Expect(string(record), gs.Equals, "test1\n")
			n, record, err = sRunner.GetRecordFromStream(reader)
			c.Expect(n, gs.Equals, 7)
			c.Expect(err, gs.IsNil)
			c.Expect(string(record), gs.Equals, "test12\n")
			n, record, err = sRunner.GetRecordFromStream(reader)
			c.Expect(n, gs.Equals, 8)
			c.Expect(err, gs.IsNil)
			c.Expect(string(record), gs.Equals, "test123\n")
			n, record, err = sRunner.GetRecordFromStream(reader)
			c.Expect(n, gs.Equals, 0)
			c.Expect(err, gs.IsNil)
			c.Expect(string(sRunner.GetRemainingData()), gs.Equals, "partial")
		})

		c.Specify("using tab delimiter", func() {
			reader := bytes.NewReader([]byte("test1\ttest2\t"))
			config.Delimiter = "\t"
			err := splitter.Init(config)
			c.Assume(err, gs.IsNil)
			n, record, err := sRunner.GetRecordFromStream(reader)
			c.Expect(n, gs.Equals, 6)
			c.Expect(err, gs.IsNil)
			c.Expect(string(record), gs.Equals, "test1\t")
			n, record, err = sRunner.GetRecordFromStream(reader)
			c.Expect(n, gs.Equals, 6)
			c.Expect(err, gs.IsNil)
			c.Expect(string(record), gs.Equals, "test2\t")
		})

		c.Specify("max record size", func() {
			b := make([]byte, message.MAX_RECORD_SIZE)
			b[message.MAX_RECORD_SIZE-1] = '\t'
			reader := bytes.NewReader(b)
			config.Delimiter = "\t"
			err := splitter.Init(config)
			c.Assume(err, gs.IsNil)
			var n int
			var record []byte
			for err == nil && len(record) == 0 {
				n, record, err = sRunner.GetRecordFromStream(reader)
			}
			c.Expect(n, gs.Equals, int(message.MAX_RECORD_SIZE))
			c.Expect(string(record), gs.Equals, string(b))
			c.Expect(err, gs.IsNil)
		})

		c.Specify("exceed max record size", func() {
			b := make([]byte, message.MAX_RECORD_SIZE+1)
			reader := bytes.NewReader(b)
			config.Delimiter = "\t"
			err := splitter.Init(config)
			c.Assume(err, gs.IsNil)
			var n int
			var record []byte
			for err == nil {
				n, record, err = sRunner.GetRecordFromStream(reader)
			}
			c.Expect(n, gs.Equals, int(message.MAX_RECORD_SIZE))
			c.Expect(len(record), gs.Equals, 0)
			c.Expect(err, gs.Equals, io.ErrShortBuffer)
		})

		c.Specify("exceed max record size w/ truncated records", func() {
			srConfig := CommonSplitterConfig{}
			keep := true
			srConfig.KeepTruncated = &keep
			sRunner := NewSplitterRunner("TokenSplitter", splitter, srConfig)

			b := make([]byte, message.MAX_RECORD_SIZE+1)
			reader := bytes.NewReader(b)
			config.Delimiter = "\t"
			err := splitter.Init(config)
			c.Assume(err, gs.IsNil)

			var n int
			var record []byte
			for err == nil {
				n, record, err = sRunner.GetRecordFromStream(reader)
			}
			c.Expect(n, gs.Equals, int(message.MAX_RECORD_SIZE))
			c.Expect(len(record), gs.Equals, int(message.MAX_RECORD_SIZE))
			c.Expect(err, gs.Equals, io.ErrShortBuffer)
		})

		c.Specify("using count", func() {
			rExpected := append(buf, '\n')
			buf = bytes.Repeat(rExpected, 10)
			buf = buf[:len(buf)-1] // 40 lines separated by 39 newlines

			config.Count = 4
			err := splitter.Init(config)
			c.Assume(err, gs.IsNil)

			for i := 0; i < 9; i++ {
				n, r := splitter.FindRecord(buf)
				c.Expect(n, gs.Equals, len(rExpected))
				c.Expect(string(r), gs.Equals, string(rExpected))
				buf = buf[n:]
			}

			// Last record is incomplete b/c it only has 3 newlines.
			n, r := splitter.FindRecord(buf)
			c.Expect(n, gs.Equals, 0)
			c.Expect(len(r), gs.Equals, 0)
		})
	})
}

func RegexSpec(c gs.Context) {
	c.Specify("A RegexSplitter", func() {
		splitter := &RegexSplitter{}
		config := splitter.ConfigStruct().(*RegexSplitterConfig)
		sRunner := makeSplitterRunner("RegexSplitter", splitter)
		buf := []byte("test1\ntest12\ntest123\npartial")

		c.Specify("fails to init w/ invalid delimiter", func() {
			config.Delimiter = "\\y"
			err := splitter.Init(config)
			c.Expect(err.Error(), gs.Equals,
				"error parsing regexp: invalid escape sequence: `\\y`")
			config.Delimiter = "(\\r)(\\n)"
			err = splitter.Init(config)
			c.Expect(err.Error(), gs.Equals,
				"regex must not contain more than one capture group: (\\r)(\\n)")
		})

		c.Specify("splits w/ no capture)", func() {
			reader := bytes.NewReader(buf)
			err := splitter.Init(config)
			c.Assume(err, gs.IsNil)

			n, record, err := sRunner.GetRecordFromStream(reader)
			c.Expect(n, gs.Equals, 6)
			c.Expect(err, gs.IsNil)
			c.Expect(string(record), gs.Equals, "test1")
			n, record, err = sRunner.GetRecordFromStream(reader)
			c.Expect(n, gs.Equals, 7)
			c.Expect(err, gs.IsNil)
			c.Expect(string(record), gs.Equals, "test12")
			n, record, err = sRunner.GetRecordFromStream(reader)
			c.Expect(n, gs.Equals, 8)
			c.Expect(err, gs.IsNil)
			c.Expect(string(record), gs.Equals, "test123")
			n, record, err = sRunner.GetRecordFromStream(reader)
			c.Expect(n, gs.Equals, 0)
			c.Expect(err, gs.IsNil)
			c.Expect(string(sRunner.GetRemainingData()), gs.Equals, "partial")
		})

		c.Specify("splits w/ start delimiter capture", func() {
			reader := bytes.NewReader(buf)
			config.Delimiter = "(\n)"
			config.DelimiterEOL = false
			err := splitter.Init(config)
			c.Assume(err, gs.IsNil)

			n, record, err := sRunner.GetRecordFromStream(reader)
			c.Expect(n, gs.Equals, 5)
			c.Expect(err, gs.IsNil)
			c.Expect(string(record), gs.Equals, "test1")
			n, record, err = sRunner.GetRecordFromStream(reader)
			c.Expect(n, gs.Equals, 7)
			c.Expect(err, gs.IsNil)
			c.Expect(string(record), gs.Equals, "\ntest12")
			n, record, err = sRunner.GetRecordFromStream(reader)
			c.Expect(n, gs.Equals, 8)
			c.Expect(err, gs.IsNil)
			c.Expect(string(record), gs.Equals, "\ntest123")
			n, record, err = sRunner.GetRecordFromStream(reader)
			c.Expect(n, gs.Equals, 0)
			c.Expect(err, gs.IsNil)
			c.Expect(string(sRunner.GetRemainingData()), gs.Equals, "\npartial")
		})

		c.Specify("splits single record w/ start delimiter", func() {
			reader := bytes.NewReader([]byte("\ntest"))
			config.Delimiter = "(\n)"
			config.DelimiterEOL = false
			err := splitter.Init(config)
			c.Assume(err, gs.IsNil)

			n, record, err := sRunner.GetRecordFromStream(reader)
			c.Expect(n, gs.Equals, 0)
			c.Expect(len(record), gs.Equals, 0)
			c.Expect(err, gs.IsNil)
			c.Expect(string(sRunner.GetRemainingData()), gs.Equals, "\ntest")
		})

		c.Specify("splits single record w/ start delimiter no capture", func() {
			reader := bytes.NewReader([]byte("\ntest"))
			config.Delimiter = "\n"
			config.DelimiterEOL = false
			err := splitter.Init(config)
			c.Assume(err, gs.IsNil)

			n, record, err := sRunner.GetRecordFromStream(reader)
			c.Expect(n, gs.Equals, 1) // moves past the delimiter since it is not captured as part of the record
			c.Expect(err, gs.IsNil)
			c.Expect(len(record), gs.Equals, 0) // consume the empty record in front of the delimiter
			n, record, err = sRunner.GetRecordFromStream(reader)
			c.Expect(n, gs.Equals, 0)
			c.Expect(len(record), gs.Equals, 0)
			c.Expect(err, gs.Equals, io.EOF)
			c.Expect(string(sRunner.GetRemainingData()), gs.Equals, "test")
		})

		c.Specify("splits w/ capture", func() {
			reader := bytes.NewReader(buf)
			config.Delimiter = "(\n)"
			err := splitter.Init(config)
			c.Assume(err, gs.IsNil)

			n, record, err := sRunner.GetRecordFromStream(reader)
			c.Expect(n, gs.Equals, 6)
			c.Expect(err, gs.IsNil)
			c.Expect(string(record), gs.Equals, "test1\n")
			n, record, err = sRunner.GetRecordFromStream(reader)
			c.Expect(n, gs.Equals, 7)
			c.Expect(err, gs.IsNil)
			c.Expect(string(record), gs.Equals, "test12\n")
			n, record, err = sRunner.GetRecordFromStream(reader)
			c.Expect(n, gs.Equals, 8)
			c.Expect(err, gs.IsNil)
			c.Expect(string(record), gs.Equals, "test123\n")
			n, record, err = sRunner.GetRecordFromStream(reader)
			c.Expect(n, gs.Equals, 0)
			c.Expect(err, gs.IsNil)
			c.Expect(string(sRunner.GetRemainingData()), gs.Equals, "partial")
		})
	})
}

func encodeMessage(hbytes, mbytes []byte) (emsg []byte) {
	emsg = make([]byte, 3+len(hbytes)+len(mbytes))
	emsg[0] = message.RECORD_SEPARATOR
	emsg[1] = uint8(len(hbytes))
	copy(emsg[2:], hbytes)
	pos := 2 + len(hbytes)
	emsg[pos] = message.UNIT_SEPARATOR
	copy(emsg[pos+1:], mbytes)
	return
}

func HekaFramingSpec(c gs.Context) {
	c.Specify("A HekaFramingSplitter", func() {
		splitter := &HekaFramingSplitter{}
		config := splitter.ConfigStruct().(*HekaFramingSplitterConfig)
		sRunner := makeSplitterRunner("HekaFramingSplitter", splitter)

		c.Specify("splits records", func() {
			b := []byte("\x1e\x02\x08\x3e\x1f\x0a\x10\x90\x1d\x56\x27\xec\x49\x4c\x8f\xba\x8e\x84\x9b\xaa\xf7\xa6\xf6\x10\xa6\x97\x8a\x8f\xb6\xc1\xae\x8e\x13\x1a\x09\x68\x65\x6b\x61\x62\x65\x6e\x63\x68\x28\x06\x3a\x03\x30\x2e\x38\x40\xbf\xe5\x01\x4a\x0a\x74\x72\x69\x6e\x6b\x2d\x78\x32\x33\x30\x1e\x02\x08\x3e\x1f\x0a\x10\x90\x1d\x56\x27\xec\x49\x4c\x8f\xba\x8e\x84\x9b\xaa\xf7\xa6\xf6\x10\xa6\x97\x8a\x8f\xb6\xc1\xae\x8e\x13\x1a\x09\x68\x65\x6b\x61\x62\x65\x6e\x63\x68\x28\x06\x3a\x03\x30\x2e\x38\x40\xbf\xe5\x01\x4a\x0a\x74\x72\x69\x6e\x6b\x2d\x78\x32\x33\x30BOGUS\x1e\x02\x08\x3e\x1f\x0a\x10\x90\x1d\x56\x27\xec\x49\x4c\x8f\xba\x8e\x84\x9b\xaa\xf7\xa6\xf6\x10\xa6\x97\x8a\x8f\xb6\xc1\xae\x8e\x13\x1a\x09\x68\x65\x6b\x61\x62\x65\x6e\x63\x68\x28\x06\x3a\x03\x30\x2e\x38\x40\xbf\xe5\x01\x4a\x0a\x74\x72\x69\x6e\x6b\x2d\x78\x32\x33\x30BOGUS\x1e\x02\x08")
			reader := bytes.NewReader(b)
			err := splitter.Init(config)
			c.Assume(err, gs.IsNil)

			n, record, err := sRunner.GetRecordFromStream(reader)
			c.Expect(n, gs.Equals, 67)
			c.Expect(err, gs.IsNil)
			c.Expect(string(record), gs.Equals, string(b[:67]))
			n, record, err = sRunner.GetRecordFromStream(reader)
			c.Expect(n, gs.Equals, 67)
			c.Expect(err, gs.IsNil)
			c.Expect(string(record), gs.Equals, string(b[67:134]))
			n, record, err = sRunner.GetRecordFromStream(reader)
			c.Expect(n, gs.Equals, 72) // skips the invalid 'BOGUS' data
			c.Expect(err, gs.IsNil)
			c.Expect(string(record), gs.Equals, string(b[139:206]))
			n, record, err = sRunner.GetRecordFromStream(reader) // trigger the need to read more data
			c.Expect(n, gs.Equals, 5)
			c.Expect(err, gs.IsNil)
			c.Expect(len(record), gs.Equals, 0)
			n, record, err = sRunner.GetRecordFromStream(reader) // hit the EOF
			c.Expect(n, gs.Equals, 0)
			c.Expect(err, gs.Equals, io.EOF)
			c.Expect(len(record), gs.Equals, 0)
		})

		c.Specify("correctly handles invalid header (no unit separator)", func() {
			b := []byte("\x1e\x02\x08\x3e\xff\x1e\x02\x08\x3e\x1f\x0a\x10\x90\x1d\x56\x27\xec\x49\x4c\x8f\xba\x8e\x84\x9b\xaa\xf7\xa6\xf6\x10\xa6\x97\x8a\x8f\xb6\xc1\xae\x8e\x13\x1a\x09\x68\x65\x6b\x61\x62\x65\x6e\x63\x68\x28\x06\x3a\x03\x30\x2e\x38\x40\xbf\xe5\x01\x4a\x0a\x74\x72\x69\x6e\x6b\x2d\x78\x32\x33\x30")
			reader := bytes.NewReader(b)
			err := splitter.Init(config)
			c.Assume(err, gs.IsNil)

			n, record, err := sRunner.GetRecordFromStream(reader)
			c.Expect(n, gs.Equals, 72)
			c.Expect(err, gs.IsNil)
			c.Expect(string(record), gs.Equals, string(b[5:]))
		})

		c.Specify("using authentication", func() {
			key := "testkey"
			config.Signers = map[string]Signer{"test_1": {key}}
			signer := "test"
			recycleChan := make(chan *PipelinePack, 1)
			pack := NewPipelinePack(recycleChan)
			msg := ts.GetTestMessage()
			mbytes, _ := proto.Marshal(msg)
			header := &message.Header{}
			header.SetMessageLength(uint32(len(mbytes)))

			c.Specify("authenticates MD5 signed message", func() {
				err := splitter.Init(config)
				c.Assume(err, gs.IsNil)

				header.SetHmacHashFunction(message.Header_MD5)
				header.SetHmacSigner(signer)
				header.SetHmacKeyVersion(uint32(1))
				hm := hmac.New(md5.New, []byte(key))
				hm.Write(mbytes)
				header.SetHmac(hm.Sum(nil))
				hbytes, _ := proto.Marshal(header)

				framed := encodeMessage(hbytes, mbytes)
				unframed := splitter.UnframeRecord(framed, pack)
				c.Expect(pack.Signer, gs.Equals, "test")
				c.Expect(string(unframed), gs.Equals, string(mbytes))
			})

			c.Specify("authenticates SHA1 signed message", func() {
				err := splitter.Init(config)
				c.Assume(err, gs.IsNil)

				header.SetHmacHashFunction(message.Header_SHA1)
				header.SetHmacSigner(signer)
				header.SetHmacKeyVersion(uint32(1))
				hm := hmac.New(sha1.New, []byte(key))
				hm.Write(mbytes)
				header.SetHmac(hm.Sum(nil))
				hbytes, _ := proto.Marshal(header)

				framed := encodeMessage(hbytes, mbytes)
				unframed := splitter.UnframeRecord(framed, pack)
				c.Expect(pack.Signer, gs.Equals, "test")
				c.Expect(string(unframed), gs.Equals, string(mbytes))
			})

			c.Specify("doesn't auth signed message with expired key", func() {
				err := splitter.Init(config)
				c.Assume(err, gs.IsNil)

				header.SetHmacHashFunction(message.Header_MD5)
				header.SetHmacSigner(signer)
				header.SetHmacKeyVersion(uint32(11)) // non-existent key version
				hm := hmac.New(md5.New, []byte(key))
				hm.Write(mbytes)
				header.SetHmac(hm.Sum(nil))
				hbytes, _ := proto.Marshal(header)

				framed := encodeMessage(hbytes, mbytes)
				unframed := splitter.UnframeRecord(framed, pack)
				c.Expect(pack.Signer, gs.Equals, "")
				// The function returns nil, and `unframed == nil` evaluates
				// to true, but `gs.IsNil` doesn't work here.
				c.Expect(string(unframed), gs.Equals, "")
			})

			c.Specify("doesn't auth signed message with incorrect hmac", func() {
				err := splitter.Init(config)
				c.Assume(err, gs.IsNil)

				header.SetHmacHashFunction(message.Header_MD5)
				header.SetHmacSigner(signer)
				header.SetHmacKeyVersion(uint32(1))
				hm := hmac.New(md5.New, []byte(key))
				hm.Write([]byte("some bytes"))
				header.SetHmac(hm.Sum(nil))
				hbytes, _ := proto.Marshal(header)

				framed := encodeMessage(hbytes, mbytes)
				unframed := splitter.UnframeRecord(framed, pack)
				c.Expect(pack.Signer, gs.Equals, "")
				// The function returns nil, and `unframed == nil` evaluates
				// to true, but `gs.IsNil` doesn't work here.
				c.Expect(string(unframed), gs.Equals, "")
			})
		})
	})
}
