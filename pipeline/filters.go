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
#
# ***** END LICENSE BLOCK *****/
package pipeline

import (
	"log"
)

type Filter interface {
	FilterMsg(msg *Message, outputs map[string]bool)
}

type LogFilter struct {
}

func (self *LogFilter) FilterMsg(msg *Message, outputs map[string]bool) {
	log.Printf("Message: %+v\n", *msg)
}

type namedOutputFilter struct {
	outputNames []string
}

func NewNamedOutputFilter(outputNames []string) *namedOutputFilter {
	self := namedOutputFilter{outputNames}
	return &self
}

func (self *namedOutputFilter) FilterMsg(msg *Message, outputs map[string]bool) {
	for _, outputName := range self.outputNames {
		outputs[outputName] = true
	}
}
