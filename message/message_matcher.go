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

package message

import "fmt"

// MatcherSpecification used by the message router to distribute messages
type MatcherSpecification struct {
	vm         *tree
	spec       string
	numCapture int
}

// CreateMatcherSpecification compiles the spec string into a simple
// virtual machine for execution
func CreateMatcherSpecification(spec string) (*MatcherSpecification, error) {
	ms := new(MatcherSpecification)
	ms.spec = spec
	err := parseMatcherSpecification(ms)
	if err != nil {
		return nil, err
	}
	return ms, nil
}

// Match compares the message against the matcher spec and return the match
// result and captures if applicable
func (m *MatcherSpecification) Match(message *Message) (match bool,
	captures map[string]string) {
	if m.numCapture > 0 {
		captures = make(map[string]string, m.numCapture)
	}
	match = evalMatcherSpecification(m.vm, message, captures)
	if !match {
		captures = nil
	}
	return
}

// String outputs the spec as text
func (m *MatcherSpecification) String() string {
	return m.spec
}

func evalMatcherSpecification(t *tree, msg *Message,
	captures map[string]string) (b bool) {
	if t == nil {
		return false
	}

	if t.left != nil {
		b = evalMatcherSpecification(t.left, msg, captures)
	} else {
		return testExpr(msg, t.stmt, captures)
	}
	if b == true && t.stmt.op.tokenId == OP_OR {
		return // short circuit
	}
	if b == false && t.stmt.op.tokenId == OP_AND {
		return // short circuit
	}

	if t.right != nil {
		b = evalMatcherSpecification(t.right, msg, captures)
	}
	return
}

func getStringValue(msg *Message, stmt *Statement) string {
	switch stmt.field.tokenId {
	case VAR_UUID:
		return msg.GetUuidString()
	case VAR_TYPE:
		return msg.GetType()
	case VAR_LOGGER:
		return msg.GetLogger()
	case VAR_PAYLOAD:
		return msg.GetPayload()
	case VAR_ENVVERSION:
		return msg.GetEnvVersion()
	case VAR_HOSTNAME:
		return msg.GetHostname()
	}
	return ""
}

func getNumericValue(msg *Message, stmt *Statement) float64 {
	switch stmt.field.tokenId {
	case VAR_TIMESTAMP:
		return float64(msg.GetTimestamp())
	case VAR_SEVERITY:
		return float64(msg.GetSeverity())
	case VAR_PID:
		return float64(msg.GetPid())
	}
	return 0
}

func regexpTest(s string, stmt *Statement, captures map[string]string) bool {
	if stmt.value.regexp.NumSubexp() == 0 {
		return stmt.value.regexp.MatchString(s)
	} else {
		findResults := stmt.value.regexp.FindStringSubmatch(s)
		resultLength := len(findResults)
		if resultLength < 2 {
			return false
		}
		for index, name := range stmt.value.regexp.SubexpNames() {
			if index == 0 {
				continue
			}
			if name == "" {
				name = fmt.Sprintf("%s(%d)", stmt.field.token, index)
			}

			if index > resultLength-1 {
				captures[name] = ""
			} else {
				captures[name] = findResults[index]
			}
		}
		return true
	}
	return false
}

func stringTest(s string, stmt *Statement, captures map[string]string) bool {
	switch stmt.op.tokenId {
	case OP_EQ:
		return (s == stmt.value.token)
	case OP_NE:
		return (s != stmt.value.token)
	case OP_LT:
		return (s < stmt.value.token)
	case OP_LTE:
		return (s <= stmt.value.token)
	case OP_GT:
		return (s > stmt.value.token)
	case OP_GTE:
		return (s >= stmt.value.token)
	case OP_RE:
		return regexpTest(s, stmt, captures)
	case OP_NRE:
		return !stmt.value.regexp.MatchString(s)
	}
	return false
}

func numericTest(f float64, stmt *Statement) bool {
	switch stmt.op.tokenId {
	case OP_EQ:
		return (f == stmt.value.double)
	case OP_NE:
		return (f != stmt.value.double)
	case OP_LT:
		return (f < stmt.value.double)
	case OP_LTE:
		return (f <= stmt.value.double)
	case OP_GT:
		return (f > stmt.value.double)
	case OP_GTE:
		return (f >= stmt.value.double)
	}
	return false
}

func testExpr(msg *Message, stmt *Statement, captures map[string]string) bool {
	switch stmt.op.tokenId {
	case TRUE:
		return true
	case FALSE:
		return false
	default:
		switch stmt.field.tokenId {
		case VAR_UUID, VAR_TYPE, VAR_LOGGER, VAR_PAYLOAD,
			VAR_ENVVERSION, VAR_HOSTNAME:
			return stringTest(getStringValue(msg, stmt), stmt, captures)
		case VAR_TIMESTAMP, VAR_SEVERITY, VAR_PID:
			return numericTest(getNumericValue(msg, stmt), stmt)
		case VAR_FIELDS:
			fi := stmt.field.fieldIndex
			ai := stmt.field.arrayIndex
			var field *Field

			if fi != 0 {
				fields := msg.FindAllFields(stmt.field.token)
				if fi >= len(fields) {
					return false
				}
				field = fields[fi]
			} else {
				if field = msg.FindFirstField(stmt.field.token); field == nil {
					return false
				}
			}
			switch field.GetValueType() {
			case Field_STRING:
				if ai >= len(field.ValueString) {
					return false
				}
				return stringTest(field.ValueString[ai], stmt, captures)
			case Field_BYTES:
				if ai >= len(field.ValueBytes) {
					return false
				}
				return stringTest(string(field.ValueBytes[ai]), stmt, captures)
			case Field_INTEGER:
				if ai >= len(field.ValueInteger) {
					return false
				}
				return numericTest(float64(field.ValueInteger[ai]), stmt)
			case Field_DOUBLE:
				if ai >= len(field.ValueDouble) {
					return false
				}
				return numericTest(field.ValueDouble[ai], stmt)
			case Field_BOOL:
				if ai >= len(field.ValueBool) {
					return false
				}
				b := field.ValueBool[ai]
				if stmt.value.tokenId == TRUE {
					return (b == true)
				} else {
					return (b == false)
				}
			}
		}
	}
	return false
}
