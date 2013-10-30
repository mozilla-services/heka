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
#   Mike Trinkala (trink@mozilla.com)
#
# ***** END LICENSE BLOCK *****/

package pipeline

import (
	"bytes"
	"github.com/mozilla-services/heka/message"
	gs "github.com/rafrombrc/gospec/src/gospec"
	"io"
)

func StreamParserSpec(c gs.Context) {
	buf := []byte("test1\ntest12\ntest123\npartial")
	c.Specify("token parser default delimiter", func() {
		reader := bytes.NewReader(buf)
		p := NewTokenParser()
		n, record, err := p.Parse(reader)
		c.Expect(n, gs.Equals, 6)
		c.Expect(err, gs.IsNil)
		c.Expect(string(record), gs.Equals, "test1\n")
		n, record, err = p.Parse(reader)
		c.Expect(n, gs.Equals, 7)
		c.Expect(err, gs.IsNil)
		c.Expect(string(record), gs.Equals, "test12\n")
		n, record, err = p.Parse(reader)
		c.Expect(n, gs.Equals, 8)
		c.Expect(err, gs.IsNil)
		c.Expect(string(record), gs.Equals, "test123\n")
		n, record, err = p.Parse(reader)
		c.Expect(n, gs.Equals, 0)
		c.Expect(err, gs.IsNil)
		c.Expect(string(p.GetRemainingData()), gs.Equals, "partial")
	})

	c.Specify("token parser tab delimiter", func() {
		reader := bytes.NewReader([]byte("test1\ttest2\t"))
		p := NewTokenParser()
		p.SetDelimiter('\t')
		n, record, err := p.Parse(reader)
		c.Expect(n, gs.Equals, 6)
		c.Expect(err, gs.IsNil)
		c.Expect(string(record), gs.Equals, "test1\t")
		n, record, err = p.Parse(reader)
		c.Expect(n, gs.Equals, 6)
		c.Expect(err, gs.IsNil)
		c.Expect(string(record), gs.Equals, "test2\t")
	})

	c.Specify("regexp parser invalid delimiter", func() {
		p := NewRegexpParser()
		err := p.SetDelimiter("\\y")
		c.Expect(err.Error(), gs.Equals, "error parsing regexp: invalid escape sequence: `\\y`")
		err = p.SetDelimiter("(\\r)(\\n)")
		c.Expect(err.Error(), gs.Equals, "the regexp must not contain more than one capture group: (\\r)(\\n)")
	})

	c.Specify("regexp parser invalid delimiter location", func() {
		p := NewRegexpParser()
		err := p.SetDelimiterLocation("middle")
		c.Expect(err.Error(), gs.Equals, "unknown delimiter location: middle")
	})

	c.Specify("regexp parser (no capture)", func() {
		reader := bytes.NewReader(buf)
		p := NewRegexpParser()

		n, record, err := p.Parse(reader)
		c.Expect(n, gs.Equals, 6)
		c.Expect(err, gs.IsNil)
		c.Expect(string(record), gs.Equals, "test1")
		n, record, err = p.Parse(reader)
		c.Expect(n, gs.Equals, 7)
		c.Expect(err, gs.IsNil)
		c.Expect(string(record), gs.Equals, "test12")
		n, record, err = p.Parse(reader)
		c.Expect(n, gs.Equals, 8)
		c.Expect(err, gs.IsNil)
		c.Expect(string(record), gs.Equals, "test123")
		n, record, err = p.Parse(reader)
		c.Expect(n, gs.Equals, 0)
		c.Expect(err, gs.IsNil)
		c.Expect(string(p.GetRemainingData()), gs.Equals, "partial")
	})

	c.Specify("regexp parser (start delimiter capture)", func() {
		reader := bytes.NewReader(buf)
		p := NewRegexpParser()
		p.SetDelimiter("(\n)")
		p.SetDelimiterLocation("start")

		n, record, err := p.Parse(reader)
		c.Expect(n, gs.Equals, 5)
		c.Expect(err, gs.IsNil)
		c.Expect(string(record), gs.Equals, "test1")
		n, record, err = p.Parse(reader)
		c.Expect(n, gs.Equals, 7)
		c.Expect(err, gs.IsNil)
		c.Expect(string(record), gs.Equals, "\ntest12")
		n, record, err = p.Parse(reader)
		c.Expect(n, gs.Equals, 8)
		c.Expect(err, gs.IsNil)
		c.Expect(string(record), gs.Equals, "\ntest123")
		n, record, err = p.Parse(reader)
		c.Expect(n, gs.Equals, 0)
		c.Expect(err, gs.IsNil)
		c.Expect(string(p.GetRemainingData()), gs.Equals, "\npartial")
	})

	c.Specify("regexp parser (start delimiter capture single record)", func() {
		reader := bytes.NewReader([]byte("\ntest"))
		p := NewRegexpParser()
		p.SetDelimiter("(\n)")
		p.SetDelimiterLocation("start")

		n, record, err := p.Parse(reader)
		c.Expect(n, gs.Equals, 0)
		c.Expect(len(record), gs.Equals, 0)
		c.Expect(err, gs.IsNil)
		c.Expect(string(p.GetRemainingData()), gs.Equals, "\ntest")
	})

	c.Specify("regexp parser (start delimiter no capture single record)", func() {
		reader := bytes.NewReader([]byte("\ntest"))
		p := NewRegexpParser()
		p.SetDelimiter("\n")
		p.SetDelimiterLocation("start")

		n, record, err := p.Parse(reader)
		c.Expect(n, gs.Equals, 1) // moves past the delimiter since it is not captured as part of the record
		c.Expect(err, gs.IsNil)
		c.Expect(len(record), gs.Equals, 0) // consume the empty record in front of the delimiter
		n, record, err = p.Parse(reader)
		c.Expect(n, gs.Equals, 0)
		c.Expect(len(record), gs.Equals, 0)
		c.Expect(err, gs.Equals, io.EOF)
		c.Expect(string(p.GetRemainingData()), gs.Equals, "test")
	})

	c.Specify("regexp parser (capture)", func() {
		reader := bytes.NewReader(buf)
		p := NewRegexpParser()
		p.SetDelimiter("(\n)")

		n, record, err := p.Parse(reader)
		c.Expect(n, gs.Equals, 6)
		c.Expect(err, gs.IsNil)
		c.Expect(string(record), gs.Equals, "test1\n")
		n, record, err = p.Parse(reader)
		c.Expect(n, gs.Equals, 7)
		c.Expect(err, gs.IsNil)
		c.Expect(string(record), gs.Equals, "test12\n")
		n, record, err = p.Parse(reader)
		c.Expect(n, gs.Equals, 8)
		c.Expect(err, gs.IsNil)
		c.Expect(string(record), gs.Equals, "test123\n")
		n, record, err = p.Parse(reader)
		c.Expect(n, gs.Equals, 0)
		c.Expect(err, gs.IsNil)
		c.Expect(string(p.GetRemainingData()), gs.Equals, "partial")
	})

	c.Specify("message.proto parser", func() {
		b := []byte("\x1e\x02\x08\x3e\x1f\x0a\x10\x90\x1d\x56\x27\xec\x49\x4c\x8f\xba\x8e\x84\x9b\xaa\xf7\xa6\xf6\x10\xa6\x97\x8a\x8f\xb6\xc1\xae\x8e\x13\x1a\x09\x68\x65\x6b\x61\x62\x65\x6e\x63\x68\x28\x06\x3a\x03\x30\x2e\x38\x40\xbf\xe5\x01\x4a\x0a\x74\x72\x69\x6e\x6b\x2d\x78\x32\x33\x30\x1e\x02\x08\x3e\x1f\x0a\x10\x90\x1d\x56\x27\xec\x49\x4c\x8f\xba\x8e\x84\x9b\xaa\xf7\xa6\xf6\x10\xa6\x97\x8a\x8f\xb6\xc1\xae\x8e\x13\x1a\x09\x68\x65\x6b\x61\x62\x65\x6e\x63\x68\x28\x06\x3a\x03\x30\x2e\x38\x40\xbf\xe5\x01\x4a\x0a\x74\x72\x69\x6e\x6b\x2d\x78\x32\x33\x30BOGUS\x1e\x02\x08\x3e\x1f\x0a\x10\x90\x1d\x56\x27\xec\x49\x4c\x8f\xba\x8e\x84\x9b\xaa\xf7\xa6\xf6\x10\xa6\x97\x8a\x8f\xb6\xc1\xae\x8e\x13\x1a\x09\x68\x65\x6b\x61\x62\x65\x6e\x63\x68\x28\x06\x3a\x03\x30\x2e\x38\x40\xbf\xe5\x01\x4a\x0a\x74\x72\x69\x6e\x6b\x2d\x78\x32\x33\x30BOGUS\x1e\x02\x08")
		reader := bytes.NewReader(b)
		p := NewMessageProtoParser()

		n, record, err := p.Parse(reader)
		c.Expect(n, gs.Equals, 67)
		c.Expect(err, gs.IsNil)
		c.Expect(string(record), gs.Equals, string(b[:67]))
		n, record, err = p.Parse(reader)
		c.Expect(n, gs.Equals, 67)
		c.Expect(err, gs.IsNil)
		c.Expect(string(record), gs.Equals, string(b[67:134]))
		n, record, err = p.Parse(reader)
		c.Expect(n, gs.Equals, 72) // skips the invalid 'BOGUS' data
		c.Expect(err, gs.IsNil)
		c.Expect(string(record), gs.Equals, string(b[139:206]))
		n, record, err = p.Parse(reader) // trigger the need to read more data
		c.Expect(n, gs.Equals, 5)
		c.Expect(err, gs.IsNil)
		c.Expect(len(record), gs.Equals, 0)
		n, record, err = p.Parse(reader) // hit the EOF
		c.Expect(n, gs.Equals, 0)
		c.Expect(err, gs.Equals, io.EOF)
		c.Expect(len(record), gs.Equals, 0)
	})

	c.Specify("message.proto parser invalid header (no unit separator)", func() {
		b := []byte("\x1e\x02\x08\x3e\xff\x1e\x02\x08\x3e\x1f\x0a\x10\x90\x1d\x56\x27\xec\x49\x4c\x8f\xba\x8e\x84\x9b\xaa\xf7\xa6\xf6\x10\xa6\x97\x8a\x8f\xb6\xc1\xae\x8e\x13\x1a\x09\x68\x65\x6b\x61\x62\x65\x6e\x63\x68\x28\x06\x3a\x03\x30\x2e\x38\x40\xbf\xe5\x01\x4a\x0a\x74\x72\x69\x6e\x6b\x2d\x78\x32\x33\x30")
		reader := bytes.NewReader(b)
		p := NewMessageProtoParser()

		n, record, err := p.Parse(reader)
		c.Expect(n, gs.Equals, 72)
		c.Expect(err, gs.IsNil)
		c.Expect(string(record), gs.Equals, string(b[5:]))
	})

	c.Specify("max record size", func() {
		b := make([]byte, message.MAX_RECORD_SIZE)
		b[message.MAX_RECORD_SIZE-1] = '\t'
		reader := bytes.NewReader(b)
		p := NewTokenParser()
		p.SetDelimiter('\t')
		var n int
		var record []byte
		var err error
		for err == nil && len(record) == 0 {
			n, record, err = p.Parse(reader)
		}
		c.Expect(n, gs.Equals, message.MAX_RECORD_SIZE)
		c.Expect(string(record), gs.Equals, string(b))
		c.Expect(err, gs.IsNil)
	})

	c.Specify("exceed max record size", func() {
		b := make([]byte, message.MAX_RECORD_SIZE+1)
		reader := bytes.NewReader(b)
		p := NewTokenParser()
		p.SetDelimiter('\t')
		var n int
		var record []byte
		var err error
		for err == nil {
			n, record, err = p.Parse(reader)
		}
		c.Expect(n, gs.Equals, message.MAX_RECORD_SIZE)
		c.Expect(len(record), gs.Equals, 0)
		c.Expect(err, gs.Equals, io.ErrShortBuffer)
	})
}
