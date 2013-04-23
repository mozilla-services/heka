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

// Public interface exposed by the Heka message router. The message router
// accepts packs on its input channel and then runs them through the
// message_matcher for every running Filter and Output plugin. For plugins
// with a positive match, the pack (and any relevant match group captures)
// will be placed on the plugin's input channel.
type MessageRouter interface {
	// Input channel from which the router gets messages to test against the
	// registered plugin message_matchers.
	InChan() chan *PipelinePack
	// Channel holding a reference to all running message_matchers for easy
	// access to the entire set.
	MrChan() chan *MatchRunner
}

type messageRouter struct {
	inChan    chan *PipelinePack
	mrChan    chan *MatchRunner
	fMatchers []*MatchRunner
	oMatchers []*MatchRunner
}

// Creates and returns a (not yet started) Heka message router.
func NewMessageRouter() (router *messageRouter) {
	router = new(messageRouter)
	router.inChan = make(chan *PipelinePack, Globals().PluginChanSize)
	router.mrChan = make(chan *MatchRunner, 0)
	router.fMatchers = make([]*MatchRunner, 0, 10)
	router.oMatchers = make([]*MatchRunner, 0, 10)
	return router
}

func (self *messageRouter) InChan() chan *PipelinePack {
	return self.inChan
}

func (self *messageRouter) MrChan() chan *MatchRunner {
	return self.mrChan
}

// Spawns a goroutine within which the router listens for messages on the
// input channel and performs its routing magic. Spawned goroutine continues
// until the router is shut down, triggered by closing the router's input
// channel.
func (self *messageRouter) Start() {
	go func() {
		var matcher *MatchRunner
		var ok = true
		var pack *PipelinePack
		for ok {
			runtime.Gosched()
			select {
			case matcher = <-self.mrChan:
				if matcher != nil {
					removed := false
					available := -1
					for i, m := range self.fMatchers {
						if m == nil {
							available = i
						}
						if matcher == m {
							close(m.inChan)
							self.fMatchers[i] = nil
							removed = true
							break
						}
					}
					if !removed {
						if available != -1 {
							self.fMatchers[available] = matcher
						} else {
							self.fMatchers = append(self.fMatchers, matcher)
						}
					}
				}
			case pack, ok = <-self.inChan:
				if !ok {
					break
				}
				for _, matcher = range self.fMatchers {
					if matcher != nil {
						atomic.AddInt32(&pack.RefCount, 1)
						matcher.inChan <- pack
					}
				}
				for _, matcher = range self.oMatchers {
					atomic.AddInt32(&pack.RefCount, 1)
					matcher.inChan <- pack
				}
				pack.Recycle()
			}
		}
		for _, matcher = range self.fMatchers {
			if matcher != nil {
				close(matcher.inChan)
			}
		}
		for _, matcher = range self.oMatchers {
			close(matcher.inChan)
		}
		log.Println("MessageRouter stopped.")
	}()
	log.Println("MessageRouter started.")
}

// Encapsulates the mechanics of testing messages against a specific plugin's
// message_matcher value.
type MatchRunner struct {
	spec   *message.MatcherSpecification
	signer string
	inChan chan *PipelinePack
}

// Creates and returns a new MatchRunner if possible, or a relevant error if
// not.
func NewMatchRunner(filter, signer string) (matcher *MatchRunner, err error) {
	var spec *message.MatcherSpecification
	if spec, err = message.CreateMatcherSpecification(filter); err != nil {
		return
	}
	matcher = &MatchRunner{
		spec:   spec,
		signer: signer,
		inChan: make(chan *PipelinePack, Globals().PluginChanSize),
	}
	return
}

// Returns the runner's MatcherSpecification object.
func (mr *MatchRunner) MatcherSpecification() *message.MatcherSpecification {
	return mr.spec
}

// Starts the runner listening for messages on its input channel. Any message
// that is a match will be embedded within a PipelineCapture and placed on the
// provided matchChan (usually the input channel for a specific Filter or
// Output plugin). Any messages that are not a match will be immediately
// recycled.
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
			if len(mr.signer) != 0 && mr.signer != pack.Signer {
				pack.Recycle()
				continue
			}
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
