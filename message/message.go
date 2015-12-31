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
#
# ***** END LICENSE BLOCK *****/

// Extensions to make Message more useable in our current code outside the scope
// of protocol buffers.  See message.pb.go for the actually message definition.

/*

Internal message representation.

*/
package message

import (
	"bytes"
	"fmt"
	"reflect"

	"github.com/gogo/protobuf/proto"
)

const (
	HEADER_DELIMITER_SIZE = 2                         // record separator + len
	HEADER_FRAMING_SIZE   = HEADER_DELIMITER_SIZE + 1 // unit separator
	MAX_HEADER_SIZE       = 255
	RECORD_SEPARATOR      = uint8(0x1e)
	UNIT_SEPARATOR        = uint8(0x1f)
	UUID_SIZE             = 16
)

var (
	MAX_MESSAGE_SIZE = uint32(64 * 1024)
	MAX_RECORD_SIZE  = uint32(HEADER_FRAMING_SIZE + MAX_HEADER_SIZE + MAX_MESSAGE_SIZE)
)

func SetMaxMessageSize(size uint32) {
	MAX_MESSAGE_SIZE = size
	MAX_RECORD_SIZE = uint32(HEADER_FRAMING_SIZE + MAX_HEADER_SIZE + MAX_MESSAGE_SIZE)
}

type MessageSigningConfig struct {
	Name    string `toml:"name"`
	Hash    string `toml:"hmac_hash"`
	Key     string `toml:"hmac_key"`
	Version uint32 `toml:"version"`
}

// Decodes provided byte slice into a Heka protocol header object.
func DecodeHeader(buf []byte, header *Header) (bool, error) {
	if buf[len(buf)-1] != UNIT_SEPARATOR {
		return false, nil
	}
	err := proto.Unmarshal(buf[0:len(buf)-1], header)
	if err != nil {
		return false, fmt.Errorf("error unmarshaling header: %s", err)
	}
	if header.GetMessageLength() > MAX_MESSAGE_SIZE {
		err = fmt.Errorf("message exceeds the maximum length [%d bytes] len: %d",
			MAX_MESSAGE_SIZE, header.GetMessageLength())
		header.Reset()
		return false, err
	}
	return true, nil
}

func (h *Header) SetMessageLength(v uint32) {
	if h != nil {
		if h.MessageLength == nil {
			h.MessageLength = new(uint32)
		}
		*h.MessageLength = v
	}
}

func (h *Header) SetHmacHashFunction(v Header_HmacHashFunction) {
	if h != nil {
		if h.HmacHashFunction == nil {
			h.HmacHashFunction = new(Header_HmacHashFunction)
		}
		*h.HmacHashFunction = v
	}
}

func (h *Header) SetHmacSigner(v string) {
	if h != nil {
		h.HmacSigner = &v
	}
}

func (h *Header) SetHmacKeyVersion(v uint32) {
	if h != nil {
		if h.HmacKeyVersion == nil {
			h.HmacKeyVersion = new(uint32)
		}
		*h.HmacKeyVersion = v
	}
}

func (h *Header) SetHmac(v []byte) {
	if h != nil {
		if cap(h.Hmac) < len(v) {
			h.Hmac = make([]byte, len(v))
		}
		copy(h.Hmac, v)
	}
}

func (m *Message) SetUuid(v []byte) {
	if m != nil {
		if cap(m.Uuid) != UUID_SIZE {
			m.Uuid = make([]byte, UUID_SIZE)
		}
		copy(m.Uuid, v)
	}
}

func (m *Message) SetTimestamp(v int64) {
	if m != nil {
		if m.Timestamp == nil {
			m.Timestamp = new(int64)
		}
		*m.Timestamp = v
	}
}

func (m *Message) SetType(v string) {
	if m != nil {
		m.Type = &v
	}
}

func (m *Message) SetLogger(v string) {
	if m != nil {
		m.Logger = &v
	}
}

func (m *Message) SetSeverity(v int32) {
	if m != nil {
		if m.Severity == nil {
			m.Severity = new(int32)
		}
		*m.Severity = v
	}
}

func (m *Message) SetPayload(v string) {
	if m != nil {
		m.Payload = &v
	}
}

func (m *Message) SetEnvVersion(v string) {
	if m != nil {
		m.EnvVersion = &v
	}
}

func (m *Message) SetPid(v int32) {
	if m != nil {
		if m.Pid == nil {
			m.Pid = new(int32)
		}
		*m.Pid = v
	}
}

func (m *Message) SetHostname(v string) {
	if m != nil {
		m.Hostname = &v
	}
}

// Message assignment operator
func (src *Message) Copy(dst *Message) {
	if src == nil || dst == nil || src == dst {
		return
	}

	if cap(src.Uuid) > 0 {
		dst.SetUuid(src.Uuid)
	} else {
		dst.Uuid = nil
	}
	if src.Timestamp != nil {
		dst.SetTimestamp(*src.Timestamp)
	} else {
		dst.Timestamp = nil
	}
	if src.Type != nil {
		dst.SetType(*src.Type)
	} else {
		dst.Type = nil
	}
	if src.Logger != nil {
		dst.SetLogger(*src.Logger)
	} else {
		dst.Logger = nil
	}
	if src.Severity != nil {
		dst.SetSeverity(*src.Severity)
	} else {
		dst.Severity = nil
	}
	if src.Payload != nil {
		dst.SetPayload(*src.Payload)
	} else {
		dst.Payload = nil
	}
	if src.EnvVersion != nil {
		dst.SetEnvVersion(*src.EnvVersion)
	} else {
		dst.EnvVersion = nil
	}
	if src.Pid != nil {
		dst.SetPid(*src.Pid)
	} else {
		dst.Pid = nil
	}
	if src.Hostname != nil {
		dst.SetHostname(*src.Hostname)
	} else {
		dst.Hostname = nil
	}
	dst.Fields = make([]*Field, len(src.Fields))
	for i, v := range src.Fields {
		dst.Fields[i] = CopyField(v)
	}
	// ignore XXX_unrecognized
}

// Message copy constructor
func CopyMessage(src *Message) *Message {
	if src == nil {
		return nil
	}
	dst := &Message{}
	src.Copy(dst)
	return dst
}

func getValueType(v reflect.Value) (t Field_ValueType, err error) {
	switch v.Kind() {
	case reflect.String:
		t = Field_STRING
	case reflect.Array, reflect.Slice:
		if v.Type().Elem().Kind() == reflect.Uint8 {
			t = Field_BYTES
		}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		t = Field_INTEGER
	case reflect.Float32, reflect.Float64:
		t = Field_DOUBLE
	case reflect.Bool:
		t = Field_BOOL
	default:
		err = fmt.Errorf("unsupported value kind: %v type: %v", v.Kind(), v.Type())
	}
	return
}

// Adds a Field (name/value) pair to the message
func (m *Message) AddField(f *Field) {
	if m == nil {
		return
	}
	l := len(m.Fields)
	c := cap(m.Fields)
	if l == c {
		tmp := make([]*Field, l+1, c*2+1)
		copy(tmp, m.Fields)
		m.Fields = tmp
	} else {
		m.Fields = m.Fields[0 : l+1]
	}
	m.Fields[l] = f
}

// Deletes a Field from the message
func (m *Message) DeleteField(f *Field) {
	if m == nil {
		return
	}
	for i, v := range m.Fields {
		if v == f {
			m.Fields = append(m.Fields[:i], m.Fields[i+1:]...)
			break
		}
	}
}

// Field constructor
func NewField(name string, value interface{}, representation string) (f *Field, err error) {
	v := reflect.ValueOf(value)
	t, err := getValueType(v)
	if err == nil {
		f = NewFieldInit(name, t, representation)
		f.AddValue(value)
	}
	return
}

// Field initializer sets up the key, value type, and format but does not actually add a value
func NewFieldInit(name string, valueType Field_ValueType, representation string) *Field {
	f := &Field{}
	f.Name = new(string)
	*f.Name = name

	f.ValueType = new(Field_ValueType)
	*f.ValueType = valueType

	f.Representation = new(string)
	*f.Representation = representation

	return f
}

// Creates an array of values in this field, of the same type, in the order they were added
func (f *Field) AddValue(value interface{}) error {
	if f == nil {
		return fmt.Errorf("Field is nil")
	}
	v := reflect.ValueOf(value)
	t, err := getValueType(v)
	if err != nil {
		return err
	}
	if t != f.GetValueType() {
		return fmt.Errorf("The field contains: %v; attempted to add %v",
			Field_ValueType_name[int32(f.GetValueType())], Field_ValueType_name[int32(t)])
	}

	switch f.GetValueType() {
	case Field_STRING:
		l := len(f.ValueString)
		c := cap(f.ValueString)
		if l == c {
			tmp := make([]string, l+1, c*2+1)
			copy(tmp, f.ValueString)
			f.ValueString = tmp
		} else {
			f.ValueString = f.ValueString[0 : l+1]
		}
		f.ValueString[l] = v.String()
	case Field_BYTES:
		l := len(f.ValueBytes)
		c := cap(f.ValueBytes)
		if l == c {
			tmp := make([][]byte, l+1, c*2+1)
			copy(tmp, f.ValueBytes)
			f.ValueBytes = tmp
		} else {
			f.ValueBytes = f.ValueBytes[0 : l+1]
		}
		b := v.Bytes()
		f.ValueBytes[l] = make([]byte, len(b))
		copy(f.ValueBytes[l], b)
	case Field_INTEGER:
		l := len(f.ValueInteger)
		c := cap(f.ValueInteger)
		if l == c {
			tmp := make([]int64, l+1, c*2+1)
			copy(tmp, f.ValueInteger)
			f.ValueInteger = tmp
		} else {
			f.ValueInteger = f.ValueInteger[0 : l+1]
		}
		f.ValueInteger[l] = v.Int()
	case Field_DOUBLE:
		l := len(f.ValueDouble)
		c := cap(f.ValueDouble)
		if l == c {
			tmp := make([]float64, l+1, c*2+1)
			copy(tmp, f.ValueDouble)
			f.ValueDouble = tmp
		} else {
			f.ValueDouble = f.ValueDouble[0 : l+1]
		}
		f.ValueDouble[l] = v.Float()
	case Field_BOOL:
		l := len(f.ValueBool)
		c := cap(f.ValueBool)
		if l == c {
			tmp := make([]bool, l+1, c*2+1)
			copy(tmp, f.ValueBool)
			f.ValueBool = tmp
		} else {
			f.ValueBool = f.ValueBool[0 : l+1]
		}
		f.ValueBool[l] = v.Bool()
	}
	return nil
}

// Helper function that returns the appropriate value object.
func (f *Field) GetValue() (value interface{}) {
	switch f.GetValueType() {
	case Field_STRING:
		if len(f.ValueString) > 0 {
			value = f.ValueString[0]
		}
	case Field_BYTES:
		if len(f.ValueBytes) > 0 {
			value = f.ValueBytes[0]
		}
	case Field_INTEGER:
		if len(f.ValueInteger) > 0 {
			value = f.ValueInteger[0]
		}
	case Field_DOUBLE:
		if len(f.ValueDouble) > 0 {
			value = f.ValueDouble[0]
		}
	case Field_BOOL:
		if len(f.ValueBool) > 0 {
			value = f.ValueBool[0]
		}
	}
	return
}

// Field copy constructor
func CopyField(src *Field) *Field {
	if src == nil {
		return nil
	}

	dst := &Field{}
	if src.Name != nil {
		dst.Name = new(string)
		*dst.Name = *src.Name
	}
	if src.ValueType != nil {
		dst.ValueType = new(Field_ValueType)
		*dst.ValueType = *src.ValueType
	}
	if src.Representation != nil {
		dst.Representation = new(string)
		*dst.Representation = *src.Representation
	}

	if src.ValueString != nil {
		dst.ValueString = make([]string, len(src.ValueString))
		copy(dst.ValueString, src.ValueString)
	}
	if src.ValueBytes != nil {
		dst.ValueBytes = make([][]byte, len(src.ValueBytes))
		copy(dst.ValueBytes, src.ValueBytes)
	}
	if src.ValueInteger != nil {
		dst.ValueInteger = make([]int64, len(src.ValueInteger))
		copy(dst.ValueInteger, src.ValueInteger)
	}
	if src.ValueDouble != nil {
		dst.ValueDouble = make([]float64, len(src.ValueDouble))
		copy(dst.ValueDouble, src.ValueDouble)
	}
	if src.ValueBool != nil {
		dst.ValueBool = make([]bool, len(src.ValueBool))
		copy(dst.ValueBool, src.ValueBool)
	}
	return dst
}

// FindFirstField finds and returns the first field with the specified name
// if not found nil is returned
func (m *Message) FindFirstField(name string) *Field {
	if m == nil {
		return nil
	}
	for _, v := range m.Fields {
		if v != nil && v.GetName() == name {
			return v
		}
	}
	return nil
}

// GetFieldValue helper function to simplify extracting single value fields
func (m *Message) GetFieldValue(name string) (value interface{}, ok bool) {
	if m == nil {
		return
	}
	f := m.FindFirstField(name)
	if f == nil {
		return
	}
	return f.GetValue(), true
}

// FindAllFields finds and returns all the fields with the specified name
// if not found a nil slice is returned
func (m *Message) FindAllFields(name string) (all []*Field) {
	if m == nil {
		return
	}
	for _, v := range m.Fields {
		if v != nil && v.GetName() == name {
			l := len(all)
			c := cap(all)
			if l == c {
				tmp := make([]*Field, l+1, c*2+1)
				copy(tmp, all)
				all = tmp
			} else {
				all = all[0 : l+1]
			}
			all[l] = v
		}
	}
	return
}

// Test for message equality, for use in tests.
func (m *Message) Equals(other interface{}) bool {
	vSelf := reflect.ValueOf(m).Elem()
	vOther := reflect.ValueOf(other).Elem()

	var sField, oField reflect.Value
	for i := 0; i < vSelf.NumField(); i++ {
		sField = vSelf.Field(i)
		oField = vOther.Field(i)
		switch i {
		case 0: // uuid
			if !bytes.Equal(sField.Bytes(), oField.Bytes()) {
				return false
			}
		case 1, 2, 3, 4, 5, 6, 7, 8:
			if sField.Kind() == reflect.Ptr {
				if sField.IsNil() || oField.IsNil() {
					if !(sField.IsNil() && oField.IsNil()) {
						return false
					}
				} else {
					s := reflect.Indirect(sField)
					o := reflect.Indirect(oField)
					if s.Interface() != o.Interface() {
						return false
					}
				}
			} else {
				if sField.Interface() != oField.Interface() {
					return false
				}
			}
		case 9: // Fields
			if !reflect.DeepEqual(sField.Interface(), oField.Interface()) {
				return false
			}
		case 10: // XXX_unrecognized
			// ignore
		}
	}
	return true
}

func (m *Message) GetUuidString() string {
	if m != nil {
		if len(m.Uuid) == UUID_SIZE {
			return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", m.Uuid[:4],
				m.Uuid[4:6], m.Uuid[6:8], m.Uuid[8:10], m.Uuid[10:])
		}
	}
	return ""
}

// Convenience function for creating a new integer field on a message object.
func NewIntField(m *Message, name string, val int, representation string) (err error) {
	if f, err := NewField(name, val, representation); err == nil {
		m.AddField(f)
	}
	return
}

// Convenience function for creating a new int64 field on a message object.
func NewInt64Field(m *Message, name string, val int64, representation string) {
	if f, err := NewField(name, val, representation); err == nil {
		m.AddField(f)
	}
	return
}

// Convenience function for creating and setting a string field called "name"
// on a message object.
func NewStringField(m *Message, name string, val string) {
	if f, err := NewField(name, val, ""); err == nil {
		m.AddField(f)
	}
	return
}
