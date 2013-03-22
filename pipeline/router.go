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
	"github.com/mozilla-services/heka/message"
	"log"
	"runtime"
	"strings"
	"sync/atomic"
)

// Pushes the message onto the filters input channel if it is a match
type MessageRouter struct {
	InChan    chan *PipelinePack
	fMatchers []*MatchRunner
	oMatchers []*MatchRunner
}

func NewMessageRouter() (router *MessageRouter) {
	router = new(MessageRouter)
	router.InChan = make(chan *PipelinePack, PIPECHAN_BUFSIZE)
	router.fMatchers = make([]*MatchRunner, 0, 10)
	router.oMatchers = make([]*MatchRunner, 0, 10)
	return router
}

func (self *MessageRouter) Start() {
	go func() {
		var matcher *MatchRunner
		var ok bool
		var pack *PipelinePack
		for {
			runtime.Gosched()
			pack, ok = <-self.InChan
			if !ok {
				break
			}
			for _, matcher = range self.fMatchers {
				atomic.AddInt32(&pack.RefCount, 1)
				matcher.inChan <- pack
			}
			for _, matcher = range self.oMatchers {
				atomic.AddInt32(&pack.RefCount, 1)
				matcher.inChan <- pack
			}
			pack.Recycle()
		}
		for _, matcher = range self.fMatchers {
			close(matcher.inChan)
		}
		for _, matcher = range self.oMatchers {
			close(matcher.inChan)
		}
		log.Println("MessageRouter stopped.")
	}()
	log.Println("MessageRouter started.")
}

type MatchRunner struct {
	spec   *message.MatcherSpecification
	inChan chan *PipelinePack
}

func NewMatchRunner(filter string) (matcher *MatchRunner, err error) {
	var spec *message.MatcherSpecification
	if spec, err = message.CreateMatcherSpecification(filter); err != nil {
		return
	}
	matcher = &MatchRunner{
		spec:   spec,
		inChan: make(chan *PipelinePack, PIPECHAN_BUFSIZE),
	}
	return
}

func (mr *MatchRunner) Start(matchChan chan *PipelineCapture) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				var err error
				var ok bool
				if err, ok = r.(error); !ok {
					panic(r)
				}
				if !strings.Contains(err.Error(), "send on closed channel") {
					panic(r)
				}
			}
		}()

		for pack := range mr.inChan {
			match, captures := mr.spec.Match(pack.Message)
			if match {
				plc := &PipelineCapture{Pack: pack, Captures: captures}
				matchChan <- plc
			} else {
				pack.Recycle()
			}
		}
	}()
}
