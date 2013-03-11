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
	InChan    chan *PipelinePack
	fMatchers []*FilterSpecification
	oMatchers []*FilterSpecification
}

func NewMessageRouter() (router *MessageRouter) {
	router = new(MessageRouter)
	router.InChan = make(chan *PipelinePack, PIPECHAN_BUFSIZE)
	router.fMatchers = make([]*FilterSpecification, 0, 10)
	router.oMatchers = make([]*FilterSpecification, 0, 10)
	return router
}

func (self *MessageRouter) Start() {
	go func() {
		var fs *FilterSpecification
		var ok bool
		var pack *PipelinePack
		for {
			runtime.Gosched()
			pack, ok = <-self.InChan
			if !ok {
				break
			}
			for _, fs = range self.fMatchers {
				atomic.AddInt32(&pack.RefCount, 1)
				fs.inChan <- pack
			}
			for _, fs = range self.oMatchers {
				atomic.AddInt32(&pack.RefCount, 1)
				fs.inChan <- pack
			}
			pack.Recycle()
		}
		for _, fs = range self.fMatchers {
			close(fs.inChan)
		}
		for _, fs = range self.oMatchers {
			close(fs.inChan)
		}
		log.Println("MessageRouter stopped.")
	}()
	log.Println("MessageRouter started.")
}
