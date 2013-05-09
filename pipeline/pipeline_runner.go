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
#   Mike Trinkala (trink@mozilla.com)
#
# ***** END LICENSE BLOCK *****/

package pipeline

import (
	"fmt"
	"github.com/mozilla-services/heka/message"
	"github.com/rafrombrc/go-notify"
	"log"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

const (
	// Control channel event types used by go-notify
	RELOAD = "reload"
	STOP   = "stop"
)

// Struct for holding global pipeline config values.
type GlobalConfigStruct struct {
	PoolSize        int
	DecoderPoolSize int
	PluginChanSize  int
	MaxMsgLoops     uint
	Stopping        bool
}

// Creates a GlobalConfigStruct object populated w/ default values.
func DefaultGlobals() (globals *GlobalConfigStruct) {
	return &GlobalConfigStruct{
		PoolSize:        100,
		DecoderPoolSize: 2,
		PluginChanSize:  50,
		MaxMsgLoops:     4,
	}
}

// Returns global pipeline config values. This function is overwritten by the
// `pipeline.NewPipelineConfig` function. Globals are generally A Bad Idea, so
// we're at least using a function instead of a struct for global state to
// make it easier to change the underlying implementation.
var Globals func() *GlobalConfigStruct

// Interface for Heka plugins that can be wired up to the config system.
type Plugin interface {
	// Receives either PluginConfig or custom config struct, populated from
	// the TOML config, and uses that data to initialize the plugin.
	Init(config interface{}) error
}

// Base interface for the Heka plugin runners.
type PluginRunner interface {
	// Plugin name.
	Name() string

	// Plugin name mutator.
	SetName(name string)

	// Underlying plugin object.
	Plugin() Plugin

	// Plugins should call `LogError` on their runner to log error messages
	// rather than doing logging directly.
	LogError(err error)

	// Plugins should call `LogMessage` on their runner to write to the log
	// rather than doing so directly.
	LogMessage(msg string)
}

// Base struct for the specialized PluginRunners
type pRunnerBase struct {
	name   string
	plugin Plugin
	h      PluginHelper
}

func (pr *pRunnerBase) Name() string {
	return pr.name
}

func (pr *pRunnerBase) SetName(name string) {
	pr.name = name
}

func (pr *pRunnerBase) Plugin() Plugin {
	return pr.plugin
}

// This one struct provides the implementation of both FilterRunner and
// OutputRunner interfaces.
type foRunner struct {
	pRunnerBase
	matcher    *MatchRunner
	tickLength time.Duration
	ticker     <-chan time.Time
	inChan     chan *PipelineCapture
	h          PluginHelper
}

// Creates and returns foRunner pointer for use as either a FilterRunner or an
// OutputRunner.
func NewFORunner(name string, plugin Plugin) (runner *foRunner) {
	runner = &foRunner{pRunnerBase: pRunnerBase{name: name, plugin: plugin}}
	runner.inChan = make(chan *PipelineCapture, Globals().PluginChanSize)
	return
}

func (foRunner *foRunner) Start(h PluginHelper, wg *sync.WaitGroup) (err error) {
	foRunner.h = h
	if foRunner.tickLength != 0 {
		foRunner.ticker = time.Tick(foRunner.tickLength)
	}

	go func() {
		defer func() {
			if r := recover(); r != nil {
				// Only recovers from panics in the main `Run` method
				// goroutine, but better than nothing.
				foRunner.LogError(fmt.Errorf("PANIC: %s", r))
			}
			wg.Done()
		}()

		if foRunner.matcher != nil {
			foRunner.matcher.Start(foRunner.inChan)
		}

		// `Run` method only returns if there's an error or we're shutting
		// down.
		if filter, ok := foRunner.plugin.(Filter); ok {
			err = filter.Run(foRunner, h)
			h.PipelineConfig().RemoveFilterRunner(foRunner.name)
		} else if output, ok := foRunner.plugin.(Output); ok {
			err = output.Run(foRunner, h)
		}
		if err != nil {
			err = fmt.Errorf("Plugin '%s' error: %s", foRunner.name, err)
		} else {
			foRunner.LogMessage("stopped")
		}
	}()
	return
}

func (foRunner *foRunner) Deliver(pack *PipelinePack) {
	plc := &PipelineCapture{Pack: pack}
	foRunner.inChan <- plc
}

func (foRunner *foRunner) Inject(pack *PipelinePack) bool {
	spec := foRunner.MatchRunner().MatcherSpecification()
	match, _ := spec.Match(pack.Message)
	if match {
		pack.Recycle()
		foRunner.LogError(fmt.Errorf("attempted to Inject a message to itself"))
		return false
	}
	foRunner.h.PipelineConfig().router.InChan() <- pack
	return true
}

func (foRunner *foRunner) LogError(err error) {
	log.Printf("Plugin '%s' error: %s", foRunner.name, err)
}

func (foRunner *foRunner) LogMessage(msg string) {
	log.Printf("Plugin '%s': %s", foRunner.name, msg)
}

func (foRunner *foRunner) Ticker() (ticker <-chan time.Time) {
	return foRunner.ticker
}

func (foRunner *foRunner) InChan() (inChan chan *PipelineCapture) {
	return foRunner.inChan
}

func (foRunner *foRunner) MatchRunner() *MatchRunner {
	return foRunner.matcher
}

func (foRunner *foRunner) Output() Output {
	return foRunner.plugin.(Output)
}

func (foRunner *foRunner) Filter() Filter {
	return foRunner.plugin.(Filter)
}

// Main Heka pipeline data structure containing raw message data, a Message
// object, and other Heka related message metadata.
type PipelinePack struct {
	// Used for storage of binary blob data that has yet to be decoded into a
	// Message object.
	MsgBytes []byte
	// Main Heka message object.
	Message *message.Message
	// Specific channel on which this pack should be recycled when all
	// processing has completed for a given message.
	RecycleChan chan *PipelinePack
	// Indicates whether or not this pack's Message object has been populated.
	Decoded bool
	// Reference count used to determine when it is safe to recycle a pack for
	// reuse by the system.
	RefCount int32
	// String id of the verified signer of the accompanying Message object, if
	// any.
	Signer string
	// Number of times the current message chain has generated new messages
	// and inserted them into the pipeline.
	MsgLoopCount uint
}

// Container data structure used on the input channel for Filters and Outputs.
type PipelineCapture struct {
	// Contains the message to be processed.
	Pack *PipelinePack
	// Contains any match group capture data that may have been obtained from
	// message_matcher regular expression processing.
	Captures map[string]string
}

// Returns a new PipelinePack pointer that will recycle itself onto the
// provided channel when a message has completed processing.
func NewPipelinePack(recycleChan chan *PipelinePack) (pack *PipelinePack) {
	msgBytes := make([]byte, message.MAX_MESSAGE_SIZE)
	message := &message.Message{}

	return &PipelinePack{
		MsgBytes:     msgBytes,
		Message:      message,
		RecycleChan:  recycleChan,
		Decoded:      false,
		RefCount:     int32(1),
		MsgLoopCount: uint(0),
	}
}

// Reset a pack to its zero state.
func (p *PipelinePack) Zero() {
	p.MsgBytes = p.MsgBytes[:cap(p.MsgBytes)]
	p.Decoded = false
	p.RefCount = 1
	p.MsgLoopCount = 0
	p.Signer = ""

	// TODO: Possibly zero the message instead depending on benchmark
	// results of re-allocating a new message
	p.Message = new(message.Message)
}

// Decrement the ref count and, if ref count == zero, zero the pack and put it
// on the appropriate recycle channel.
func (p *PipelinePack) Recycle() {
	cnt := atomic.AddInt32(&p.RefCount, -1)
	if cnt == 0 {
		p.Zero()
		p.RecycleChan <- p
	}
}

// Main function driving Heka execution. Loads config, initializes
// PipelinePack pools, and starts all the runners. Then it listens for signals
// and drives the shutdown process when that is triggered.
func Run(config *PipelineConfig) {
	log.Println("Starting hekad...")

	var inputsWg sync.WaitGroup
	var outputsWg sync.WaitGroup
	var err error

	for name, output := range config.OutputRunners {
		outputsWg.Add(1)
		if err = output.Start(config, &outputsWg); err != nil {
			log.Printf("Output '%s' failed to start: %s", name, err)
			outputsWg.Done()
			continue
		}
		log.Println("Output started: ", name)
	}

	for name, filter := range config.FilterRunners {
		config.filtersWg.Add(1)
		if err = filter.Start(config, &config.filtersWg); err != nil {
			log.Printf("Filter '%s' failed to start: %s", name, err)
			config.filtersWg.Done()
			continue
		}
		log.Println("Filter started: ", name)
	}

	// Initialize all of the PipelinePacks that we'll need
	for i := 0; i < Globals().PoolSize; i++ {
		config.inputRecycleChan <- NewPipelinePack(config.inputRecycleChan)
		config.injectRecycleChan <- NewPipelinePack(config.injectRecycleChan)
	}

	config.router.Start()

	for name, input := range config.InputRunners {
		inputsWg.Add(1)
		if err = input.Start(config, &inputsWg); err != nil {
			log.Printf("Input '%s' failed to start: %s", name, err)
			inputsWg.Done()
			continue
		}
		log.Printf("Input started: %s\n", name)
	}

	// wait for sigint
	sigChan := make(chan os.Signal)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGHUP, syscall.SIGUSR1)

	globals := Globals()
	for !globals.Stopping {
		select {
		case sig := <-sigChan:
			switch sig {
			case syscall.SIGHUP:
				log.Println("Reload initiated.")
				if err := notify.Post(RELOAD, nil); err != nil {
					log.Println("Error sending reload event: ", err)
				}
			case syscall.SIGINT:
				log.Println("Shutdown initiated.")
				globals.Stopping = true
			case syscall.SIGUSR1:
				log.Println("Queue report initiated.")
				go config.allReportsMsg()
			}
		}
	}

	defer func() {
		if r := recover(); r != nil {
			log.Printf("PANIC during shutdown: %s", r)
		}
	}()
	for _, input := range config.InputRunners {
		input.Input().Stop()
		log.Printf("Stop message sent to input '%s'", input.Name())
	}
	inputsWg.Wait()

	log.Println("Waiting for decoders shutdown")
	for _, decoder := range config.allDecoders {
		close(decoder.InChan())
		log.Printf("Stop message sent to decoder '%s'", decoder.Name())
	}
	config.decodersWg.Wait()
	log.Println("Decoders shutdown complete")

	config.filtersLock.Lock()
	for _, filter := range config.FilterRunners {
		close(filter.InChan())
		log.Printf("Stop message sent to filter '%s'", filter.Name())
	}
	config.filtersLock.Unlock()
	config.filtersWg.Wait()

	for _, output := range config.OutputRunners {
		close(output.InChan())
		log.Printf("Stop message sent to output '%s'", output.Name())
	}
	outputsWg.Wait()
	log.Println("Shutdown complete.")
}
