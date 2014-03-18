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
	"fmt"
	"github.com/mozilla-services/heka/message"
	"io"
	"regexp"
)

// StreamParser interface to read a spilt a stream into records
type StreamParser interface {
	// Finds the next record in the stream.
	// Returns the number of bytes read from the stream. This will not always
	// correlate to the record size since delimiters can be discarded and data
	// corruption skipped which also means the record could empty even if bytes
	// were read. 'record' will remain valid until the next call to Parse.
	// bytesRead can be non zero even in an error condition i.e. ErrShortBuffer
	Parse(reader io.Reader) (bytesRead int, record []byte, err error)

	// Retrieves the remainder of the parse buffer.  This is the
	// only way to fetch the last record in a stream that specifies a start of
	// line  delimiter or contains a partial last line.  It should only be
	// called when at the EOF and no additional data will be appended to
	// the stream.
	GetRemainingData() []byte

	// Sets the internal buffer to at least 'size' bytes.
	SetMinimumBufferSize(size int)
}

// Internal buffer management for the StreamParser
type streamParserBuffer struct {
	buf      []byte
	readPos  int
	scanPos  int
	needData bool
	err      string
}

func newStreamParserBuffer() (s *streamParserBuffer) {
	s = new(streamParserBuffer)
	s.buf = make([]byte, 1024*8)
	s.needData = true
	return
}

func (s *streamParserBuffer) GetRemainingData() (record []byte) {
	if s.readPos-s.scanPos > 0 {
		record = s.buf[s.scanPos:s.readPos]
	}
	s.scanPos = 0
	s.readPos = 0
	return
}

func (s *streamParserBuffer) SetMinimumBufferSize(size int) {
	if cap(s.buf) < size {
		newSlice := make([]byte, size)
		copy(newSlice, s.buf)
		s.buf = newSlice
	}
	return
}

func (s *streamParserBuffer) read(reader io.Reader) (n int, err error) {
	if cap(s.buf)-s.readPos <= 1024*4 {
		if s.scanPos == 0 { // line will not fit in the current buffer
			newSize := cap(s.buf) * 2
			if newSize > message.MAX_RECORD_SIZE {
				if cap(s.buf) == message.MAX_RECORD_SIZE {
					if s.readPos == cap(s.buf) {
						s.scanPos = 0
						s.readPos = 0
						return cap(s.buf), io.ErrShortBuffer
					} else {
						newSize = 0 // don't allocate any more memory, just read into what is left
					}
				} else {
					newSize = message.MAX_RECORD_SIZE
				}
			}
			if newSize > 0 {
				s.SetMinimumBufferSize(newSize)
			}
		} else { // reclaim the space at the beginning of the buffer
			copy(s.buf, s.buf[s.scanPos:s.readPos])
			s.readPos, s.scanPos = s.readPos-s.scanPos, 0
		}
	}
	n, err = reader.Read(s.buf[s.readPos:])
	return
}

// Byte delimited line parser
type TokenParser struct {
	*streamParserBuffer
	delimiter byte
}

func NewTokenParser() (t *TokenParser) {
	t = new(TokenParser)
	t.streamParserBuffer = newStreamParserBuffer()
	t.delimiter = '\n'
	return
}

func (t *TokenParser) Parse(reader io.Reader) (bytesRead int, record []byte, err error) {
	if t.needData {
		if bytesRead, err = t.read(reader); err != nil {
			return
		}
	}
	t.readPos += bytesRead

	bytesRead, record = t.findRecord(t.buf[t.scanPos:t.readPos])
	t.scanPos += bytesRead
	if len(record) == 0 {
		t.needData = true
	} else {
		if t.readPos == t.scanPos {
			t.readPos = 0
			t.scanPos = 0
			t.needData = true
		} else {
			t.needData = false
		}
	}
	return
}

// Sets the byte delimiter to parse on, defaults to a newline.
func (t *TokenParser) SetDelimiter(delim byte) {
	t.delimiter = delim
}

func (t *TokenParser) findRecord(buf []byte) (bytesRead int, record []byte) {
	n := bytes.IndexByte(buf, t.delimiter)
	if n == -1 {
		return
	}
	bytesRead = n + 1 // include the delimiter for backwards compatibility
	record = buf[:bytesRead]
	return
}

// Regexp line parser using a start or end of line regexp delimiter
type RegexpParser struct {
	*streamParserBuffer
	delimiter    *regexp.Regexp
	delimiterEol bool
	captureLen   int
}

func NewRegexpParser() (r *RegexpParser) {
	r = new(RegexpParser)
	r.streamParserBuffer = newStreamParserBuffer()
	r.delimiter = regexp.MustCompile("\n")
	r.delimiterEol = true
	return
}

// Sets the regex delimiter to parse on, defaults to a newline.  A single
// capture group is allowed and the capture will be included in the parsed
// data according to the DelimiterLocation.
func (r *RegexpParser) SetDelimiter(delim string) (err error) {
	if r.delimiter, err = regexp.Compile(delim); err != nil {
		return
	}
	if r.delimiter.NumSubexp() > 1 {
		return fmt.Errorf("the regexp must not contain more than one capture group: %s", delim)
	}
	return
}

// Specifies whether the delimiter occurs at the 'start' or 'end' of the line,
// defaults to 'end'.
func (r *RegexpParser) SetDelimiterLocation(location string) (err error) {
	if location == "start" {
		r.delimiterEol = false
	} else if location == "" || location == "end" {
		r.delimiterEol = true
	} else {
		return fmt.Errorf("unknown delimiter location: %s", location)
	}
	return
}

func (r *RegexpParser) Parse(reader io.Reader) (bytesRead int, record []byte, err error) {
	if r.needData {
		if bytesRead, err = r.read(reader); err != nil {
			return
		}
	}
	r.readPos += bytesRead

	bytesRead, record = r.findRecord(r.buf[r.scanPos:r.readPos])
	r.scanPos += bytesRead
	if len(record) == 0 {
		r.needData = true
	} else {
		if r.readPos == r.scanPos {
			r.readPos = 0
			r.scanPos = 0
			r.needData = true
		} else {
			r.needData = false
		}
	}
	return
}

func (r *RegexpParser) findRecord(buf []byte) (bytesRead int, record []byte) {
	var loc []int
	loc = r.delimiter.FindSubmatchIndex(buf[r.captureLen:])
	if loc == nil {
		return
	}
	if len(loc) == 4 {
		if r.delimiterEol { // append the capture to the end of the previous record
			record = buf[:loc[3]]
			bytesRead = loc[1]
		} else { // append the capture to the beginning of the next record
			record = buf[:loc[0]+r.captureLen]
			bytesRead = loc[3] + r.captureLen
			r.captureLen = loc[3] - loc[2]
			bytesRead -= r.captureLen
		}
	} else { // no capture discard the delimiter
		record = buf[:loc[0]]
		bytesRead = loc[1]
	}
	return
}

// Protobuf record parser
type MessageProtoParser struct {
	*streamParserBuffer
	header *message.Header
}

func NewMessageProtoParser() (m *MessageProtoParser) {
	m = new(MessageProtoParser)
	m.streamParserBuffer = newStreamParserBuffer()
	m.header = new(message.Header)
	return
}

func (m *MessageProtoParser) Parse(reader io.Reader) (bytesRead int, record []byte, err error) {
	if m.needData {
		if bytesRead, err = m.read(reader); err != nil {
			return
		}
	}
	m.readPos += bytesRead

	bytesRead, record = m.findRecord(m.buf[m.scanPos:m.readPos])
	m.scanPos += bytesRead
	if len(record) == 0 {
		m.needData = true
	} else {
		if m.readPos == m.scanPos {
			m.readPos = 0
			m.scanPos = 0
			m.needData = true
		} else {
			m.needData = false
		}
	}
	return
}

func (m *MessageProtoParser) findRecord(buf []byte) (bytesRead int, record []byte) {
	bytesRead = bytes.IndexByte(buf, message.RECORD_SEPARATOR)
	if bytesRead == -1 {
		bytesRead = len(buf)
		return // read more data to find the start of the next message
	}

	if len(buf) < bytesRead+message.HEADER_DELIMITER_SIZE {
		return // read more data to get the header length byte
	}
	headerLength := int(buf[bytesRead+1])
	headerEnd := bytesRead + headerLength + message.HEADER_FRAMING_SIZE
	if len(buf) < headerEnd {
		return // read more data to get the remainder of the header
	}
	if m.header.MessageLength != nil || DecodeHeader(buf[bytesRead+message.HEADER_DELIMITER_SIZE:headerEnd], m.header) {
		messageEnd := headerEnd + int(m.header.GetMessageLength())
		if len(buf) < messageEnd {
			return // read more data to get the remainder of the message
		}
		record = buf[bytesRead:messageEnd]
		bytesRead = messageEnd
		m.header.Reset()
	} else {
		var n int
		bytesRead++                               // advance over the current record separator
		n, record = m.findRecord(buf[bytesRead:]) // header was invalid, look again
		bytesRead += n
	}
	return
}
