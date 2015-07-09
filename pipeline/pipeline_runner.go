/***** BEGIN LICENSE BLOCK *****
# This Source Code Form is subject to the terms of the Mozilla Public
# License, v. 2.0. If a copy of the MPL was not distributed with this file,
# You can obtain one at http://mozilla.org/MPL/2.0/.
#
# The Initial Developer of the Original Code is the Mozilla Foundation.
# Portions created by the Initial Developer are Copyright (C) 2012-2015
# the Initial Developer. All Rights Reserved.
#
# Contributor(s):
#   Rob Miller (rmiller@mozilla.com)
#   Mike Trinkala (trink@mozilla.com)
#   Ben Bangert (bbangert@mozilla.com)
#
# ***** END LICENSE BLOCK *****/

package pipeline

import (
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	proto "code.google.com/p/gogoprotobuf/proto"
	"github.com/mozilla-services/heka/message"
	notify "github.com/rafrombrc/go-notify"
)

const (
	// Control channel event types used by go-notify
	RELOAD = "reload"
	STOP   = "stop"
)

// Struct for holding global pipeline config values.
type GlobalConfigStruct struct {
	MaxMsgProcessDuration uint64
	PoolSize              int
	PluginChanSize        int
	MaxMsgLoops           uint
	MaxMsgProcessInject   uint
	MaxMsgTimerInject     uint
	MaxPackIdle           time.Duration
	stopping              bool
	stoppingMutex         sync.RWMutex
	BaseDir               string
	ShareDir              string
	SampleDenominator     int
	sigChan               chan os.Signal
	Hostname              string
}

// Creates a GlobalConfigStruct object populated w/ default values.
func DefaultGlobals() (globals *GlobalConfigStruct) {
	idle, _ := time.ParseDuration("2m")
	hostname, _ := os.Hostname()
	return &GlobalConfigStruct{
		PoolSize:              100,
		PluginChanSize:        50,
		MaxMsgLoops:           4,
		MaxMsgProcessInject:   1,
		MaxMsgProcessDuration: 1000000,
		MaxMsgTimerInject:     10,
		MaxPackIdle:           idle,
		SampleDenominator:     1000,
		sigChan:               make(chan os.Signal, 1),
		Hostname:              hostname,
	}
}

func (g *GlobalConfigStruct) SigChan() chan os.Signal {
	return g.sigChan
}

// Initiates a shutdown of heka
//
// This method returns immediately by spawning a goroutine to do to
// work so that the caller won't end up blocking part of the shutdown
// sequence
func (g *GlobalConfigStruct) ShutDown() {
	go func() {
		g.sigChan <- syscall.SIGINT
	}()
}

func (g *GlobalConfigStruct) IsShuttingDown() (stopping bool) {
	// So simple, no need for overhead of defer
	g.stoppingMutex.RLock()
	stopping = g.stopping
	g.stoppingMutex.RUnlock()
	return
}

func (g *GlobalConfigStruct) stop() {
	g.stoppingMutex.Lock()
	g.stopping = true
	g.stoppingMutex.Unlock()
}

// Log a message out
func (g *GlobalConfigStruct) LogMessage(src, msg string) {
	LogInfo.Printf("%s: %s", src, msg)
}

func isAbs(path string) bool {
	return strings.HasPrefix(path, string(os.PathSeparator)) || len(filepath.VolumeName(path)) > 0
}

// Expects either an absolute or relative file path. If absolute, simply
// returns the path unchanged. If relative, prepends globally configured
// BaseDir.
func (g *GlobalConfigStruct) PrependBaseDir(path string) string {
	if isAbs(path) {
		return path
	}
	return filepath.Join(g.BaseDir, path)
}

// Expects either an absolute or relative file path. If absolute, simply
// returns the path unchanged. If relative, prepends globally configured
// ShareDir.
func (g *GlobalConfigStruct) PrependShareDir(path string) string {
	if isAbs(path) {
		return path
	}
	return filepath.Join(g.ShareDir, path)
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
	// Reference count used to determine when it is safe to recycle a pack for
	// reuse by the system.
	RefCount int32
	// String id of the verified signer of the accompanying Message object, if
	// any.
	Signer string
	// Number of times the current message chain has generated new messages
	// and inserted them into the pipeline.
	MsgLoopCount uint
	// Used internally to stamp diagnostic information onto a packet.
	diagnostics *PacketTracking
	// Used to track whether or not a pack's MsgBytes needs to be re-encoded
	// before being injected into the router. Should be set to true by any
	// decoder that leaves the pack with a valid protobuf encoding of the
	// message stored in pack.MsgBytes.
	TrustMsgBytes bool
}

// Returns a new PipelinePack pointer that will recycle itself onto the
// provided channel when a message has completed processing.
func NewPipelinePack(recycleChan chan *PipelinePack) (pack *PipelinePack) {
	msgBytes := make([]byte, 0, message.MAX_MESSAGE_SIZE)
	message := &message.Message{}
	message.SetSeverity(7)

	return &PipelinePack{
		MsgBytes:      msgBytes,
		Message:       message,
		RecycleChan:   recycleChan,
		RefCount:      int32(1),
		MsgLoopCount:  uint(0),
		diagnostics:   NewPacketTracking(),
		TrustMsgBytes: false,
	}
}

// Zero resets a pack to its zero state.
func (p *PipelinePack) Zero() {
	p.MsgBytes = p.MsgBytes[:0]
	p.RefCount = 1
	p.MsgLoopCount = 0
	p.Signer = ""
	p.diagnostics.Reset()
	p.TrustMsgBytes = false

	// TODO: Possibly zero the message instead depending on benchmark
	// results of re-allocating a new message
	p.Message = new(message.Message)
}

// Recycle decrements the ref count and, if ref count == zero, zeroes the pack
// and put it on the appropriate recycle channel.
func (p *PipelinePack) Recycle() {
	cnt := atomic.AddInt32(&p.RefCount, -1)
	if cnt == 0 {
		p.Zero()
		p.RecycleChan <- p
	}
}

// EncodeMsgBytes protobuf encodes the pack's message struct and copies the
// result into the pack's MsgBytes attribute.
func (p *PipelinePack) EncodeMsgBytes() error {
	if p.TrustMsgBytes {
		return nil
	}
	msgBytes, err := proto.Marshal(p.Message)
	if err == nil {
		if cap(p.MsgBytes) < len(msgBytes) {
			p.MsgBytes = msgBytes
		} else {
			p.MsgBytes = p.MsgBytes[:len(msgBytes)]
			copy(p.MsgBytes, msgBytes)
		}
		p.TrustMsgBytes = true
	}
	return err
}

// Main function driving Heka execution. Loads config, initializes
// PipelinePack pools, and starts all the runners. Then it listens for signals
// and drives the shutdown process when that is triggered.
func Run(config *PipelineConfig) {
	LogInfo.Println("Starting hekad...")

	var outputsWg sync.WaitGroup
	var err error

	globals := config.Globals

	for name, output := range config.OutputRunners {
		outputsWg.Add(1)
		if err = output.Start(config, &outputsWg); err != nil {
			LogError.Printf("Output '%s' failed to start: %s", name, err)
			outputsWg.Done()
			if !output.IsStoppable() {
				globals.ShutDown()
			}
			continue
		}
		LogInfo.Println("Output started:", name)
	}

	for name, filter := range config.FilterRunners {
		config.filtersWg.Add(1)
		if err = filter.Start(config, &config.filtersWg); err != nil {
			LogError.Printf("Filter '%s' failed to start: %s", name, err)
			config.filtersWg.Done()
			if !filter.IsStoppable() {
				globals.ShutDown()
			}
			continue
		}
		LogInfo.Println("Filter started:", name)
	}

	// Finish initializing the router's matchers.
	config.router.initMatchSlices()

	// Setup the diagnostic trackers
	inputTracker := NewDiagnosticTracker("input", globals)
	injectTracker := NewDiagnosticTracker("inject", globals)

	// Create the report pipeline pack
	config.reportRecycleChan <- NewPipelinePack(config.reportRecycleChan)

	// Initialize all of the PipelinePacks that we'll need
	for i := 0; i < globals.PoolSize; i++ {
		inputPack := NewPipelinePack(config.inputRecycleChan)
		inputTracker.AddPack(inputPack)
		config.inputRecycleChan <- inputPack

		injectPack := NewPipelinePack(config.injectRecycleChan)
		injectTracker.AddPack(injectPack)
		config.injectRecycleChan <- injectPack
	}

	go inputTracker.Run()
	go injectTracker.Run()
	config.router.Start()

	for name, input := range config.InputRunners {
		config.inputsWg.Add(1)
		if err = input.Start(config, &config.inputsWg); err != nil {
			LogError.Printf("Input '%s' failed to start: %s", name, err)
			config.inputsWg.Done()
			if !input.IsStoppable() {
				globals.ShutDown()
			}
			continue
		}
		LogInfo.Println("Input started:", name)
	}

	// wait for sigint
	signal.Notify(globals.sigChan, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP, SIGUSR1)

	for !globals.IsShuttingDown() {
		select {
		case sig := <-globals.sigChan:
			switch sig {
			case syscall.SIGHUP:
				LogInfo.Println("Reload initiated.")
				if err := notify.Post(RELOAD, nil); err != nil {
					LogError.Println("Error sending reload event: ", err)
				}
			case syscall.SIGINT, syscall.SIGTERM:
				LogInfo.Println("Shutdown initiated.")
				globals.stop()
			case SIGUSR1:
				LogInfo.Println("Queue report initiated.")
				go config.allReportsStdout()
			}
		}
	}

	config.inputsLock.Lock()
	for _, input := range config.InputRunners {
		input.Input().Stop()
		LogInfo.Printf("Stop message sent to input '%s'", input.Name())
	}
	config.inputsLock.Unlock()
	config.inputsWg.Wait()

	config.allDecodersLock.Lock()
	LogInfo.Println("Waiting for decoders shutdown")
	for _, decoder := range config.allDecoders {
		close(decoder.InChan())
		LogInfo.Printf("Stop message sent to decoder '%s'", decoder.Name())
	}
	config.allDecoders = config.allDecoders[:0]
	config.allDecodersLock.Unlock()
	config.decodersWg.Wait()
	LogInfo.Println("Decoders shutdown complete")

	config.filtersLock.Lock()
	for _, filter := range config.FilterRunners {
		// needed for a clean shutdown without deadlocking or orphaning messages
		// 1. removes the matcher from the router
		// 2. closes the matcher input channel and lets it drain
		// 3. closes the filter input channel and lets it drain
		// 4. exits the filter
		config.router.RemoveFilterMatcher() <- filter.MatchRunner()
		LogInfo.Printf("Stop message sent to filter '%s'", filter.Name())
	}
	config.filtersLock.Unlock()
	config.filtersWg.Wait()

	for _, output := range config.OutputRunners {
		config.router.RemoveOutputMatcher() <- output.MatchRunner()
		LogInfo.Printf("Stop message sent to output '%s'", output.Name())
	}
	outputsWg.Wait()

	for name, encoder := range config.allEncoders {
		if stopper, ok := encoder.(NeedsStopping); ok {
			LogInfo.Printf("Stopping encoder '%s'", name)
			stopper.Stop()
		}
	}

	LogInfo.Println("Shutdown complete.")
}
