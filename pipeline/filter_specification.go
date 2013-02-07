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
	vm     []*Statement
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

// FilterMsg implements the filter interface needed by the router
func (f *FilterSpecification) FilterMsg(pipelinePack *PipelinePack) {
	stack := new(Stack)
	trueSym := yySymType{tokenId: TRUE}
	falseSym := yySymType{tokenId: FALSE}
	msg := pipelinePack.Message
	for _, stmt := range f.vm {
		if stmt.op.tokenId != OP_OR && stmt.op.tokenId != OP_AND {
			stack.Push(stmt)
		} else {
			v2 := stack.Pop()
			v1 := stack.Pop()
			switch stmt.op.tokenId {
			case OP_OR:
				if testExpr(msg, v1) || testExpr(msg, v2) {
					stack.Push(&Statement{op: trueSym})
				} else {
					stack.Push(&Statement{op: falseSym})
				}
			case OP_AND:
				if testExpr(msg, v1) && testExpr(msg, v2) {
					stack.Push(&Statement{op: trueSym})
				} else {
					stack.Push(&Statement{op: falseSym})
				}
			}
		}
	}
	rslt := testExpr(msg, stack.Pop())
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

/// @todo replace with an AST so we can perform a short circuit eval instead
/// of a full eval
type Stack struct {
	top  *Item
	size int
}

type Item struct {
	stmt *Statement
	next *Item
}

func (s *Stack) Size() int {
	return s.size
}

func (s *Stack) Push(stmt *Statement) {
	s.top = &Item{stmt, s.top}
	s.size++
}

func (s *Stack) Pop() (stmt *Statement) {
	if s.size > 0 {
		stmt, s.top = s.top.stmt, s.top.next
		s.size--
		return
	}
	return nil
}
