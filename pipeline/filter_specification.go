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
func CreateFilterSpecification(f string) (*FilterSpecification, error) {
	fs := new(FilterSpecification)
	fs.filter = f
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

func getStringValue(msg *message.Message, vid int) string {
	switch vid {
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

func getNumericValue(msg *message.Message, vid int) float64 {
	switch vid {
	case VAR_TIMESTAMP:
		return float64(msg.GetTimestamp())
	case VAR_SEVERITY:
		return float64(msg.GetSeverity())
	case VAR_PID:
		return float64(msg.GetPid())
	}
	return 0
}

func stringTest(msg *message.Message, n *Statement) bool {
	s := getStringValue(msg, n.field.tokenId)
	switch n.op.tokenId {
	case OP_EQ:
		return (s == n.value.token)
	case OP_NE:
		return (s != n.value.token)
	case OP_LT:
		return (s < n.value.token)
	case OP_LTE:
		return (s <= n.value.token)
	case OP_GT:
		return (s > n.value.token)
	case OP_GTE:
		return (s >= n.value.token)
	}
	return false
}

func numericTest(msg *message.Message, n *Statement) bool {
	s := getNumericValue(msg, n.field.tokenId)
	switch n.op.tokenId {
	case OP_EQ:
		return (s == n.value.double)
	case OP_NE:
		return (s != n.value.double)
	case OP_LT:
		return (s < n.value.double)
	case OP_LTE:
		return (s <= n.value.double)
	case OP_GT:
		return (s > n.value.double)
	case OP_GTE:
		return (s >= n.value.double)
	}
	return false
}

func testExpr(msg *message.Message, n *Statement) bool {
	switch n.op.tokenId {
	case TRUE:
		return true
	case FALSE:
		return false
	default:
		switch n.field.tokenId {
		case VAR_UUID, VAR_TYPE, VAR_LOGGER, VAR_PAYLOAD,
			VAR_ENVVERSION, VAR_HOSTNAME:
			return stringTest(msg, n)
		case VAR_TIMESTAMP, VAR_SEVERITY, VAR_PID:
			return numericTest(msg, n)
		}
	}
	return false
}
