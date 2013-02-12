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
	"github.com/mozilla-services/heka/message"
)

// FilterSpecification used by the message router to distribute messages
type FilterSpecification struct {
	vm     *tree
	filter string
}

// CreateFilterSpecification compiles the filter string into a simple
// virtual machine for execution
func CreateFilterSpecification(filter string) (*FilterSpecification, error) {
	fs := new(FilterSpecification)
	fs.filter = filter
	err := parseFilterSpecification(fs)
	if err != nil {
		return nil, err
	}
	return fs, nil
}

func evalFilterSpecification(t *tree, msg *message.Message) (b bool) {
	if t == nil {
		return false
	}

	if t.left != nil {
		b = evalFilterSpecification(t.left, msg)
	} else {
		return testExpr(msg, t.stmt)
	}
	if b == true && t.stmt.op.tokenId == OP_OR {
		return // short circuit
	}
	if b == false && t.stmt.op.tokenId == OP_AND {
		return // short circuit
	}

	if t.right != nil {
		b = evalFilterSpecification(t.right, msg)
	}
	return
}

// FilterMsg implements the filter interface needed by the router
func (f *FilterSpecification) FilterMsg(pipelinePack *PipelinePack) {
	rslt := evalFilterSpecification(f.vm, pipelinePack.Message)
	if rslt {
		pipelinePack.Blocked = false
	} else {
		pipelinePack.Blocked = true
	}
}

func getStringValue(msg *message.Message, stmt *Statement) string {
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

func getNumericValue(msg *message.Message, stmt *Statement) float64 {
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

func stringTest(s string, stmt *Statement) bool {
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

func testExpr(msg *message.Message, stmt *Statement) bool {
	switch stmt.op.tokenId {
	case TRUE:
		return true
	case FALSE:
		return false
	default:
		switch stmt.field.tokenId {
		case VAR_UUID, VAR_TYPE, VAR_LOGGER, VAR_PAYLOAD,
			VAR_ENVVERSION, VAR_HOSTNAME:
			return stringTest(getStringValue(msg, stmt), stmt)
		case VAR_TIMESTAMP, VAR_SEVERITY, VAR_PID:
			return numericTest(getNumericValue(msg, stmt), stmt)
		case VAR_FIELDS:
			fi := stmt.field.fieldIndex
			ai := stmt.field.arrayIndex
			fields := msg.FindAllFields(stmt.field.token)
			if fi >= len(fields) {
				return false
			}
			field := fields[fi]
			switch field.GetValueType() {
			case message.Field_STRING:
				if ai >= len(field.ValueString) {
					return false
				}
				return stringTest(field.ValueString[ai], stmt)
			case message.Field_BYTES:
				if ai >= len(field.ValueBytes) {
					return false
				}
				return stringTest(string(field.ValueBytes[ai]), stmt)
			case message.Field_INTEGER:
				if ai >= len(field.ValueInteger) {
					return false
				}
				return numericTest(float64(field.ValueInteger[ai]), stmt)
			case message.Field_DOUBLE:
				if ai >= len(field.ValueDouble) {
					return false
				}
				return numericTest(field.ValueDouble[ai], stmt)
			case message.Field_BOOL:
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
