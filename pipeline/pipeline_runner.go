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
	. "github.com/mozilla-services/heka/message"
	"github.com/rafrombrc/go-notify"
	"log"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"syscall"
)

const (
	// Control channel event types used by go-notify
	RELOAD = "reload"
	STOP   = "stop"

	// buffer size for plugin channels
	PIPECHAN_BUFSIZE = 500
)

var PoolSize int

type Plugin interface {
	Init(config interface{}) error
}

type PluginRunner interface {
	InChan() chan *PipelinePack
	Name() string
	SetName(name string)
	Plugin() Plugin
	LogError(err error)
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

type PipelinePack struct {
	MsgBytes []byte
	Message  *Message
	Config   *PipelineConfig
	Decoded  bool
	RefCount int32
}

func NewPipelinePack(config *PipelineConfig) (pack *PipelinePack) {
	msgBytes := make([]byte, MAX_MESSAGE_SIZE)
	message := &Message{}

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
	p.Message = new(Message)
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

	var wg sync.WaitGroup
	var err error

	for name, output := range config.OutputRunners {
		if err = output.Start(config, &wg); err != nil {
			log.Printf("Output '%s' failed to start: %s", name, err)
			continue
		}
		wg.Add(1)
		log.Println("Output started: ", name)
	}
	for name, filter := range config.FilterRunners {
		if err = filter.Start(config, &wg); err != nil {
			log.Printf("Filter '%s' failed to start: %s", name, err)
			continue
		}
		wg.Add(1)
		log.Println("Filter started: ", name)
	}

	// Initialize all of the PipelinePacks that we'll need
	for i := 0; i < config.PoolSize; i++ {
		config.RecycleChan <- NewPipelinePack(config)
	}

	config.Router().Start()

	for name, input := range config.InputRunners {
		if err = input.Start(config, &wg); err != nil {
			log.Printf("Input '%s' failed to start: %s", name, err)
			continue
		}
		wg.Add(1)
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

	for _, filter := range config.FilterRunners {
		close(filter.InChan())
		log.Printf("Stop message sent to filter '%s'", filter.Name())
	}
	for _, output := range config.OutputRunners {
		close(output.InChan())
		log.Printf("Stop message sent to output '%s'", output.Name())
	}

	wg.Wait()
	wg.Add(1)
	mgi.Stop()
	log.Println("Shutdown complete.")
}
