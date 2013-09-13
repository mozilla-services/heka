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
	"math/rand"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"
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
	// Channel to facilitate adding a matcher to the router which starts the
	// message flow to the associated filter.
	AddFilterMatcher() chan *MatchRunner
	// Channel to facilitate removing a Filter.  If the matcher exists it will
	// be removed from the router, the matcher channel closed and drained, the
	// filter channel closed and drained, and the filter exited.
	RemoveFilterMatcher() chan *MatchRunner
	// Channel to facilitate removing an Output.  If the matcher exists it will
	// be removed from the router, the matcher channel closed and drained, the
	// output channel closed and drained, and the output exited.
	RemoveOutputMatcher() chan *MatchRunner
}

type messageRouter struct {
	inChan              chan *PipelinePack
	addFilterMatcher    chan *MatchRunner
	removeFilterMatcher chan *MatchRunner
	removeOutputMatcher chan *MatchRunner
	fMatchers           []*MatchRunner
	oMatchers           []*MatchRunner
	processMessageCount int64
}

// Creates and returns a (not yet started) Heka message router.
func NewMessageRouter() (router *messageRouter) {
	router = new(messageRouter)
	router.inChan = make(chan *PipelinePack, Globals().PluginChanSize)
	router.addFilterMatcher = make(chan *MatchRunner, 0)
	router.removeFilterMatcher = make(chan *MatchRunner, 0)
	router.removeOutputMatcher = make(chan *MatchRunner, 0)
	router.fMatchers = make([]*MatchRunner, 0, 10)
	router.oMatchers = make([]*MatchRunner, 0, 10)
	return router
}

func (self *messageRouter) InChan() chan *PipelinePack {
	return self.inChan
}

func (self *messageRouter) AddFilterMatcher() chan *MatchRunner {
	return self.addFilterMatcher
}

func (self *messageRouter) RemoveFilterMatcher() chan *MatchRunner {
	return self.removeFilterMatcher
}

func (self *messageRouter) RemoveOutputMatcher() chan *MatchRunner {
	return self.removeOutputMatcher
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
			case matcher = <-self.addFilterMatcher:
				if matcher != nil {
					exists := false
					available := -1
					for i, m := range self.fMatchers {
						if m == nil {
							available = i
						}
						if matcher == m {
							exists = true
							break
						}
					}
					if !exists {
						if available != -1 {
							self.fMatchers[available] = matcher
						} else {
							self.fMatchers = append(self.fMatchers, matcher)
						}
					}
				}
			case matcher = <-self.removeFilterMatcher:
				if matcher != nil {
					for i, m := range self.fMatchers {
						if matcher == m {
							close(m.inChan)
							self.fMatchers[i] = nil
							break
						}
					}
				}
			case matcher = <-self.removeOutputMatcher:
				if matcher != nil {
					for i, m := range self.oMatchers {
						if matcher == m {
							close(m.inChan)
							self.oMatchers[i] = nil
							break
						}
					}
				}
			case pack, ok = <-self.inChan:
				if !ok {
					break
				}
				pack.diagnostics.Reset()
				atomic.AddInt64(&self.processMessageCount, 1)
				for _, matcher = range self.fMatchers {
					if matcher != nil {
						atomic.AddInt32(&pack.RefCount, 1)
						pack.diagnostics.AddStamp(matcher.pluginRunner)
						matcher.inChan <- pack
					}
				}
				for _, matcher = range self.oMatchers {
					if matcher != nil {
						atomic.AddInt32(&pack.RefCount, 1)
						pack.diagnostics.AddStamp(matcher.pluginRunner)
						matcher.inChan <- pack
					}
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
	spec          *message.MatcherSpecification
	signer        string
	inChan        chan *PipelinePack
	pluginRunner  PluginRunner
	matchSamples  int64
	matchDuration int64
	reportLock    sync.Mutex
}

// Creates and returns a new MatchRunner if possible, or a relevant error if
// not.
func NewMatchRunner(filter, signer string, runner PluginRunner) (matcher *MatchRunner, err error) {
	var spec *message.MatcherSpecification
	if spec, err = message.CreateMatcherSpecification(filter); err != nil {
		return
	}
	matcher = &MatchRunner{
		spec:         spec,
		signer:       signer,
		inChan:       make(chan *PipelinePack, Globals().PluginChanSize),
		pluginRunner: runner,
	}
	return
}

// Returns the runner's MatcherSpecification object.
func (mr *MatchRunner) MatcherSpecification() *message.MatcherSpecification {
	return mr.spec
}

// Starts the runner listening for messages on its input channel. Any message
// that is a match will be placed on the provided matchChan (usually the input
// channel for a specific Filter or Output plugin). Any messages that are not a
// match will be immediately recycled.
func (mr *MatchRunner) Start(matchChan chan *PipelinePack) {
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

		var (
			startTime time.Time
			random    int = rand.Intn(1000) + DURATION_SAMPLE_DENOMINATOR
			// Don't have everyone sample at the same time. We always start with
			// a sample so there will be a ballpark figure immediately. We could
			// use a ticker to sample at a regular interval but that seems like
			// overkill at this  point.
			counter  int = random
			match    bool
			duration int64
		)

		var capacity int64 = int64(cap(mr.inChan))
		for pack := range mr.inChan {
			if len(mr.signer) != 0 && mr.signer != pack.Signer {
				pack.Recycle()
				continue
			}
			// We may want to keep separate samples for match/nomatch conditions.
			// In most cases the random sampling will capture the most common
			// condition which is usesful for the overall system health but not
			// matcher tuning.  Capturing the duration adds ~40ns
			if counter == random {
				startTime = time.Now()

				match = mr.spec.Match(pack.Message)

				duration = time.Since(startTime).Nanoseconds()
				mr.reportLock.Lock()
				mr.matchDuration += duration
				mr.matchSamples++
				mr.reportLock.Unlock()
				if mr.matchSamples > capacity {
					// the timings can vary greatly, so we need to establish a
					// decent baseline before we start sampling
					counter = 0
				}
			} else {
				match = mr.spec.Match(pack.Message)
				counter++
			}

			if match {
				matchChan <- pack
			} else {
				pack.Recycle()
			}
		}
		close(matchChan)
	}()
}
