/***** BEGIN LICENSE BLOCK *****
# This Source Code Form is subject to the terms of the Mozilla Public
# License, v. 2.0. If a copy of the MPL was not distributed with this file,
# You can obtain one at http://mozilla.org/MPL/2.0/.
#
# The Initial Developer of the Original Code is the Mozilla Foundation.
# Portions created by the Initial Developer are Copyright (C) 2013
# the Initial Developer. All Rights Reserved.
#
# Contributor(s):
#   Rob Miller (rmiller@mozilla.com)
#   Mike Trinkala (trink@mozilla.com)
#
# ***** END LICENSE BLOCK *****/

package pipeline

import (
	"log"
	"runtime"
	"sync/atomic"
)

// Pushes the message onto the filters input channel if it is a match
type MessageRouter struct {
	InChan chan *PipelinePack
}

func (self *MessageRouter) Start() {
	go func() {
		log.Println("MessageRouter started")
		for {
			runtime.Gosched()
			pack := <-self.InChan
			for _, runner := range pack.Config.FilterRunners {
				atomic.AddInt32(&pack.RefCount, 1)
				runner.InChan() <- pack
			}
			pack.Recycle()
		}
	}()
}
