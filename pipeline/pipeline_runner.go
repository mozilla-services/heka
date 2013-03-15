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

	// buffer size for plugin channels
	PIPECHAN_BUFSIZE = 500
)

var PoolSize int

// Interface for Heka plugins that can be wired up to the config system.
type Plugin interface {
	Init(config interface{}) error
}

// Base interface for the Heka plugin runners.
type PluginRunner interface {
	InChan() chan *PipelinePack
	Name() string
	SetName(name string)
	Plugin() Plugin
	LogError(err error)
	LogMessage(msg string)
}

// Base struct for the specialized PluginRunners
type pRunnerBase struct {
	inChan chan *PipelinePack
	name   string
	plugin Plugin
}

func (pr *pRunnerBase) InChan() chan *PipelinePack {
	return pr.inChan
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

func (pr *pRunnerBase) Input() Input {
	return pr.plugin.(Input)
}

func (pr *pRunnerBase) Output() Output {
	return pr.plugin.(Output)
}

func (pr *pRunnerBase) Filter() Filter {
	return pr.plugin.(Filter)
}

type foRunner struct {
	pRunnerBase
	matcher    *MatchRunner
	tickLength time.Duration
	ticker     <-chan time.Time
}

func NewFORunner(name string, plugin Plugin) (runner *foRunner) {
	runner = &foRunner{pRunnerBase: pRunnerBase{name: name, plugin: plugin}}
	runner.inChan = make(chan *PipelinePack, PIPECHAN_BUFSIZE)
	return
}

func (foRunner *foRunner) Start(h PluginHelper, wg *sync.WaitGroup) (err error) {
	if foRunner.tickLength != 0 {
		foRunner.ticker = time.Tick(foRunner.tickLength)
	}

	if filter, ok := foRunner.plugin.(Filter); ok {
		err = filter.Start(foRunner, h, wg)
	} else if output, ok := foRunner.plugin.(Output); ok {
		err = output.Start(foRunner, h, wg)
	}

	if err != nil {
		err = fmt.Errorf("Plugin '%s' failed to start: %s", foRunner.name, err)
	} else if foRunner.matcher != nil {
		foRunner.matcher.Start(foRunner.inChan)
	}
	return
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

type PipelinePack struct {
	MsgBytes []byte
	Message  *message.Message
	Config   *PipelineConfig
	Decoded  bool
	RefCount int32
}

func NewPipelinePack(config *PipelineConfig) (pack *PipelinePack) {
	msgBytes := make([]byte, message.MAX_MESSAGE_SIZE)
	message := &message.Message{}

	return &PipelinePack{
		MsgBytes: msgBytes,
		Message:  message,
		Config:   config,
		Decoded:  false,
		RefCount: int32(1),
	}
}

func (p *PipelinePack) Zero() {
	p.MsgBytes = p.MsgBytes[:cap(p.MsgBytes)]
	p.Decoded = false
	p.RefCount = 1

	// TODO: Possibly zero the message instead depending on benchmark
	// results of re-allocating a new message
	p.Message = new(message.Message)
}

func (p *PipelinePack) Recycle() {
	cnt := atomic.AddInt32(&p.RefCount, -1)
	if cnt == 0 {
		p.Zero()
		p.Config.RecycleChan <- p
	}
}

func Run(config *PipelineConfig) {
	log.Println("Starting hekad...")

	var inputsWg sync.WaitGroup
	var filtersWg sync.WaitGroup
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
		filtersWg.Add(1)
		if err = filter.Start(config, &filtersWg); err != nil {
			log.Printf("Filter '%s' failed to start: %s", name, err)
			filtersWg.Done()
			continue
		}
		log.Println("Filter started: ", name)
	}

	// Initialize all of the PipelinePacks that we'll need
	for i := 0; i < config.PoolSize; i++ {
		config.RecycleChan <- NewPipelinePack(config)
	}

	config.Router().Start()

	for name, input := range config.InputRunners {
		// Special case the MGI, it shuts down last.
		if name != "MessageGeneratorInput" {
			inputsWg.Add(1)
		}
		if err = input.Start(config, &inputsWg); err != nil {
			log.Printf("Input '%s' failed to start: %s", name, err)
			inputsWg.Done()
			continue
		}
		log.Printf("Input started: %s\n", name)
	}

	// wait for sigint
	sigChan := make(chan os.Signal)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGHUP)

	ok := true
	for ok {
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
				ok = false
			}
		}
	}

	var mgi Input
	for name, input := range config.InputRunners {
		// First we stop all the inputs save the MGI to prevent new messages
		// from being accepted.
		if name == "MessageGeneratorInput" {
			mgi = input.Input()
			continue
		}
		input.Input().Stop()
		log.Printf("Stop message sent to input '%s'", input.Name())
	}
	inputsWg.Wait()

	for _, filter := range config.FilterRunners {
		close(filter.InChan())
		log.Printf("Stop message sent to filter '%s'", filter.Name())
	}
	filtersWg.Wait()

	for _, output := range config.OutputRunners {
		close(output.InChan())
		log.Printf("Stop message sent to output '%s'", output.Name())
	}
	outputsWg.Wait()

	inputsWg.Add(1)
	mgi.Stop()
	log.Println("Shutdown complete.")
}
