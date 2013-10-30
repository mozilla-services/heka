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
#   Ben Bangert (bbangert@mozilla.com)
#
# ***** END LICENSE BLOCK *****/

package pipeline

import (
	"crypto/rand"
	"errors"
	"fmt"
	"github.com/mozilla-services/heka/message"
	"github.com/rafrombrc/go-notify"
	"log"
	"math/big"
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
	PoolSize              int
	DecoderPoolSize       int
	PluginChanSize        int
	MaxMsgLoops           uint
	MaxMsgProcessInject   uint
	MaxMsgProcessDuration uint64
	MaxMsgTimerInject     uint
	MaxPackIdle           time.Duration
	Stopping              bool
	BaseDir               string
	sigChan               chan os.Signal
}

// Creates a GlobalConfigStruct object populated w/ default values.
func DefaultGlobals() (globals *GlobalConfigStruct) {
	idle, _ := time.ParseDuration("2m")
	return &GlobalConfigStruct{
		PoolSize:              100,
		DecoderPoolSize:       2,
		PluginChanSize:        50,
		MaxMsgLoops:           4,
		MaxMsgProcessInject:   1,
		MaxMsgProcessDuration: 1000000,
		MaxMsgTimerInject:     10,
		MaxPackIdle:           idle,
	}
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

// Log a message out
func (g *GlobalConfigStruct) LogMessage(src, msg string) {
	log.Printf("%s: %s", src, msg)
}

// Returns global pipeline config values. This function is overwritten by the
// `pipeline.NewPipelineConfig` function. Globals are generally A Bad Idea, so
// we're at least using a function instead of a struct for global state to
// make it easier to change the underlying implementation.
var Globals func() *GlobalConfigStruct

// Diagnostic object for packet tracking
type PacketTracking struct {
	// Records last accessed time
	LastAccess time.Time

	// Records the plugins the packet has been handed to
	lastPlugins []PluginRunner
}

// Create a new blank PacketTracking
func NewPacketTracking() *PacketTracking {
	return &PacketTracking{time.Now(), make([]PluginRunner, 0, 8)}
}

// Stamps a packet with the tracking data for the plugin its handed to,
// clearing any existing stamps
func (p *PacketTracking) Stamp(pluginRunner PluginRunner) {
	p.lastPlugins = p.lastPlugins[:0]
	p.lastPlugins = append(p.lastPlugins, pluginRunner)
	p.LastAccess = time.Now()
}

// Adds a stamp to the packet
func (p *PacketTracking) AddStamp(pluginRunner PluginRunner) {
	p.lastPlugins = append(p.lastPlugins, pluginRunner)
	p.LastAccess = time.Now()
}

// Resets the packet stamping
func (p *PacketTracking) Reset() {
	p.lastPlugins = p.lastPlugins[:0]
	p.LastAccess = time.Now()
}

// Returns the names of the plugins that have last accessed the packet
func (p *PacketTracking) PluginNames() (names []string) {
	names = make([]string, 0, 4)
	for _, pr := range p.lastPlugins {
		names = append(names, pr.Name())
	}
	return
}

// Returns the names of the plugin runners that last access the packet
func (p *PacketTracking) Runners() (runners []PluginRunner) {
	return p.lastPlugins
}

// A diagnostic tracker that can track pipeline packs and do accounting
// to determine possible leaks
type DiagnosticTracker struct {
	// Track all the packs that have been created
	packs []*PipelinePack

	// Identify the name of the recycle channel it monitors packs for
	ChannelName string
}

// Create and return a new diagnostic tracker
func NewDiagnosticTracker(channelName string) *DiagnosticTracker {
	return &DiagnosticTracker{make([]*PipelinePack, 0, 50), channelName}
}

// Add a pipeline pack for monitoring
func (d *DiagnosticTracker) AddPack(pack *PipelinePack) {
	d.packs = append(d.packs, pack)
}

// Run the monitoring routine, this should be spun up in a new goroutine
func (d *DiagnosticTracker) Run() {
	var (
		pack           *PipelinePack
		earliestAccess time.Time
		pluginCounts   map[PluginRunner]int
		count          int
		runner         PluginRunner
	)
	g := Globals()
	idleMax := g.MaxPackIdle
	probablePacks := make([]*PipelinePack, 0, len(d.packs))
	ticker := time.NewTicker(time.Duration(30) * time.Second)
	for {
		<-ticker.C
		probablePacks = probablePacks[:0]
		pluginCounts = make(map[PluginRunner]int)

		// Locate all the packs that have not been touched in idleMax duration
		// that are not recycled
		earliestAccess = time.Now().Add(-idleMax)
		for _, pack = range d.packs {
			if len(pack.diagnostics.lastPlugins) == 0 {
				continue
			}
			if pack.diagnostics.LastAccess.Before(earliestAccess) {
				probablePacks = append(probablePacks, pack)
				for _, runner = range pack.diagnostics.Runners() {
					pluginCounts[runner] += 1
				}
			}
		}

		// Drop a warning about how many packs have been idle
		if len(probablePacks) > 0 {
			g.LogMessage("Diagnostics", fmt.Sprintf("%d packs have been idle more than %d seconds.",
				d.ChannelName, len(probablePacks), idleMax))
			g.LogMessage("Diagnostics", fmt.Sprintf("(%s) Plugin names and quantities found on idle packs:",
				d.ChannelName))
			for runner, count = range pluginCounts {
				runner.SetLeakCount(count)
				g.LogMessage("Diagnostics", fmt.Sprintf("\t%s: %d", runner.Name(), count))
			}
			log.Println("")
		}
	}
}

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

	// Plugin Globals, these are the globals accepted for the plugin in the
	// config file
	PluginGlobals() *PluginGlobals

	// Sets the amount of currently 'leaked' packs that have gone through
	// this plugin. The new value will overwrite prior ones.
	SetLeakCount(count int)

	// Returns the current leak count
	LeakCount() int
}

// Base struct for the specialized PluginRunners
type pRunnerBase struct {
	name          string
	plugin        Plugin
	pluginGlobals *PluginGlobals
	h             PluginHelper
	leakCount     int
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

func (pr *pRunnerBase) PluginGlobals() *PluginGlobals {
	return pr.pluginGlobals
}

func (pr *pRunnerBase) SetLeakCount(count int) {
	pr.leakCount = count
}

func (pr *pRunnerBase) LeakCount() int {
	return pr.leakCount
}

// Retry helper, created with a RetryOptions struct
//
// Everytime Wait is called, the times this has been used is incremented.
// Calling Reset will reset the time counter indicating the operation that
// was being retried succeeded.
type RetryHelper struct {
	maxDelay  time.Duration
	delay     time.Duration
	curDelay  time.Duration
	maxJitter time.Duration
	retries   int
	times     int
}

// Creates and returns a RetryHelper pointer to be used when retrying
// plugin restarts or other parts that require exponential backoff
func NewRetryHelper(opts RetryOptions) (helper *RetryHelper, err error) {
	if opts.Delay == "" {
		opts.Delay = "250ms"
	}
	if opts.MaxDelay == "" {
		opts.MaxDelay = "30s"
	}
	if opts.MaxJitter == "" {
		opts.MaxJitter = "500ms"
	}
	delay, err := time.ParseDuration(opts.Delay)
	if err != nil {
		return
	}
	maxDelay, err := time.ParseDuration(opts.MaxDelay)
	if err != nil {
		return
	}
	maxJitter, err := time.ParseDuration(opts.MaxJitter)
	if err != nil {
		return
	}
	helper = &RetryHelper{
		maxDelay:  maxDelay,
		delay:     delay,
		curDelay:  delay,
		retries:   opts.MaxRetries,
		maxJitter: maxJitter,
		times:     0,
	}
	return
}

// Wait for a retry
//
// If the max retries has been exceeded, an error will be returned
func (r *RetryHelper) Wait() error {
	if r.retries != -1 && r.times >= r.retries {
		return errors.New("Max retries exceeded")
	}
	jitter, _ := rand.Int(rand.Reader, big.NewInt(r.maxJitter.Nanoseconds()))
	jitterWait := time.Duration(jitter.Int64()) * time.Nanosecond
	timer := time.NewTimer(r.curDelay + jitterWait)
	select {
	case <-timer.C:
		break
	}
	r.curDelay *= 2
	r.times += 1
	if r.curDelay > r.maxDelay {
		r.curDelay = r.maxDelay
	}
	return nil
}

// Reset the retry counter
func (r *RetryHelper) Reset() {
	r.times = 0
	r.curDelay = r.delay
}

// This one struct provides the implementation of both FilterRunner and
// OutputRunner interfaces.
type foRunner struct {
	pRunnerBase
	matcher    *MatchRunner
	tickLength time.Duration
	ticker     <-chan time.Time
	inChan     chan *PipelinePack
	h          PluginHelper
	retainPack *PipelinePack
	leakCount  int
}

// Creates and returns foRunner pointer for use as either a FilterRunner or an
// OutputRunner.
func NewFORunner(name string, plugin Plugin,
	pluginGlobals *PluginGlobals) (runner *foRunner) {
	runner = &foRunner{
		pRunnerBase: pRunnerBase{
			name:          name,
			plugin:        plugin,
			pluginGlobals: pluginGlobals,
		},
	}
	runner.inChan = make(chan *PipelinePack, Globals().PluginChanSize)
	return
}

func (foRunner *foRunner) Start(h PluginHelper, wg *sync.WaitGroup) (err error) {
	foRunner.h = h
	if foRunner.tickLength != 0 {
		foRunner.ticker = time.Tick(foRunner.tickLength)
	}

	go foRunner.Starter(h, wg)
	return
}

func (foRunner *foRunner) Starter(h PluginHelper, wg *sync.WaitGroup) {
	var (
		pluginType string
		err        error
	)
	globals := Globals()
	defer func() {
		wg.Done()
	}()

	rh, err := NewRetryHelper(foRunner.pluginGlobals.Retries)
	if err != nil {
		foRunner.LogError(err)
		globals.ShutDown()
		return
	}

	var pw *PluginWrapper
	pc := h.PipelineConfig()

	for !globals.Stopping {
		if foRunner.matcher != nil {
			foRunner.matcher.Start(foRunner.inChan)
		}

		// `Run` method only returns if there's an error or we're shutting
		// down.
		if filter, ok := foRunner.plugin.(Filter); ok {
			pluginType = "filter"
			err = filter.Run(foRunner, h)
		} else if output, ok := foRunner.plugin.(Output); ok {
			pluginType = "output"
			err = output.Run(foRunner, h)
		} else {
			foRunner.LogError(errors.New(
				"Unable to assert this is an Output or Filter"))
			return
		}
		if err != nil {
			foRunner.LogError(err)
		} else {
			foRunner.LogMessage("stopped")
		}

		// Are we supposed to stop? Save ourselves some time by exiting now
		if globals.Stopping {
			return
		}

		// If its a lua sandbox, we let it shut down
		if _, ok := foRunner.plugin.(*SandboxFilter); ok {
			return
		}

		// We stop and let this quit if its not a restarting plugin
		if recon, ok := foRunner.plugin.(Restarting); ok {
			recon.CleanupForRestart()
		} else {
			foRunner.LogMessage("has stopped, shutting down.")
			globals.ShutDown()
			return
		}

		// Re-initialize our plugin using its wrapper
		if pluginType == "filter" {
			pw = pc.filterWrappers[foRunner.name]
		} else {
			pw = pc.outputWrappers[foRunner.name]
		}
		// Attempt to recreate the plugin until it works without error
		// or until we were told to stop
	createLoop:
		for !globals.Stopping {
			err = rh.Wait()
			if err != nil {
				foRunner.LogError(err)
				globals.ShutDown()
				return
			}
			p, err := pw.CreateWithError()
			if err != nil {
				foRunner.LogError(err)
				continue
			}
			foRunner.plugin = p.(Plugin)
			rh.Reset()
			break createLoop
		}
		foRunner.LogMessage("exited, now restarting.")
	}
}

func (foRunner *foRunner) Inject(pack *PipelinePack) bool {
	spec := foRunner.MatchRunner().MatcherSpecification()
	match := spec.Match(pack.Message)
	if match {
		pack.Recycle()
		foRunner.LogError(fmt.Errorf("attempted to Inject a message to itself"))
		return false
	}
	// Do the actual injection in a separate goroutine so we free up the
	// caller; this prevents deadlocks when the caller's InChan is backed up,
	// backing up the router, which would block us here.
	go func() {
		foRunner.h.PipelineConfig().router.InChan() <- pack
	}()
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

func (foRunner *foRunner) RetainPack(pack *PipelinePack) {
	foRunner.retainPack = pack
}

func (foRunner *foRunner) InChan() (inChan chan *PipelinePack) {
	if foRunner.retainPack != nil {
		retainChan := make(chan *PipelinePack)
		go func() {
			retainChan <- foRunner.retainPack
			foRunner.retainPack = nil
			close(retainChan)
		}()
		return retainChan
	}
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
	// Used internally to stamp diagnostic information onto a packet
	diagnostics *PacketTracking
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

// Main function driving Heka execution. Loads config, initializes
// PipelinePack pools, and starts all the runners. Then it listens for signals
// and drives the shutdown process when that is triggered.
func Run(config *PipelineConfig) {
	log.Println("Starting hekad...")

	var outputsWg sync.WaitGroup
	var err error

	globals := Globals()
	sigChan := make(chan os.Signal)
	globals.sigChan = sigChan

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

	// Setup the diagnostic trackers
	inputTracker := NewDiagnosticTracker("input")
	injectTracker := NewDiagnosticTracker("inject")

	// Create the report pipeline pack
	config.reportRecycleChan <- NewPipelinePack(config.reportRecycleChan)

	// Initialize all of the PipelinePacks that we'll need
	for i := 0; i < Globals().PoolSize; i++ {
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

	// wait for sigint
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGHUP, SIGUSR1)

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
			case SIGUSR1:
				log.Println("Queue report initiated.")
				go config.allReportsStdout()
			}
		}
	}

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
	outputsWg.Wait()
	log.Println("Shutdown complete.")
}
