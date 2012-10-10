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
package hekagrater

import (
	"log"
)

type Output interface {
	run(inChan <-chan *Message)
}

type OutputRunner struct {
	output Output
}

func (self *OutputRunner) Start() chan *Message {
	inChan := make(chan *Message)
	go self.output.run(inChan)
	return inChan
}

type LogOutput struct {
}

func (self *LogOutput) run(inChan <-chan *Message) {
	for {
		msg := <-inChan
		log.Printf("%+v\n", msg)
	}
}