/***** BEGIN LICENSE BLOCK *****
# This Source Code Form is subject to the terms of the Mozilla Public
# License, v. 2.0. If a copy of the MPL was not distributed with this file,
# You can obtain one at http://mozilla.org/MPL/2.0/.
#
# The Initial Developer of the Original Code is the Mozilla Foundation.
# Portions created by the Initial Developer are Copyright (C) 2013-2015
# the Initial Developer. All Rights Reserved.
#
# Contributor(s):
#   Rob Miller (rmiller@mozilla.com)
#   Mike Trinkala (trink@mozilla.com)
#
# ***** END LICENSE BLOCK *****/

package pipeline

import (
	"errors"
	"fmt"
	"math/rand"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/mozilla-services/heka/message"
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
	// Inject message for matching and possible delivery to all filter and
	// output plugins.
	Inject(pack *PipelinePack) error
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
	processMessageCount int64
	inChan              chan *PipelinePack
	addFilterMatcher    chan *MatchRunner
	removeFilterMatcher chan *MatchRunner
	removeOutputMatcher chan *MatchRunner
	fMatchers           []*MatchRunner
	oMatchers           []*MatchRunner
	// These are used during initialization time only to prevent false
	// duplicate matchers, they will *not* be kept up to date as matchers are
	// added to / removed from the router. The slices defined above contain
	// the definitive list of active matchers.
	fMatcherMap map[string]*MatchRunner
	oMatcherMap map[string]*MatchRunner
	abortChan   chan struct{}
}

// Creates and returns a (not yet started) Heka message router.
func NewMessageRouter(chanSize int, abortChan chan struct{}) (router *messageRouter) {
	router = new(messageRouter)
	router.inChan = make(chan *PipelinePack, chanSize)
	router.addFilterMatcher = make(chan *MatchRunner, 0)
	router.removeFilterMatcher = make(chan *MatchRunner, 0)
	router.removeOutputMatcher = make(chan *MatchRunner, 0)
	router.fMatcherMap = make(map[string]*MatchRunner)
	router.oMatcherMap = make(map[string]*MatchRunner)
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

func (self *messageRouter) Inject(pack *PipelinePack) error {
	select {
	case self.inChan <- pack:
		return nil
	case <-self.abortChan:
		return AbortError
	}
}

// initMatchSlices creates the `fMatchers` and `oMatchers` MatchRunner slices
// and populates them with the matchers that are in the respective matcher
// maps. Should be called exactly once after all of the config has been loaded
// but before the router is started.
func (self *messageRouter) initMatchSlices() {
	self.fMatchers = make([]*MatchRunner, 0, len(self.fMatcherMap))
	self.oMatchers = make([]*MatchRunner, 0, len(self.oMatcherMap))
	for _, matcher := range self.fMatcherMap {
		self.fMatchers = append(self.fMatchers, matcher)
	}
	for _, matcher := range self.oMatcherMap {
		self.oMatchers = append(self.oMatchers, matcher)
	}
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
							m.Close()
							self.fMatchers[i] = nil
							break
						}
					}
				}
			case matcher = <-self.removeOutputMatcher:
				if matcher != nil {
					for i, m := range self.oMatchers {
						if matcher == m {
							m.Close()
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
						matcher.inChan <- pack
					}
				}
				for _, matcher = range self.oMatchers {
					if matcher != nil {
						atomic.AddInt32(&pack.RefCount, 1)
						matcher.inChan <- pack
					}
				}
				pack.recycle()
			}
		}
		for _, matcher = range self.fMatchers {
			if matcher != nil {
				matcher.Close()
			}
		}
		for _, matcher = range self.oMatchers {
			matcher.Close()
		}
		LogInfo.Println("MessageRouter stopped.")
	}()
	LogInfo.Println("MessageRouter started.")
}

// Encapsulates the mechanics of testing messages against a specific plugin's
// message_matcher value.
type MatchRunner struct {
	closing       int32
	matchSamples  int64
	matchDuration int64
	spec          *message.MatcherSpecification
	signer        string
	inChan        chan *PipelinePack
	matchChan     chan *PipelinePack
	stopChan      chan bool
	pluginRunner  PluginRunner
	reportLock    sync.Mutex
	bufFeeder     *BufferFeeder
	globals       *GlobalConfigStruct
	retry         *RetryHelper
}

// Creates and returns a new MatchRunner if possible, or a relevant error if
// not.
func NewMatchRunner(filter, signer string, runner PluginRunner, chanSize int,
	matchChan chan *PipelinePack) (matcher *MatchRunner, err error) {

	var spec *message.MatcherSpecification
	if spec, err = message.CreateMatcherSpecification(filter); err != nil {
		return
	}
	retry, _ := NewRetryHelper(RetryOptions{
		MaxDelay:   "1s",
		Delay:      "50ms",
		MaxRetries: -1,
	})
	matcher = &MatchRunner{
		spec:         spec,
		signer:       signer,
		inChan:       make(chan *PipelinePack, chanSize),
		matchChan:    matchChan,
		pluginRunner: runner,
		retry:        retry,
	}
	return
}

// Returns the runner's MatcherSpecification object.
func (mr *MatchRunner) MatcherSpecification() *message.MatcherSpecification {
	return mr.spec
}

// Returns the Matcher InChan length for backpresure detection and reporting
func (mr *MatchRunner) InChanLen() int {
	return len(mr.inChan)
}

func (mr *MatchRunner) Close() {
	atomic.StoreInt32(&mr.closing, 1)
	close(mr.inChan)
}

// Returns the runner's average match duration in nanoseconds
func (mr *MatchRunner) GetAvgDuration() (duration int64) {
	mr.reportLock.Lock()
	if mr.matchSamples != 0 {
		duration = mr.matchDuration / mr.matchSamples
	}
	mr.reportLock.Unlock()
	return
}

func (mr *MatchRunner) run(sampleDenom int) {
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
		random    int = rand.Intn(1000) + sampleDenom
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
			pack.recycle()
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
			pack.diagnostics.AddStamp(mr.pluginRunner)
			err := mr.deliver(pack)
			if err != nil {
				mr.pluginRunner.LogError(fmt.Errorf("can't deliver matched message: %s",
					err))
			}
		} else {
			pack.recycle()
		}
	}
	if mr.matchChan != nil {
		close(mr.matchChan)
	}
	if mr.stopChan != nil {
		close(mr.stopChan)
	}
}

// Starts the runner listening for messages on its input channel. Any message
// that is a match will be placed on the provided matchChan, or written out to
// the disk queue if buffering is in play. Any messages that are not a match
// will be immediately recycled.
func (mr *MatchRunner) Start(sampleDenom int) {
	go mr.run(sampleDenom)
}

func (mr *MatchRunner) deliver(pack *PipelinePack) error {
	if mr.bufFeeder != nil {
		err := mr.bufFeeder.QueueRecord(pack)
		if err == QueueIsFull {
			switch mr.bufFeeder.Config.FullAction {
			case "shutdown":
				mr.globals.ShutDown()
			case "block":
				for {
					err = mr.bufFeeder.QueueRecord(pack)
					if err != QueueIsFull {
						break
					}
					if atomic.LoadInt32(&mr.closing) != 0 {
						break
					}
					mr.retry.Wait()
				}
				mr.retry.Reset()
			case "drop":
			}
		}
		pack.recycle()
		return err
	}
	if mr.matchChan != nil {
		mr.matchChan <- pack
		return nil
	}
	return errors.New("no queue buffer or match chan for delivery")
}
