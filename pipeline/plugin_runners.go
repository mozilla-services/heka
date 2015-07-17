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
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/mozilla-services/heka/client"
	"github.com/mozilla-services/heka/message"
)

var ErrUnknownPluginType = errors.New("Unable to assert this is an Output or Filter")

// Base interface for the Heka plugin runners.
type PluginRunner interface {
	// All plugins either can or cannot stop without causing Heka to shutdown.
	Stoppable

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

	// Sets the amount of currently 'leaked' packs that have gone through
	// this plugin. The new value will overwrite prior ones.
	SetLeakCount(count int)

	// Returns the current leak count
	LeakCount() int
}

// Base struct for the specialized PluginRunners
type pRunnerBase struct {
	notStoppable
	name      string
	plugin    Plugin
	h         PluginHelper
	leakCount int
	maker     PluginMaker
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

func (pr *pRunnerBase) SetLeakCount(count int) {
	pr.leakCount = count
}

func (pr *pRunnerBase) LeakCount() int {
	return pr.leakCount
}

// AddDecodeFailureFields adds two fields to the provided message object. The
// first field is a boolean field called `decode_failure`, set to true. The
// second is a string field called `decode_error` which will contain the
// provided error message, truncated to 500 bytes if necessary.
func AddDecodeFailureFields(m *message.Message, errMsg string) error {
	field0, err := message.NewField("decode_failure", true, "")
	if err != nil {
		err = fmt.Errorf("field creation error: %s", err.Error())
		return err
	}
	if len(errMsg) > 500 {
		errMsg = errMsg[:500]
	}
	field1, err := message.NewField("decode_error", errMsg, "")
	if err != nil {
		err = fmt.Errorf("field creation error: %s", err.Error())
		return err
	}
	m.AddField(field0)
	m.AddField(field1)
	return nil
}

type DeliverFunc func(pack *PipelinePack)

type Deliverer interface {
	Deliver(pack *PipelinePack)
	DeliverFunc() DeliverFunc
	Done()
}

type deliverer struct {
	deliver DeliverFunc
	dRunner DecoderRunner
	decoder Decoder
	pConfig *PipelineConfig
}

func (d *deliverer) Deliver(pack *PipelinePack) {
	d.deliver(pack)
}

func (d *deliverer) DeliverFunc() DeliverFunc {
	return d.deliver
}

func (d *deliverer) Done() {
	if d.dRunner != nil {
		d.pConfig.StopDecoderRunner(d.dRunner)
	}

	if d.decoder != nil {
		d.pConfig.allSyncDecodersLock.Lock()
		for i, reporting := range d.pConfig.allSyncDecoders {
			if d.decoder == reporting.decoder {
				d.pConfig.allSyncDecoders = append(d.pConfig.allSyncDecoders[:i], d.pConfig.allSyncDecoders[i+1:]...)
				break
			}
		}
		d.pConfig.allSyncDecodersLock.Unlock()
	}
}

// Heka PluginRunner for Input plugins.
type InputRunner interface {
	PluginRunner
	// InChan returns the input channel from which Inputs can get fresh
	// PipelinePacks, ready to be populated.
	InChan() chan *PipelinePack
	// Input returns the associated Input plugin object.
	Input() Input
	// Ticker returns a ticker channel configured to send ticks at an interval
	// specified by the plugin's ticker_interval config value, if provided.
	Ticker() (ticker <-chan time.Time)
	// Starts Input in a separate goroutine and returns. Should decrement the
	// plugin when the Input stops and the goroutine has completed.
	Start(h PluginHelper, wg *sync.WaitGroup) (err error)
	// Injects PipelinePack into the Heka Router's input channel for delivery
	// to all Filter and Output plugins with corresponding message_matchers.
	Inject(pack *PipelinePack) error
	// If Transient returns true, Heka won't try to keep the Input running,
	// nor will it generate reporting data. Life span and reporting for a
	// transient InputRunner must be managed by the code that creates the
	// runner.
	Transient() bool
	// SetTransient specifies whether or not this Input should be considered
	// transient.
	SetTransient(transient bool)
	// Creates and returns a new Deliverer, creating a new Decoder or
	// DecoderRunner to handle decoding before router injection if necessary.
	// It is the caller's responsibility to call `Done()` when the deliverer
	// is no longer in use to ensure that any DecoderRunner goroutines get
	// cleaned up.
	NewDeliverer(token string) Deliverer
	// Deliver accepts packs from the Input plugin and performs the
	// appropriate one of three possible next actions. Possible actions are 1)
	// placing the pack on the Decoder's input channel, if a decoder is
	// specified and syncDecode is false; 2) synchronously decoding the pack
	// and then placing it on the router's input channel, if a decoder is
	// specified and syncDecode is true; or 3) placing the pack directly on
	// the router's input channel, if no decoder is specified. Delegates to
	// DeliverTo.
	Deliver(pack *PipelinePack)
	NewSplitterRunner(token string) SplitterRunner
	// Tells if synchrounous decode is enabled
	SynchronousDecode() bool
}

type iRunner struct {
	pRunnerBase
	input              Input
	config             CommonInputConfig
	pConfig            *PipelineConfig
	inChan             chan *PipelinePack
	ticker             <-chan time.Time
	transient          bool
	syncDecode         bool
	sendDecodeFailures bool
	deliver            DeliverFunc
	canExit            bool
	shutdownWanters    []WantsDecoderRunnerShutdown
	shutdownLock       sync.Mutex
}

func (ir *iRunner) Ticker() (ticker <-chan time.Time) {
	return ir.ticker
}

func (ir *iRunner) Transient() bool {
	return ir.transient
}

func (ir *iRunner) SetTransient(transient bool) {
	ir.transient = transient
}

func (ir *iRunner) IsStoppable() bool {
	return ir.canExit
}

// Creates and returns a new (not yet started) InputRunner associated w/ the
// provided Input. If transient is true Heka won't try to manage the input's
// life span at all, it's up to the caller to do so.
func NewInputRunner(name string, input Input, config CommonInputConfig) (ir InputRunner) {

	runner := &iRunner{
		pRunnerBase: pRunnerBase{
			name:   name,
			plugin: input.(Plugin),
		},
		input:  input,
		config: config,
	}
	if config.SyncDecode != nil {
		runner.syncDecode = *config.SyncDecode
	}
	if config.SendDecodeFailures != nil {
		runner.sendDecodeFailures = *config.SendDecodeFailures
	}
	if config.CanExit != nil && *config.CanExit {
		runner.canExit = true
	}

	return runner
}

func (ir *iRunner) Input() Input {
	return ir.input
}

func (ir *iRunner) InChan() chan *PipelinePack {
	return ir.inChan
}

func (ir *iRunner) Start(h PluginHelper, wg *sync.WaitGroup) (err error) {
	ir.h = h
	ir.pConfig = h.PipelineConfig()
	ir.inChan = ir.pConfig.inputRecycleChan

	if ir.config.Ticker != 0 {
		tickLength := time.Duration(ir.config.Ticker) * time.Second
		ir.ticker = time.Tick(tickLength)
	}

	if ir.config.Splitter == "" {
		ir.config.Splitter = "NullSplitter"
	}

	ir.pConfig.makersLock.RLock()
	splitters := ir.pConfig.makers["Splitter"]
	splitterMaker, ok := splitters[ir.config.Splitter]
	ir.pConfig.makersLock.RUnlock()
	if !ok {
		return fmt.Errorf("%s specifies undefined splitter %s", ir.name,
			ir.config.Splitter)
	}
	if _, err = splitterMaker.MakeRunner(ir.name); err != nil {
		return fmt.Errorf("%s error creating splitter %s: %s", ir.name,
			ir.config.Splitter, err.Error())
	}

	if ir.config.Decoder != "" {
		_, ok := ir.pConfig.Decoder(ir.config.Decoder)
		if !ok {
			return fmt.Errorf("no registered '%s' decoder", ir.config.Decoder)
		}
	}
	go ir.Starter(h, wg)
	return
}

func (ir *iRunner) Starter(h PluginHelper, wg *sync.WaitGroup) {
	defer wg.Done()

	globals := ir.pConfig.Globals
	rh, err := NewRetryHelper(ir.config.Retries)
	if err != nil {
		ir.LogError(err)
		if !ir.IsStoppable() {
			globals.ShutDown()
		}
		return
	}

	for !globals.IsShuttingDown() {

		// ir.Input().Run() shouldn't return unless error or shutdown.
		err := ir.input.Run(ir, h)
		registered, ok := ir.pConfig.InputRunners[ir.name]

		if !ok || registered != ir || globals.IsShuttingDown() {
			// Plugin was removed deliberately from the list of InputRunners or
			// has been superseded by another instance, or we're in shutdown.
			// In this case, avoid triggering a Heka shutdown ourselves.
			ir.Unregister(ir.pConfig)
			return
		} else if err == nil {
			// Plugin exited cleanly.
			break
		} else {
			// Plugin exited by returning an error.
			ir.LogError(err)

			// If we don't support restart, just stop here.
			recon, ok := ir.plugin.(Restarting)
			if !ok {
				break
			}

			// Otherwise we'll execute the Retry config
			recon.CleanupForRestart()
			if ir.maker == nil {
				ir.pConfig.makersLock.RLock()
				ir.maker = ir.pConfig.makers["Input"][ir.name]
				ir.pConfig.makersLock.RUnlock()
			}

		initLoop:
			if err = rh.Wait(); err != nil {
				// We've used up our retry attempts, exit.
				ir.LogError(err)
				break
			}
			if globals.IsShuttingDown() {
				break
			}
			ir.LogMessage(fmt.Sprintf("Restarting (attempt %d/%d)\n",
				rh.times, rh.retries))

			// If we've not been created elsewhere, call the plugin's Init()
			if !ir.transient {
				if err = ir.plugin.Init(ir.maker.Config()); err != nil {
					// We couldn't reInit the plugin, do a mini-retry loop
					ir.LogError(err)
					goto initLoop
				}
			}

		}
	}

	ir.Unregister(ir.pConfig)

	// If we're not a stoppable input, trigger Heka shutdown.
	if !ir.IsStoppable() {
		globals.ShutDown()
	}
}

func (ir *iRunner) Unregister(pConfig *PipelineConfig) error {

	// Send shutdown signal to any decoders that need it.
	if len(ir.shutdownWanters) > 0 {
		ir.shutdownLock.Lock()
		for _, wanter := range ir.shutdownWanters {
			wanter.Shutdown()
		}
		ir.shutdownLock.Unlock()
	}

	return nil
}

func (ir *iRunner) Inject(pack *PipelinePack) error {
	if err := pack.EncodeMsgBytes(); err != nil {
		err = fmt.Errorf("encoding message: %s", err.Error())
		ir.LogError(err)
		pack.recycle()
		return err
	}
	return ir.pConfig.router.Inject(pack)
}

func (ir *iRunner) LogError(err error) {
	LogError.Printf("Input '%s' error: %s", ir.name, err)
}

func (ir *iRunner) LogMessage(msg string) {
	LogInfo.Printf("Input '%s': %s", ir.name, msg)
}

func (ir *iRunner) getDeliverFunc(token string) (DeliverFunc, DecoderRunner, Decoder) {
	var deliver DeliverFunc
	decoderName := ir.config.Decoder
	// If no decoder is specified we just inject into the router.
	if decoderName == "" {
		deliver = func(pack *PipelinePack) {
			ir.Inject(pack)
		}
		return deliver, nil, nil
	}

	ir.pConfig.makersLock.RLock()
	_, ok := ir.pConfig.DecoderMakers[decoderName]
	ir.pConfig.makersLock.RUnlock()
	if !ok {
		ir.LogError(fmt.Errorf("decoder '%s' not registered", decoderName))
		return nil, nil, nil
	}

	var fullName string
	if token == "" {
		fullName = fmt.Sprintf("%s-%s", ir.name, decoderName)
	} else {
		fullName = fmt.Sprintf("%s-%s-%s", ir.name, decoderName, token)
	}

	// No synchronous decode means create a DecoderRunner and drop packs on
	// its inChan.
	if !ir.syncDecode {
		dr, _ := ir.pConfig.DecoderRunner(decoderName, fullName)
		dr.SetSendFailure(ir.sendDecodeFailures)
		inChan := dr.InChan()
		deliver = func(pack *PipelinePack) {
			inChan <- pack
		}
		return deliver, dr, nil
	}

	// Synchronous decode means create a decoder instance and call Decode
	// directly.
	decoder, _ := ir.pConfig.Decoder(decoderName)
	if wanter, ok := decoder.(WantsDecoderRunner); ok {
		dr := NewDecoderRunner(fullName, decoder, 0).(*dRunner)
		dr.h = ir.h
		dr.router = ir.pConfig.router
		wanter.SetDecoderRunner(dr)
	}
	if wanter, ok := decoder.(WantsDecoderRunnerShutdown); ok {
		ir.shutdownLock.Lock()
		ir.shutdownWanters = append(ir.shutdownWanters, wanter)
		ir.shutdownLock.Unlock()
	}

	ir.pConfig.allSyncDecodersLock.Lock()
	ir.pConfig.allSyncDecoders = append(ir.pConfig.allSyncDecoders, ReportingDecoder{
		name:    fullName,
		decoder: decoder,
	})
	ir.pConfig.allSyncDecodersLock.Unlock()
	// See if the decoder sets TrustMsgBytes for us.
	_, trustMsgBytes := decoder.(EncodesMsgBytes)
	deliver = func(pack *PipelinePack) {
		packs, err := decoder.Decode(pack)
		if err != nil {
			errMsg := err.Error()
			e := fmt.Errorf("decoding: %s", errMsg)
			ir.LogError(e)
			if !ir.sendDecodeFailures {
				pack.recycle()
				return
			}
			if err = AddDecodeFailureFields(pack.Message, errMsg); err != nil {
				ir.LogError(err)
			}
			pack.TrustMsgBytes = false
			ir.Inject(pack)
			return
		}
		for _, p := range packs {
			if !trustMsgBytes {
				p.TrustMsgBytes = false
			}
			ir.Inject(p)
		}
	}
	return deliver, nil, decoder
}

func (ir *iRunner) NewDeliverer(token string) Deliverer {
	deliver, dRunner, decoder := ir.getDeliverFunc(token)
	d := &deliverer{
		deliver: deliver,
		dRunner: dRunner,
		decoder: decoder,
		pConfig: ir.pConfig,
	}
	return d
}

func (ir *iRunner) Deliver(pack *PipelinePack) {
	if ir.deliver == nil {
		ir.deliver, _, _ = ir.getDeliverFunc("")
	}
	ir.deliver(pack)
}

func (ir *iRunner) SynchronousDecode() bool {
	return ir.syncDecode
}

func (ir *iRunner) NewSplitterRunner(token string) SplitterRunner {
	if ir.config.Splitter == "" {
		ir.config.Splitter = "NullSplitter"
	}
	ir.pConfig.makersLock.RLock()
	maker := ir.pConfig.makers["Splitter"][ir.config.Splitter]
	ir.pConfig.makersLock.RUnlock()
	var name string
	if token == "" {
		name = fmt.Sprintf("%s-%s", ir.name, ir.config.Splitter)
	} else {
		name = fmt.Sprintf("%s-%s-%s", ir.name, ir.config.Splitter, token)
	}
	srInterface, _ := maker.MakeRunner(name)
	sr := srInterface.(*sRunner)
	sr.ir = ir
	ir.pConfig.allSplittersLock.Lock()
	ir.pConfig.allSplitters = append(ir.pConfig.allSplitters, sr)
	ir.pConfig.allSplittersLock.Unlock()
	return sr
}

// Heka PluginRunner for Decoder plugins. Decoding is typically a simpler job,
// so these runners handle a bit more than the others.
type DecoderRunner interface {
	PluginRunner
	// Returns associated Decoder plugin object.
	Decoder() Decoder
	// Starts the DecoderRunner so it's listening for incoming PipelinePacks.
	// Should decrement the wait group after shut down has completed.
	Start(h PluginHelper, wg *sync.WaitGroup)
	// Returns the channel into which incoming PipelinePacks to be decoded
	// should be dropped.
	InChan() chan *PipelinePack
	// Returns the running Heka router for direct use by decoder plugins.
	Router() MessageRouter
	// Fetches a new pack from the input supply and returns it to the caller,
	// for decoders that generate multiple messages from a single input
	// message.
	NewPack() *PipelinePack
	// SetSendFailure allows the InputRunner to specify whether or not
	// messages that fail decoding should still be tagged and given to the
	// router.
	SetSendFailure(sendFailure bool)
}

type dRunner struct {
	pRunnerBase
	decoder     Decoder
	inChan      chan *PipelinePack
	router      *messageRouter
	h           PluginHelper
	sendFailure bool
	encodes     bool
	globals     *GlobalConfigStruct
}

// Creates and returns a new (but not yet started) DecoderRunner for the
// provided Decoder plugin.
func NewDecoderRunner(name string, decoder Decoder, chanSize int) DecoderRunner {
	dr := &dRunner{
		pRunnerBase: pRunnerBase{
			name:   name,
			plugin: decoder.(Plugin),
		},
		decoder: decoder,
		inChan:  make(chan *PipelinePack, chanSize),
	}
	_, dr.encodes = decoder.(EncodesMsgBytes)
	return dr
}

func (dr *dRunner) Decoder() Decoder {
	return dr.decoder
}

func (dr *dRunner) Start(h PluginHelper, wg *sync.WaitGroup) {
	dr.h = h
	pConfig := h.PipelineConfig()
	dr.router = pConfig.router
	dr.globals = pConfig.Globals
	if wanter, ok := dr.decoder.(WantsDecoderRunner); ok {
		wanter.SetDecoderRunner(dr)
	}
	go dr.start(h, wg)
}

func (dr *dRunner) start(h PluginHelper, wg *sync.WaitGroup) {
	var (
		pack  *PipelinePack
		packs []*PipelinePack
		err   error
	)
	for pack = range dr.inChan {
		if packs, err = dr.decoder.Decode(pack); packs != nil {
			for _, p := range packs {
				dr.deliver(p)
			}
		} else {
			if err != nil {
				dr.LogError(err)
				if dr.sendFailure {
					if err = AddDecodeFailureFields(pack.Message, err.Error()); err != nil {
						dr.LogError(err)
					}
					pack.TrustMsgBytes = false
					dr.deliver(pack)
					continue
				}
			}
			pack.recycle()
			continue
		}
	}
	if wanter, ok := dr.decoder.(WantsDecoderRunnerShutdown); ok {
		wanter.Shutdown()
	}
	dr.LogMessage("stopped")
	wg.Done()
}

func (dr *dRunner) deliver(pack *PipelinePack) {
	if !dr.encodes || !pack.TrustMsgBytes {
		err := pack.EncodeMsgBytes()
		if err != nil {
			err = fmt.Errorf("encoding message: %s", err.Error())
			dr.LogError(err)
			pack.recycle()
			return
		}
	}
	dr.router.Inject(pack)
}

func (dr *dRunner) InChan() chan *PipelinePack {
	return dr.inChan
}

func (dr *dRunner) Router() MessageRouter {
	return dr.router
}

func (dr *dRunner) NewPack() *PipelinePack {
	var pack *PipelinePack
	select {
	case pack = <-dr.h.PipelineConfig().inputRecycleChan:
	case <-dr.globals.abortChan:
	}
	return pack // Might be nil if we're aborting.
}

func (dr *dRunner) LogError(err error) {
	LogError.Printf("Decoder '%s' error: %s", dr.name, err)
}

func (dr *dRunner) LogMessage(msg string) {
	LogInfo.Printf("Decoder '%s': %s", dr.name, msg)
}

func (dr *dRunner) SetSendFailure(sendFailure bool) {
	dr.sendFailure = sendFailure
}

// Any decoder that needs access to its DecoderRunner can implement this
// interface and it will be provided at DecoderRunner start time.
type WantsDecoderRunner interface {
	SetDecoderRunner(dr DecoderRunner)
}

// Any decoder that needs to know when the DecoderRunner is exiting can
// implement this interface and it will be called on DecoderRunner exit.
type WantsDecoderRunnerShutdown interface {
	Shutdown()
}

// Heka PluginRunner interface for Filter type plugins.
type FilterRunner interface {
	PluginRunner
	// Input channel on which the Filter should listen for incoming messages
	// to be processed. Closure of the channel signals shutdown to the filter.
	InChan() chan *PipelinePack
	// Channel that will be closed when the Filter is exiting, should be used
	// by Filter plugins in situations that might block to ensure that shutdown
	// messages are received.
	StopChan() chan bool
	// Associated Filter plugin object.
	Filter() Filter
	// Associated Filter plugin object using the deprecated API.
	OldFilter() OldFilter
	// Starts the Filter (so it's listening on the input channel for messages
	// to be processed) in a separate goroutine and returns. Should decrement
	// the wait group when the Filter has stopped and the goroutine has
	// completed.
	Start(h PluginHelper, wg *sync.WaitGroup) (err error)
	// Returns a ticker channel configured to send ticks at an interval
	// specified by the plugin's ticker_interval config value, if provided.
	Ticker() (ticker <-chan time.Time)
	// Hands provided PipelinePack to the Heka Router for delivery to any
	// Filter or Output plugins with a corresponding message_matcher. Returns
	// false and doesn't perform message injection if the message would be
	// caught by the sending Filter's message_matcher.
	Inject(pack *PipelinePack) bool
	// Parsing engine for this Filter's message_matcher.
	MatchRunner() *MatchRunner
	// Retains a pack for future delivery to the plugin when a plugin needs to
	// shut down and wants to retain the pack for the next time its running
	// properly.
	RetainPack(pack *PipelinePack)
	// Returns whether or not use_buffering was set in the filter's
	// configuration
	UsesBuffering() bool
	// Updates the queue buffer cursor to indicate that a given message has
	// been delivered and can be safely removed from the queue.
	UpdateCursor(queueCursor string)
	// Returns whether or not the filter is currently generating back-pressure,
	// either through the input channels being full or through the disk buffer
	// up to 90% of the configured max.
	BackPressured() bool
}

// Heka PluginRunner for Output plugins.
type OutputRunner interface {
	PluginRunner
	// Input channel where Output should be listening for incoming messages.
	InChan() chan *PipelinePack
	// Channel that will be closed when the Output is exiting, should be used
	// by Output plugins in situations that might block to ensure that shutdown
	// messages are received.
	StopChan() chan bool
	// Associated Output plugin instance.
	Output() Output
	// Associated Output plugin instance using the deprecated API.
	OldOutput() OldOutput
	// Starts the Output plugin listening on the input channel in a separate
	// goroutine and returns. Wait group should be released when the Output
	// plugin shuts down cleanly and the goroutine has completed.
	Start(h PluginHelper, wg *sync.WaitGroup) (err error)
	// Returns a ticker channel configured to send ticks at an interval
	// specified by the plugin's ticker_interval config value, if provided.
	Ticker() (ticker <-chan time.Time)
	// Retains a pack for future delivery to the plugin when a plugin needs
	// to shut down and wants to retain the pack for the next time its
	// running properly
	RetainPack(pack *PipelinePack)
	// Parsing engine for this Output's message_matcher.
	MatchRunner() *MatchRunner
	// Returns an instance of the Encoder specified by the output's config, or
	// nil if none was specified. Multiple calls will return the same
	// instance.
	Encoder() Encoder
	// Uses the output's Encoder to encode the message attached to the
	// provided PipelinePack. Will prepend a Heka stream framing header if
	// use_framing was set to true in the output configuration.
	Encode(pack *PipelinePack) (output []byte, err error)
	// Returns whether or not use_framing was set to true in the output's
	// configuration, i.e. whether or not Heka stream framing will be applied
	// to the results of calls to the Encode method.
	UsesFraming() bool
	// Allows an output to specify whether or not it's using framing.
	SetUseFraming(useFraming bool)
	// Returns whether or not use_buffering was set in the output's
	// configuration
	UsesBuffering() bool
	// Updates the queue buffer cursor to indicate that a given message has
	// been delivered and can be safely removed from the queue.
	UpdateCursor(queueCursor string)
	// Returns whether or not the output is currently generating back-pressure,
	// either through the input channels being full or through the disk buffer
	// up to 90% of the configured max.
	BackPressured() bool
}

type foRunnerKind int

const (
	foUnknown foRunnerKind = iota
	foFilter
	foOutput
)

func (kind foRunnerKind) String() string {
	switch kind {
	case foFilter:
		return "filter"
	case foOutput:
		return "output"
	}
	return "unknown"
}

// This one struct provides the implementation of both FilterRunner and
// OutputRunner interfaces.
type foRunner struct {
	processMessageCount int64
	dropMessageCount    int64
	capacity            int
	pRunnerBase
	pluginType   string
	config       CommonFOConfig
	matcher      *MatchRunner
	ticker       <-chan time.Time
	inChan       chan *PipelinePack
	backChan     chan *PipelinePack
	h            PluginHelper
	retainPack   *PipelinePack
	leakCount    int
	encoder      Encoder // output only
	useFraming   bool    // output only
	canExit      bool
	useBuffering bool
	kind         foRunnerKind
	pConfig      *PipelineConfig
	lastErr      error
	bufReader    *BufferReader
	stopChan     chan bool
}

const pluginPoolSize = 2

// Creates and returns foRunner pointer for use as either a FilterRunner or an
// OutputRunner.
func NewFORunner(name string, plugin Plugin, config CommonFOConfig,
	pluginType string, chanSize int) (*foRunner, error) {

	runner := &foRunner{
		pRunnerBase: pRunnerBase{
			name:   name,
			plugin: plugin,
		},
		pluginType: pluginType,
		config:     config,
	}

	if config.Matcher == "" {
		return nil, fmt.Errorf("'%s' missing message matcher", name)
	}

	if config.Ticker != 0 {
		canTick := false
		// Check to make sure we know what to do with the ticker interval.
		if _, ok := plugin.(OldFilter); ok {
			canTick = true
		} else if _, ok := plugin.(OldOutput); ok {
			canTick = true
		} else if _, ok := plugin.(TickerPlugin); ok {
			canTick = true
		}
		if !canTick {
			return nil, fmt.Errorf("'%s' can't support a ticker_interval setting", name)
		}
	}

	if config.UseBuffering != nil && *config.UseBuffering {
		runner.useBuffering = true
		if config.Buffering.FullAction == "" {
			config.Buffering.FullAction = "shutdown"
		}
		switch config.Buffering.FullAction {
		case "shutdown", "drop", "block":
		default:
			msg := "buffer full_action must be 'shutdown', 'drop', or 'block', got '%s'"
			return nil, fmt.Errorf(msg, config.Buffering.FullAction)
		}
		runner.capacity = int(config.Buffering.MaxBufferSize) * 90 / 100
	}

	var matchChan chan *PipelinePack
	if runner.useBuffering {
		runner.inChan = make(chan *PipelinePack, pluginPoolSize)
		runner.backChan = make(chan *PipelinePack, pluginPoolSize)
		for i := 0; i < pluginPoolSize; i++ {
			pack := NewPipelinePack(runner.backChan)
			pack.BufferedPack = true
			pack.DelivErrChan = make(chan error, 1)
			runner.backChan <- pack
		}
	} else {
		runner.inChan = make(chan *PipelinePack, chanSize)
		matchChan = runner.inChan
		runner.capacity = chanSize
	}
	// matchChan is nil if buffering is used, this is intentional.
	matcher, err := NewMatchRunner(config.Matcher, config.Signer, runner, chanSize,
		matchChan)
	if err != nil {
		return nil, fmt.Errorf("Can't create message matcher for '%s': %s", name, err)
	}
	runner.matcher = matcher

	if config.CanExit != nil && *config.CanExit {
		runner.canExit = true
	}

	if config.UseFraming != nil && *config.UseFraming {
		runner.useFraming = true
	}

	if _, ok := plugin.(OldFilter); ok {
		runner.kind = foFilter
	} else if _, ok := plugin.(OldOutput); ok {
		runner.kind = foOutput
	} else if _, ok := plugin.(Filter); ok {
		runner.kind = foFilter
	} else if _, ok := plugin.(Output); ok {
		runner.kind = foOutput
	} else {
		err := fmt.Errorf("FORunner plugin must be filter or output, %s is neither",
			name)
		return nil, err
	}

	return runner, nil
}

func (foRunner *foRunner) BackPressured() bool {
	if !foRunner.useBuffering {
		// reading a channel length is generally fast ~1ns
		// we need to check the entire chain back to the router
		return len(foRunner.inChan) >= foRunner.capacity ||
			foRunner.matcher.InChanLen() >= foRunner.capacity
	}

	return foRunner.capacity > 0 && foRunner.bufReader.queueSize.Get() >= uint64(foRunner.capacity)
}

func (foRunner *foRunner) Start(h PluginHelper, wg *sync.WaitGroup) (err error) {
	foRunner.h = h
	foRunner.pConfig = h.PipelineConfig()

	if foRunner.pluginType == "SandboxFilter" {
		// No maker means we're a dynamic filter and we can exit.
		foRunner.pConfig.makersLock.RLock()
		maker := foRunner.pConfig.makers["Filter"][foRunner.name]
		foRunner.pConfig.makersLock.RUnlock()
		if maker == nil {
			foRunner.canExit = true
		}
	}

	if foRunner.config.Ticker != 0 {
		tickLength := time.Duration(foRunner.config.Ticker) * time.Second
		foRunner.ticker = time.Tick(tickLength)
	}

	if foRunner.config.Encoder != "" {
		fullName := fmt.Sprintf("%s-%s", foRunner.name, foRunner.config.Encoder)
		encoder, ok := foRunner.pConfig.Encoder(foRunner.config.Encoder, fullName)
		if !ok {
			return fmt.Errorf("%s can't create encoder %s", foRunner.name,
				foRunner.config.Encoder)
		}
		foRunner.encoder = encoder
	}

	var bufFeeder *BufferFeeder
	if foRunner.useBuffering {
		bufFeeder, foRunner.bufReader, err = NewBufferSet("output_queue", foRunner.name,
			foRunner.config.Buffering, foRunner, foRunner.pConfig)
		if err != nil {
			return fmt.Errorf("can't initialize buffer: %s", err)
		}
	}

	foRunner.stopChan = make(chan bool)

	if foRunner.matcher != nil {
		foRunner.matcher.bufFeeder = bufFeeder
		foRunner.matcher.globals = foRunner.pConfig.Globals
		foRunner.matcher.stopChan = foRunner.stopChan
		switch foRunner.kind {
		case foFilter:
			foRunner.pConfig.router.fMatcherMap[foRunner.name] = foRunner.matcher
		case foOutput:
			foRunner.pConfig.router.oMatcherMap[foRunner.name] = foRunner.matcher
		}
	}

	newStyleAPI := false
	switch foRunner.kind {
	case foFilter:
		if _, ok := foRunner.plugin.(Filter); ok {
			newStyleAPI = true
		}
	case foOutput:
		if _, ok := foRunner.plugin.(Output); ok {
			newStyleAPI = true
		}
	}

	if newStyleAPI {
		plugin, ok := foRunner.plugin.(MessageProcessor)
		if !ok {
			return errors.New("Not a new-style plugin.")
		}
		go foRunner.Starter(plugin, h, wg)
	} else {
		go foRunner.OldStarter(h, wg)
	}
	return
}

// bufferLoop is invoked for plugins that support the newer API when buffering
// is turned on.
func (foRunner *foRunner) bufferLoop(plugin MessageProcessor, h PluginHelper,
	tickReceiver TickerPlugin) error {

	err := foRunner.bufReader.NewStreamOutput(plugin, foRunner.backChan, tickReceiver,
		foRunner.ticker, foRunner.stopChan)
	if err != nil {
		foRunner.LogError(fmt.Errorf("StreamOutput stopped: %s", err.Error()))
	}
	return err
}

// channelLoop is invoked for plugins that support the newer API when buffering
// is not turned on.
func (foRunner *foRunner) channelLoop(plugin MessageProcessor, h PluginHelper,
	tickReceiver TickerPlugin) error {

	rh, _ := NewRetryHelper(RetryOptions{
		MaxDelay:   "1s",
		Delay:      "10ms",
		MaxRetries: -1,
	})

	resetNeeded := false
	ok := true
	var pack *PipelinePack
	for ok {
		if resetNeeded {
			rh.Reset()
		}
		select {
		case pack, ok = <-foRunner.inChan:
			if !ok {
				break
			}
		RetryLoop:
			for !foRunner.pConfig.Globals.IsShuttingDown() {
				err := plugin.ProcessMessage(pack)
				if err == nil {
					pack.recycle()
					break RetryLoop // Bumps us back to the outer loop.
				}
				switch err.(type) {
				case PluginExitError:
					pack.recycle()
					return err
				case RetryMessageError:
					foRunner.LogError(err)
					rh.Wait()
					resetNeeded = true
					continue // Try the same one again.
				default:
					foRunner.LogError(err)
					pack.recycle()
					break RetryLoop
				}
			}
		case <-foRunner.ticker:
			if tickReceiver == nil {
				// Again, this shouldn't happen.
				panic(fmt.Sprintf("Not a TickerPlugin: %s", foRunner.name))
			}
			err := tickReceiver.TimerEvent()
			if err != nil {
				err = fmt.Errorf("Error running TimerEvent for %s: %s",
					foRunner.name, err.Error())
				if _, isFatal := err.(PluginExitError); isFatal {
					return err
				}
			}
		}
	}

	return nil
}

// Starter is the main goroutine launched for plugins that support the newer
// API.
func (foRunner *foRunner) Starter(plugin MessageProcessor, h PluginHelper,
	wg *sync.WaitGroup) {

	defer wg.Done()

	globals := foRunner.pConfig.Globals
	if foRunner.matcher != nil {
		foRunner.matcher.Start(globals.SampleDenominator)
	}

	var (
		tickReceiver TickerPlugin
		ok           bool
		err          error
	)

	if foRunner.ticker != nil {
		tickReceiver, ok = plugin.(TickerPlugin)
		if !ok {
			// This shouldn't happen, config validation should prevent a non-
			// ticker plugin w/o a TimerEvent method from getting this far.
			panic(fmt.Sprintf("Not a TickerPlugin: %s", foRunner.name))
		}
	}

	rh, err := NewRetryHelper(foRunner.config.Retries)
	if err != nil {
		foRunner.LogError(err)
		if !foRunner.IsStoppable() {
			globals.ShutDown()
		}
		return
	}

	defer foRunner.exit()

	// Initial Prepare loop.
	resetNeeded := false
	for {
		switch foRunner.kind {
		case foFilter:
			f := foRunner.plugin.(Filter)
			err = f.Prepare(foRunner, h)
		case foOutput:
			o := foRunner.plugin.(Output)
			err = o.Prepare(foRunner, h)
		}

		if err == nil {
			if resetNeeded {
				rh.Reset()
			}
			break
		}

		// Prepare returned an error. Log the error and try again.
		foRunner.LogError(err)
		if globals.IsShuttingDown() {
			foRunner.lastErr = err
			return
		}

		resetNeeded = true
		if e := rh.Wait(); e != nil {
			// No more retries.
			foRunner.lastErr = err
			if !foRunner.IsStoppable() {
				globals.ShutDown()
			}
			return
		}
	}

	for !globals.IsShuttingDown() {
		if foRunner.useBuffering {
			err = foRunner.bufferLoop(plugin, h, tickReceiver)
		} else {
			err = foRunner.channelLoop(plugin, h, tickReceiver)
		}

		switch foRunner.kind {
		case foFilter:
			f := foRunner.plugin.(Filter)
			f.CleanUp()
		case foOutput:
			o := foRunner.plugin.(Output)
			o.CleanUp()
		}

		if err == nil {
			rh.Reset()
		} else {
			// Keep track of all the errors for later.
			foRunner.lastErr = err
			foRunner.LogError(err)
		}

		foRunner.LogMessage("stopped")

		// Are we shutting down? Save ourselves some time by exiting now.
		if globals.IsShuttingDown() {
			break
		}

		// We stop and let this quit if its not a restarting plugin.
		recon, ok := foRunner.plugin.(Restarting)
		if !ok {
			break
		}
		recon.CleanupForRestart()
		if foRunner.maker == nil {
			var makers map[string]PluginMaker
			foRunner.pConfig.makersLock.RLock()
			switch foRunner.kind {
			case foFilter:
				makers = foRunner.pConfig.makers["Filter"]
			case foOutput:
				makers = foRunner.pConfig.makers["Output"]
			}
			foRunner.maker = makers[foRunner.name]
			foRunner.pConfig.makersLock.RUnlock()
		}

	initLoop:
		if err = rh.Wait(); err != nil {
			// An error means we've used up our retry attempts, so we
			// exit.
			foRunner.lastErr = err
			foRunner.LogError(err)
			break
		}
		if globals.IsShuttingDown() {
			break
		}
		foRunner.LogMessage("now restarting")
		if err = foRunner.plugin.Init(foRunner.maker.Config()); err != nil {
			foRunner.LogError(err)
			goto initLoop
		}
		switch foRunner.kind {
		case foFilter:
			f := foRunner.plugin.(Filter)
			err = f.Prepare(foRunner, foRunner.h)
		case foOutput:
			o := foRunner.plugin.(Output)
			err = o.Prepare(foRunner, foRunner.h)
		}
		if err != nil {
			foRunner.LogError(err)
			goto initLoop
		}
	}
}

func (foRunner *foRunner) IsStoppable() bool {
	return foRunner.canExit
}

func (foRunner *foRunner) Unregister(pConfig *PipelineConfig) error {
	switch foRunner.kind {
	case foFilter:
		go pConfig.RemoveFilterRunner(foRunner.Name())
	case foOutput:
		go pConfig.RemoveOutputRunner(foRunner)
	}
	return nil
}

func (foRunner *foRunner) exit() {
	if !foRunner.useBuffering {
		defer func() {
			var orphaned int
			for pack := range foRunner.inChan {
				// drain and recycle the orphaned packs
				orphaned++
				pack.recycle()
			}
			if orphaned == 1 {
				foRunner.LogError(fmt.Errorf("Lost/Dropped 1 message"))
			} else if orphaned > 1 {
				foRunner.LogError(fmt.Errorf("Lost/Dropped %d messages", orphaned))
			}
		}()
	}
	// Just exit if we're stopping.
	if foRunner.pConfig.Globals.IsShuttingDown() {
		return
	}

	// Also, if this isn't a "stoppable" plugin we shut everything down.
	if !foRunner.IsStoppable() {
		foRunner.LogMessage("has stopped, shutting down.")
		foRunner.pConfig.Globals.ShutDown()
		return
	}

	// If we're stoppable we unregister the plugin and, if necessary, send a
	// termination message.
	foRunner.LogMessage("has stopped, exiting plugin without shutting down.")
	foRunner.Unregister(foRunner.pConfig)

	// A TerminatedError means the plugin was terminated, and has generated
	// its own termination message, so we can return now.
	if _, ok := foRunner.lastErr.(TerminatedError); ok {
		return
	}

	pack, e := foRunner.pConfig.PipelinePack(0)
	if e != nil {
		LogError.Printf("can't generate termination message: %s", e.Error())
		return
	}
	pack.Message.SetType("heka.terminated")
	pack.Message.SetLogger(HEKA_DAEMON)
	message.NewStringField(pack.Message, "plugin", foRunner.name)

	var errMsg string
	if foRunner.lastErr != nil {
		errMsg = foRunner.lastErr.Error()
	} else if foRunner.pluginType == "SandboxFilter" {
		errMsg = "Filter unloaded."
	} else {
		// This is simply when run returns no errors, but the plugin has
		// exited anyway.
		errMsg = "Run exited."
	}
	errMsg = "Error: " + errMsg

	payload := fmt.Sprintf("%s (type %s) terminated. %s", foRunner.name,
		foRunner.pluginType, errMsg)
	pack.Message.SetPayload(payload)
	// Do not call the Inject method b/c we might get burned by the explicit
	// message matcher check in cases where it looks like we'd be injecting a
	// message to ourself.
	pack.EncodeMsgBytes()
	foRunner.pConfig.router.inChan <- pack
}

// runBoth manages lifespan and return codes for both the plugin's Run method
// and the BufferReader's streamOutput method for plugins that use the older
// API.
func (foRunner *foRunner) runBoth(h PluginHelper) error {
	pluginErrChan := make(chan error, 1)
	bufErrChan := make(chan error, 1)

	// Start plugin.
	go func() {
		var err error
		switch foRunner.kind {
		case foFilter:
			filter := foRunner.OldFilter()
			err = filter.Run(foRunner, h)
		case foOutput:
			output := foRunner.OldOutput()
			err = output.Run(foRunner, h)
		}
		pluginErrChan <- err
	}()

	// Start buffer reader.
	go func() {
		err := foRunner.bufReader.StreamOutput(foRunner, foRunner.backChan,
			foRunner.stopChan)
		bufErrChan <- err
	}()

	var pluginErr, bufErr error
	select {
	case pluginErr = <-pluginErrChan:
	case bufErr = <-bufErrChan:
	}

	shuttingDown := foRunner.pConfig.Globals.IsShuttingDown()
	if pluginErr == nil {
		close(foRunner.inChan) // Triggers plugin exit.
		pluginErr = <-pluginErrChan
		foRunner.inChan = make(chan *PipelinePack, pluginPoolSize)
	} else {
		if !shuttingDown {
			close(foRunner.stopChan) // Triggers bufReader exit.
		}
		bufErr = <-bufErrChan
		foRunner.stopChan = make(chan bool)
	}

	var err error
	if pluginErr != nil && bufErr != nil {
		err = fmt.Errorf("plugin error: %s\nbuffer reader error: %s", pluginErr,
			bufErr)
	} else if pluginErr != nil {
		err = fmt.Errorf("plugin error: %s", pluginErr)
	} else if bufErr != nil {
		err = fmt.Errorf("buffer reader error: %s", bufErr)
	}
	return err
}

// OldStarter is the main goroutine driving plugins that support the older API.
func (foRunner *foRunner) OldStarter(helper PluginHelper, wg *sync.WaitGroup) {
	defer wg.Done()

	var err error
	globals := foRunner.pConfig.Globals

	rh, err := NewRetryHelper(foRunner.config.Retries)
	if err != nil {
		foRunner.LogError(err)
		if !foRunner.IsStoppable() {
			globals.ShutDown()
		}
		return
	}

	if foRunner.matcher != nil {
		foRunner.matcher.Start(globals.SampleDenominator)
	}

	// Handle the cleanup
	defer foRunner.exit()

	for !globals.IsShuttingDown() {
		if foRunner.useBuffering {
			// Only returns if there's an error or we're shutting down.
			err = foRunner.runBoth(helper)
		} else {
			// Run only returns if there's an error or we're shutting down.
			switch foRunner.kind {
			case foFilter:
				filter := foRunner.OldFilter()
				err = filter.Run(foRunner, helper)
			case foOutput:
				output := foRunner.OldOutput()
				err = output.Run(foRunner, helper)
			}
		}

		if err == nil {
			rh.Reset()
		} else {
			// Keep track of all the errors for later
			foRunner.lastErr = err
			foRunner.LogError(err)
		}

		foRunner.LogMessage("stopped")

		// Are we supposed to stop? Save ourselves some time by exiting now.
		if globals.IsShuttingDown() {
			break
		}

		// We stop and let this quit if its not a restarting plugin.
		recon, ok := foRunner.plugin.(Restarting)
		if !ok {
			break
		}
		recon.CleanupForRestart()
		if foRunner.maker == nil {
			var makers map[string]PluginMaker
			foRunner.pConfig.makersLock.RLock()
			switch foRunner.kind {
			case foFilter:
				makers = foRunner.pConfig.makers["Filter"]
			case foOutput:
				makers = foRunner.pConfig.makers["Output"]
			}
			foRunner.maker = makers[foRunner.name]
			foRunner.pConfig.makersLock.RUnlock()
		}
	initLoop:
		if err = rh.Wait(); err != nil {
			// An error means we've used up our retry attempts, so we
			// exit.
			foRunner.lastErr = err
			foRunner.LogError(err)
			break
		}
		if globals.IsShuttingDown() {
			break
		}
		foRunner.LogMessage("now restarting")
		if err = foRunner.plugin.Init(foRunner.maker.Config()); err != nil {
			foRunner.LogError(err)
			goto initLoop
		}
	}
}

// Message sending function for buffered plugins using the old-style API.
func (foRunner foRunner) SendRecord(pack *PipelinePack) error {
	select {
	case foRunner.inChan <- pack:
		// Wait until pack is delivered.
		select {
		case err := <-pack.DelivErrChan:
			if err == nil {
				atomic.AddInt64(&foRunner.processMessageCount, 1)
				pack.recycle()
			} else {
				if _, ok := err.(RetryMessageError); !ok {
					foRunner.LogError(fmt.Errorf("can't send record: %s", err))
					atomic.AddInt64(&foRunner.dropMessageCount, 1)
					pack.recycle()
					err = nil // Swallow the error so there's no retry.
				}
			}
			return err
		case <-foRunner.stopChan:
			pack.recycle()
			return ErrStopping
		}
	case <-foRunner.stopChan:
		pack.recycle()
		return ErrStopping
	}
}

func (foRunner *foRunner) UpdateCursor(queueCursor string) {
	if foRunner.bufReader == nil {
		return
	}
	err := foRunner.bufReader.updateCursor(queueCursor)
	if err != nil {
		foRunner.LogError(fmt.Errorf("updating buffer cursor: %s", err))
	}
}

func (foRunner *foRunner) Inject(pack *PipelinePack) bool {
	if pack.BufferedPack {
		foRunner.LogError(errors.New("can't inject buffered plugin pack"))
		return false
	}
	// Make sure we're not creating an obvious infinite routing loop.
	spec := foRunner.MatchRunner().MatcherSpecification()
	match := spec.Match(pack.Message)
	if match {
		foRunner.LogError(errors.New("attempted to Inject a message to itself"))
		pack.recycle()
		return false
	}
	// Make sure the pack's MsgBytes is populated.
	err := pack.EncodeMsgBytes()
	if err != nil {
		foRunner.LogError(fmt.Errorf("encoding message: %s", err.Error()))
		pack.recycle()
		return false
	}
	// Do the actual injection in a separate goroutine so we free up the
	// caller; this prevents deadlocks when the caller's InChan is backed up,
	// backing up the router, which would block us here.
	go func() {
		foRunner.h.PipelineConfig().router.Inject(pack)
	}()
	return true
}

func (foRunner *foRunner) LogError(err error) {
	LogError.Printf("Plugin '%s' error: %s", foRunner.name, err)
}

func (foRunner *foRunner) LogMessage(msg string) {
	LogInfo.Printf("Plugin '%s': %s", foRunner.name, msg)
}

func (foRunner *foRunner) StopChan() chan bool {
	return foRunner.stopChan
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

		go func(pack *PipelinePack) {
			retainChan <- pack
			close(retainChan)
		}(foRunner.retainPack)

		foRunner.retainPack = nil
		return retainChan
	}
	return foRunner.inChan
}

func (foRunner *foRunner) MatchRunner() *MatchRunner {
	return foRunner.matcher
}

func (foRunner *foRunner) SetMatchRunner(mr *MatchRunner) {
	foRunner.matcher = mr
}

func (foRunner *foRunner) Filter() Filter {
	return foRunner.plugin.(Filter)
}

func (foRunner *foRunner) Output() Output {
	return foRunner.plugin.(Output)
}

func (foRunner *foRunner) OldFilter() OldFilter {
	return foRunner.plugin.(OldFilter)
}

func (foRunner *foRunner) OldOutput() OldOutput {
	return foRunner.plugin.(OldOutput)
}

func (foRunner *foRunner) Encoder() Encoder {
	return foRunner.encoder
}

func (foRunner *foRunner) Encode(pack *PipelinePack) (output []byte, err error) {
	var encoded []byte
	if encoded, err = foRunner.encoder.Encode(pack); err != nil || encoded == nil {
		return
	}
	if foRunner.useFraming {
		client.CreateHekaStream(encoded, &output, nil)
	} else {
		output = encoded
	}
	return
}

func (foRunner *foRunner) UsesFraming() bool {
	return foRunner.useFraming
}

func (foRunner *foRunner) SetUseFraming(useFraming bool) {
	foRunner.useFraming = useFraming
}

func (foRunner *foRunner) UsesBuffering() bool {
	return foRunner.useBuffering
}

type PluginExitError struct {
	msg string
}

func NewPluginExitError(msg string, subs ...interface{}) PluginExitError {
	if len(subs) > 0 {
		msg = fmt.Sprintf(msg, subs...)
	}
	return PluginExitError{msg}
}

func (err PluginExitError) Error() string {
	return err.msg
}

type RetryMessageError struct {
	msg string
}

func NewRetryMessageError(msg string, subs ...interface{}) RetryMessageError {
	if len(subs) > 0 {
		msg = fmt.Sprintf(msg, subs...)
	}
	return RetryMessageError{msg}
}

func (err RetryMessageError) Error() string {
	return err.msg
}
