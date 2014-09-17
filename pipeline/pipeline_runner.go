/***** BEGIN LICENSE BLOCK *****
# This Source Code Form is subject to the terms of the Mozilla Public
# License, v. 2.0. If a copy of the MPL was not distributed with this file,
# You can obtain one at http://mozilla.org/MPL/2.0/.
#
# The Initial Developer of the Original Code is the Mozilla Foundation.
# Portions created by the Initial Developer are Copyright (C) 2012-2014
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
	"github.com/mozilla-services/heka/message"
	"github.com/rafrombrc/go-notify"
	"io/ioutil"
	"log"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"strconv"
	"strings"
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

var watchedSignals = []os.Signal{syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP, SIGUSR1}

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
}

// Creates a GlobalConfigStruct object populated w/ default values.
func DefaultGlobals() (globals *GlobalConfigStruct) {
	idle, _ := time.ParseDuration("2m")
	return &GlobalConfigStruct{
		PoolSize:              100,
		PluginChanSize:        50,
		MaxMsgLoops:           4,
		MaxMsgProcessInject:   1,
		MaxMsgProcessDuration: 1000000,
		MaxMsgTimerInject:     10,
		MaxPackIdle:           idle,
		SampleDenominator:     1000,
	}
}

// Initiates a shutdown of heka
//
// This method returns immediately by spawning a goroutine to do to
// work so that the caller won't end up blocking part of the shutdown
// sequence
func (g *GlobalConfigStruct) ShutDown() {
	g.stop()
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
	log.Printf("%s: %s", src, msg)
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
	// Used internally to stamp diagnostic information onto a packet
	diagnostics *PacketTracking
}

// Returns a new PipelinePack pointer that will recycle itself onto the
// provided channel when a message has completed processing.
func NewPipelinePack(recycleChan chan *PipelinePack) (pack *PipelinePack) {
	msgBytes := make([]byte, message.MAX_MESSAGE_SIZE)
	message := &message.Message{}
	message.SetSeverity(7)

	return &PipelinePack{
		MsgBytes:     msgBytes,
		Message:      message,
		RecycleChan:  recycleChan,
		Decoded:      false,
		RefCount:     int32(1),
		MsgLoopCount: uint(0),
		diagnostics:  NewPacketTracking(),
	}
}

// Reset a pack to its zero state.
func (p *PipelinePack) Zero() {
	p.MsgBytes = p.MsgBytes[:cap(p.MsgBytes)]
	p.Decoded = false
	p.RefCount = 1
	p.MsgLoopCount = 0
	p.Signer = ""
	p.diagnostics.Reset()

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

// handleSignals takes a channel to recieve signals from and receives from it
// until we get a signal which we need to respond to.
// SIGUSR1 generates a report
// SIGINT, SIGTERM, and SIGHUP cause this function to return with the signal
// as the return value.
func handleSignals(pConfig *PipelineConfig, signalChan chan os.Signal) (signal os.Signal) {
	for {
		signal = <-signalChan
		switch signal {
		case syscall.SIGUSR1:
			log.Println("Queue report initiated.")
			go pConfig.allReportsStdout()
		case syscall.SIGINT, syscall.SIGTERM:
			log.Println("Shutdown initiated.")
			pConfig.Globals.ShutDown()
			return
		case syscall.SIGHUP:
			log.Println("Reload initiated.")
			if err := notify.Post(RELOAD, nil); err != nil {
				log.Println("Error sending reload event: ", err)
			}
			return
		}
	}
}

func setupGlobals(configPath string) *GlobalConfigStruct {
	config := &HekadConfig{}
	var err error
	var cpuProfName string
	var memProfName string

	config, err = LoadHekadConfig(configPath)
	if err != nil {
		log.Fatal("Error reading config: ", err)
	}
	if config.SampleDenominator <= 0 {
		log.Fatalln("'sample_denominator' value must be greater than 0.")
	}
	globals, cpuProfName, memProfName := setGlobalConfigs(config)

	if err = os.MkdirAll(globals.BaseDir, 0755); err != nil {
		log.Fatalf("Error creating 'base_dir' %s: %s", config.BaseDir, err)
	}

	if config.PidFile != "" {
		contents, err := ioutil.ReadFile(config.PidFile)
		if err == nil {
			pid, err := strconv.Atoi(strings.TrimSpace(string(contents)))
			if err != nil {
				log.Fatalf("Error reading proccess id from pidfile '%s': %s", config.PidFile, err)
			}

			process, err := os.FindProcess(pid)

			// on Windows, err != nil if the process cannot be found
			if runtime.GOOS == "windows" {
				if err == nil {
					log.Fatalf("Process %d is already running.", pid)
				}
			} else if process != nil {
				// err is always nil on POSIX, so we have to send the process
				// a signal to check whether it exists
				if err = process.Signal(syscall.Signal(0)); err == nil {
					log.Fatalf("Process %d is already running.", pid)
				}
			}
		}
		if err = ioutil.WriteFile(config.PidFile, []byte(strconv.Itoa(os.Getpid())),
			0644); err != nil {

			log.Fatalf("Unable to write pidfile '%s': %s", config.PidFile, err)
		}
		log.Printf("Wrote pid to pidfile '%s'", config.PidFile)
		defer func() {
			if err = os.Remove(config.PidFile); err != nil {
				log.Printf("Unable to remove pidfile '%s': %s", config.PidFile, err)
			}
		}()
	}

	if cpuProfName != "" {
		profFile, err := os.Create(cpuProfName)
		if err != nil {
			log.Fatalln(err)
		}

		pprof.StartCPUProfile(profFile)
		defer func() {
			pprof.StopCPUProfile()
			profFile.Close()
		}()
	}

	if memProfName != "" {
		defer func() {
			profFile, err := os.Create(memProfName)
			if err != nil {
				log.Fatalln(err)
			}
			pprof.WriteHeapProfile(profFile)
			profFile.Close()
		}()
	}

	return globals
}

// Start configures Heka and listens for signals. It then starts the pipeline.
func Start(configPath string) {

	var sig os.Signal
	signalChan := make(chan os.Signal)
	defer close(signalChan)

	signal.Notify(signalChan, watchedSignals...)

	manager, err := NetManager()
	if err != nil {
		log.Fatal(err)
	}

	shutdown := false
	for !shutdown {
		manager.Reset()
		log.Println("Starting hekad...")

		// Setup global config
		globals := setupGlobals(configPath)
		// Set up and load the pipeline configuration
		pConfig := NewPipelineConfig(globals)
		if err := LoadFullConfig(pConfig, configPath); err != nil {
			log.Fatal("Error reading config: ", err)
		}

		// Init has happened, so all new listeners in use should have a plugin
		// associated with them.

		// Check which listeners still have no plugin associated
		for _, pListener := range manager.listeners {
			if pListener.plugin == nil {
				// Close all connections for this listener
				for e := pListener.conns.Front(); e != nil; e = e.Next() {
					conn := e.Value.(net.Conn)
					err = conn.Close()
					if err != nil {
						log.Println("Couldnt close connection", conn)
					}
				}
				// Close the listener
				err = pListener.Close()
				if err != nil {
					log.Println("Error closing unused listener", err)
				}
			}
		}

		// Start the pipeline
		Run(pConfig)
		// This function blocks reading from respondChan until we recieve a
		// signal which triggers a reload, or shutdown
		sig = handleSignals(pConfig, signalChan)

		if sig != syscall.SIGHUP {
			shutdown = true
		} else {
			log.Println("Restarting")
			manager.Restart()
			globals.ShutDown()
		}

		// Cleanup the pipeline
		Cleanup(pConfig)
		log.Println("Shutdown complete.")

		// Nil out plugins so we can track which ones get reused
		if manager.restarting {
			for k, pListener := range manager.listeners {
				log.Println("niling", k)
				// We want to reset the plugin each time so we can check again later
				pListener.plugin = nil
			}
		}
	}
	log.Println("Exiting Heka.")
}

// Main function driving Heka execution. Loads config, initializes
// PipelinePack pools, and starts all the runners.
func Run(config *PipelineConfig) {
	var err error

	globals := config.Globals

	for name, output := range config.OutputRunners {
		config.outputsWg.Add(1)
		if err = output.Start(config, &config.outputsWg); err != nil {
			log.Printf("Output '%s' failed to start: %s", name, err)
			config.outputsWg.Done()
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
			log.Printf("Input '%s' failed to start: %s", name, err)
			config.inputsWg.Done()
			continue
		}
		log.Printf("Input started: %s\n", name)
	}
}

func Cleanup(config *PipelineConfig) {
	config.inputsLock.Lock()
	for _, input := range config.InputRunners {
		input.Input().Stop()
		log.Printf("Stop message sent to input '%s'", input.Name())
	}
	config.inputsLock.Unlock()
	config.inputsWg.Wait()

	log.Println("Waiting for decoders shutdown")
	for _, decoder := range config.allDecoders {
		close(decoder.InChan())
		log.Printf("Stop message sent to decoder '%s'", decoder.Name())
	}
	config.decodersWg.Wait()
	log.Println("Decoders shutdown complete")

	config.filtersLock.Lock()
	for _, filter := range config.FilterRunners {
		// needed for a clean shutdown without deadlocking or orphaning messages
		// 1. removes the matcher from the router
		// 2. closes the matcher input channel and lets it drain
		// 3. closes the filter input channel and lets it drain
		// 4. exits the filter
		config.router.RemoveFilterMatcher() <- filter.MatchRunner()
		log.Printf("Stop message sent to filter '%s'", filter.Name())
	}
	config.filtersLock.Unlock()
	config.filtersWg.Wait()

	for _, output := range config.OutputRunners {
		config.router.RemoveOutputMatcher() <- output.MatchRunner()
		log.Printf("Stop message sent to output '%s'", output.Name())
	}
	config.outputsWg.Wait()

	for name, encoder := range config.allEncoders {
		if stopper, ok := encoder.(NeedsStopping); ok {
			log.Printf("Stopping encoder '%s'", name)
			stopper.Stop()
		}
	}
}
