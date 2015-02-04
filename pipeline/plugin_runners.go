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
	"errors"
	"fmt"
	"github.com/mozilla-services/heka/client"
	"github.com/mozilla-services/heka/message"
	"sync"
	"time"
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
	Inject(pack *PipelinePack)
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
	// placing the pack on a Decoder's input channel, if a decoder is
	// specified and syncDecode is false; 2) synchronously decoding the pack
	// and then placing it on the router's input channel, if a decoder is
	// specified and syncDecode is true; or 3) placing the pack directly on
	// the router's input channel, if no decoder is specified.
	Deliver(pack *PipelinePack)
	// UseMsgBytes returns true if the InputRunner has a decoder that expects
	// to find the raw input data in the PipelinePack's `MsgBytes` attribute,
	// which is typically a ProtobufDecoder. Returns false otherwise.
	UseMsgBytes() bool
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
	useMsgBytes        bool
	deliver            DeliverFunc
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

	// This bit is a bit kludgey; we're creating a decoder just to see if it's
	// a ProtobufDecoder or a MultiDecoder starting with a ProtobufDecoder,
	// and if so we set useMsgBytes to true. This should go away entirely when
	// the splitter branch lands, this is just to keep the dev branch working
	// in the meantime.
	if ir.config.Decoder != "" {
		decoder, ok := ir.pConfig.Decoder(ir.config.Decoder)
		if !ok {
			return fmt.Errorf("%s can't create decoder %s", ir.name, ir.config.Decoder)
		}
		_, ok = decoder.(*ProtobufDecoder)
		if ok {
			ir.useMsgBytes = true
		} else if d, ok := decoder.(*MultiDecoder); ok {
			if len(d.Decoders) > 0 {
				_, ok = d.Decoders[0].(*ProtobufDecoder)
				if ok {
					ir.useMsgBytes = true
				}
			}
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
		globals.ShutDown()
		return
	}

	for !globals.IsShuttingDown() {
		// ir.Input().Run() shouldn't return unless error or shutdown.
		err := ir.input.Run(ir, h)
		if err != nil {
			ir.LogError(err)
		} else if !ir.transient {
			rh.Reset()
			ir.LogMessage("stopped")
		}

		// Send shutdown signal to any decoders that need it.
		if len(ir.shutdownWanters) > 0 {
			ir.shutdownLock.Lock()
			for _, wanter := range ir.shutdownWanters {
				wanter.Shutdown()
			}
			ir.shutdownLock.Unlock()
		}

		// Are we supposed to stop? Save ourselves some time by exiting now.
		if globals.IsShuttingDown() {
			break
		}

		// If we're transient we just exit.
		if ir.transient {
			break
		}
		// We're not transient, we either try to restart or we shut down
		// altogether.
		recon, ok := ir.plugin.(Restarting)
		if !ok {
			ir.LogMessage("has stopped, shutting down.")
			globals.ShutDown()
			break
		}

		recon.CleanupForRestart()
		if ir.maker == nil {
			ir.maker = ir.pConfig.makers["Input"][ir.name]
		}
	initLoop:
		if err = rh.Wait(); err != nil {
			// We've used up our retry attempts, exit.
			ir.LogError(err)
			globals.ShutDown()
			break
		}
		ir.LogMessage("exited, now restarting.")
		if err = ir.plugin.Init(ir.maker.Config()); err != nil {
			ir.LogError(err)
			goto initLoop
		}
	}
}

func (ir *iRunner) Inject(pack *PipelinePack) {
	ir.pConfig.router.InChan() <- pack
}

func (ir *iRunner) LogError(err error) {
	LogError.Printf("Input '%s' error: %s", ir.name, err)
}

func (ir *iRunner) LogMessage(msg string) {
	LogInfo.Printf("Input '%s': %s", ir.name, msg)
}

func (ir *iRunner) UseMsgBytes() bool {
	return ir.useMsgBytes
}

func (ir *iRunner) getDeliverFunc(token string) (DeliverFunc, DecoderRunner) {
	var deliver DeliverFunc
	decoderName := ir.config.Decoder

	// If no decoder is specified we just inject into the router.
	if decoderName == "" {
		deliver = func(pack *PipelinePack) {
			ir.Inject(pack)
		}
		return deliver, nil
	}

	ir.pConfig.makersLock.RLock()
	_, ok := ir.pConfig.DecoderMakers[decoderName]
	ir.pConfig.makersLock.RUnlock()
	if !ok {
		ir.LogError(fmt.Errorf("decoder '%s' not registered", decoderName))
		return nil, nil
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
		return deliver, dr
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
	deliver = func(pack *PipelinePack) {
		packs, err := decoder.Decode(pack)
		if err != nil {
			errMsg := err.Error()
			ir.LogError(fmt.Errorf("decoding: %s", errMsg))
			if !ir.sendDecodeFailures {
				pack.Recycle()
				return
			}
			if err = AddDecodeFailureFields(pack.Message, errMsg); err != nil {
				ir.LogError(err)
			}
			ir.Inject(pack)
			return
		}
		for _, p := range packs {
			ir.Inject(p)
		}
	}
	return deliver, nil
}

func (ir *iRunner) NewDeliverer(token string) Deliverer {
	deliver, dRunner := ir.getDeliverFunc(token)
	d := &deliverer{
		deliver: deliver,
		dRunner: dRunner,
		pConfig: ir.pConfig,
	}
	return d
}

func (ir *iRunner) Deliver(pack *PipelinePack) {
	if ir.deliver == nil {
		ir.deliver, _ = ir.getDeliverFunc("")
	}
	ir.deliver(pack)
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
	inChan      chan *PipelinePack
	router      *messageRouter
	h           PluginHelper
	sendFailure bool
}

// Creates and returns a new (but not yet started) DecoderRunner for the
// provided Decoder plugin.
func NewDecoderRunner(name string, decoder Decoder, chanSize int) DecoderRunner {
	return &dRunner{
		pRunnerBase: pRunnerBase{
			name:   name,
			plugin: decoder.(Plugin),
		},
		inChan: make(chan *PipelinePack, chanSize),
	}
}

func (dr *dRunner) Decoder() Decoder {
	return dr.plugin.(Decoder)
}

func (dr *dRunner) Start(h PluginHelper, wg *sync.WaitGroup) {
	dr.h = h
	dr.router = h.PipelineConfig().router
	go dr.start(h, wg)
}

func (dr *dRunner) start(h PluginHelper, wg *sync.WaitGroup) {
	var (
		pack  *PipelinePack
		packs []*PipelinePack
		err   error
	)
	if wanter, ok := dr.Decoder().(WantsDecoderRunner); ok {
		wanter.SetDecoderRunner(dr)
	}
	for pack = range dr.inChan {
		if packs, err = dr.Decoder().Decode(pack); packs != nil {
			for _, p := range packs {
				dr.router.InChan() <- p
			}
		} else {
			if err != nil {
				dr.LogError(err)
				if dr.sendFailure {
					if err = AddDecodeFailureFields(pack.Message, err.Error()); err != nil {
						dr.LogError(err)
					}
					dr.router.InChan() <- pack
					continue
				}
			}
			pack.Recycle()
			continue
		}
	}
	if wanter, ok := dr.Decoder().(WantsDecoderRunnerShutdown); ok {
		wanter.Shutdown()
	}
	dr.LogMessage("stopped")
	wg.Done()
}

func (dr *dRunner) InChan() chan *PipelinePack {
	return dr.inChan
}

func (dr *dRunner) Router() MessageRouter {
	return dr.router
}

func (dr *dRunner) NewPack() *PipelinePack {
	return <-dr.h.PipelineConfig().inputRecycleChan
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
	// Associated Filter plugin object.
	Filter() Filter
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
}

// Heka PluginRunner for Output plugins.
type OutputRunner interface {
	PluginRunner
	// Input channel where Output should be listening for incoming messages.
	InChan() chan *PipelinePack
	// Associated Output plugin instance.
	Output() Output
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
	pRunnerBase
	pluginType string
	config     CommonFOConfig
	matcher    *MatchRunner
	ticker     <-chan time.Time
	inChan     chan *PipelinePack
	h          PluginHelper
	retainPack *PipelinePack
	leakCount  int
	encoder    Encoder // output only
	useFraming bool    // output only
	canExit    bool
	kind       foRunnerKind
	pConfig    *PipelineConfig
	lastErr    error
}

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
	runner.inChan = make(chan *PipelinePack, chanSize)

	if config.Matcher == "" {
		return nil, fmt.Errorf("'%s' missing message matcher", name)
	}

	matcher, err := NewMatchRunner(config.Matcher, config.Signer, runner, chanSize)
	if err != nil {
		return nil, fmt.Errorf("Can't create message matcher for '%s': %s", name, err)
	}
	runner.matcher = matcher

	if config.UseFraming != nil && *config.UseFraming {
		runner.useFraming = true
	}

	if _, ok := plugin.(Filter); ok {
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

func (foRunner *foRunner) Start(h PluginHelper, wg *sync.WaitGroup) (err error) {
	foRunner.h = h
	foRunner.pConfig = h.PipelineConfig()

	if foRunner.pluginType == "SandboxFilter" {
		// No maker means we're a dynamic filter and we can exit.
		maker := foRunner.pConfig.makers["Filter"][foRunner.name]
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

	if foRunner.matcher != nil {
		switch foRunner.kind {
		case foFilter:
			foRunner.pConfig.router.fMatcherMap[foRunner.name] = foRunner.matcher
		case foOutput:
			foRunner.pConfig.router.oMatcherMap[foRunner.name] = foRunner.matcher
		}
	}

	go foRunner.Starter(h, wg)
	return
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
	for pack := range foRunner.inChan {
		pack.Recycle()
	}
	return nil
}

func (foRunner *foRunner) exit() {
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

	pack := foRunner.pConfig.PipelinePack(0)
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
	foRunner.pConfig.router.inChan <- pack
}

func (foRunner *foRunner) Starter(helper PluginHelper, wg *sync.WaitGroup) {
	defer wg.Done()

	var err error
	globals := foRunner.pConfig.Globals

	rh, err := NewRetryHelper(foRunner.config.Retries)
	if err != nil {
		foRunner.LogError(err)
		globals.ShutDown()
		return
	}

	if foRunner.matcher != nil {
		sampleDenom := globals.SampleDenominator
		foRunner.matcher.Start(foRunner.inChan, sampleDenom)
	}

	// Handle the cleanup
	defer foRunner.exit()

	for !globals.IsShuttingDown() {
		// `Run` method only returns if there's an error or we're shutting
		// down.
		switch foRunner.kind {
		case foFilter:
			filter := foRunner.Filter()
			err = filter.Run(foRunner, helper)
		case foOutput:
			output := foRunner.Output()
			err = output.Run(foRunner, helper)
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
			switch foRunner.kind {
			case foFilter:
				makers = foRunner.pConfig.makers["Filter"]
			case foOutput:
				makers = foRunner.pConfig.makers["Output"]
			}
			foRunner.maker = makers[foRunner.name]
		}
	initLoop:
		if err = rh.Wait(); err != nil {
			// An error means we've used up our retry attempts, so we
			// exit.
			foRunner.lastErr = err
			foRunner.LogError(err)
			break
		}
		foRunner.LogMessage("now restarting")
		if err = foRunner.plugin.Init(foRunner.maker.Config()); err != nil {
			foRunner.LogError(err)
			goto initLoop
		}
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
	LogError.Printf("Plugin '%s' error: %s", foRunner.name, err)
}

func (foRunner *foRunner) LogMessage(msg string) {
	LogInfo.Printf("Plugin '%s': %s", foRunner.name, msg)
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

func (foRunner *foRunner) Output() Output {
	return foRunner.plugin.(Output)
}

func (foRunner *foRunner) Filter() Filter {
	return foRunner.plugin.(Filter)
}

func (foRunner *foRunner) Encoder() Encoder {
	return foRunner.encoder
}

func (foRunner *foRunner) Encode(pack *PipelinePack) (output []byte, err error) {
	var encoded []byte
	if encoded, err = foRunner.encoder.Encode(pack); err != nil {
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
