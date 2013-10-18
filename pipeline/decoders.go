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
	"code.google.com/p/go-uuid/uuid"
	"code.google.com/p/goprotobuf/proto"
	"fmt"
	"github.com/mozilla-services/heka/message"
	"log"
	"regexp"
	"strconv"
	"strings"
	"sync"
)

// Encapsulates access to a set of DecoderRunners.
type DecoderSet interface {
	// Returns running DecoderRunner registered under the specified name, or
	// nil and ok == false if no such name is registered.
	ByName(name string) (decoder DecoderRunner, ok bool)
}

type decoderSet struct {
	chansByName map[string]chan DecoderRunner
	byName      map[string]DecoderRunner
}

// Creates and returns a decoderSet that exposes an API to access the
// DecoderRunners in the provided channels. Expects that the channels are
// fully populated with all available DecoderRunners before being passed to
// this function.
func newDecoderSet(decoderChans map[string]chan DecoderRunner) (ds *decoderSet, err error) {
	ds = &decoderSet{
		chansByName: decoderChans,
		byName:      make(map[string]DecoderRunner),
	}
	return
}

func (ds *decoderSet) ByName(name string) (decoder DecoderRunner, ok bool) {
	if decoder, ok = ds.byName[name]; ok {
		// We've already got it, return it.
		return
	}
	var dChan chan DecoderRunner
	if dChan, ok = ds.chansByName[name]; ok {
		decoder = <-dChan
		dChan <- decoder
		ds.byName[name] = decoder
	}
	return
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
	// UUID to distinguish the duplicate instances of the same registered
	// Decoder plugin type from each other.
	UUID() string
	// Returns the running Heka router for direct use by decoder plugins.
	Router() MessageRouter
	// Fetches a new pack from the input supply and returns it to the caller,
	// for decoders that generate multiple messages from a single input
	// message.
	NewPack() *PipelinePack
}

type dRunner struct {
	pRunnerBase
	inChan chan *PipelinePack
	uuid   string
	router *messageRouter
	h      PluginHelper
}

// Creates and returns a new (but not yet started) DecoderRunner for the
// provided Decoder plugin.
func NewDecoderRunner(name string, decoder Decoder,
	pluginGlobals *PluginGlobals) DecoderRunner {
	return &dRunner{
		pRunnerBase: pRunnerBase{
			name:          name,
			plugin:        decoder.(Plugin),
			pluginGlobals: pluginGlobals,
		},
		uuid:   uuid.NewRandom().String(),
		inChan: make(chan *PipelinePack, Globals().PluginChanSize),
	}
}

func (dr *dRunner) Decoder() Decoder {
	return dr.plugin.(Decoder)
}

func (dr *dRunner) Start(h PluginHelper, wg *sync.WaitGroup) {
	dr.h = h
	dr.router = h.PipelineConfig().router
	go func() {
		var pack *PipelinePack

		if wanter, ok := dr.Decoder().(WantsDecoderRunner); ok {
			wanter.SetDecoderRunner(dr)
		}
		var err error
		for pack = range dr.inChan {
			if err = dr.Decoder().Decode(pack); err != nil {
				dr.LogError(err)
				pack.Recycle()
				continue
			}
			pack.Decoded = true
			h.PipelineConfig().router.InChan() <- pack
		}
		if wanter, ok := dr.Decoder().(WantsDecoderRunnerShutdown); ok {
			wanter.Shutdown()
		}
		dr.LogMessage("stopped")
		wg.Done()
	}()
}

func (dr *dRunner) InChan() chan *PipelinePack {
	return dr.inChan
}

func (dr *dRunner) UUID() string {
	return dr.uuid
}

func (dr *dRunner) Router() MessageRouter {
	return dr.router
}

func (dr *dRunner) NewPack() *PipelinePack {
	return <-dr.h.PipelineConfig().inputRecycleChan
}

func (dr *dRunner) LogError(err error) {
	log.Printf("Decoder '%s' error: %s", dr.name, err)
}

func (dr *dRunner) LogMessage(msg string) {
	log.Printf("Decoder '%s': %s", dr.name, msg)
}

// Any decoder that needs access to its DecoderRunner can implement this
// interface and it will be provided at DecoderRunner start time.
type WantsDecoderRunner interface {
	SetDecoderRunner(dr DecoderRunner)
}

// Any decoder that needs to know when the DecoderRunner is exiting can
// implement this interface and it will called on DecoderRunner exit.
type WantsDecoderRunnerShutdown interface {
	Shutdown()
}

// Heka Decoder plugin interface.
type Decoder interface {
	// Extract data loaded into the PipelinePack (usually in pack.MsgBytes)
	// and use it to populated pack.Message message object.
	Decode(pack *PipelinePack) error
}

// Decoder for converting ProtocolBuffer data into Message objects.
type ProtobufDecoder struct{}

func (self *ProtobufDecoder) Init(config interface{}) error {
	return nil
}

func (self *ProtobufDecoder) Decode(pack *PipelinePack) error {
	return proto.Unmarshal(pack.MsgBytes, pack.Message)
}

type extraStatMessage struct {
	timestamp uint64
	pack      *PipelinePack
}

// Decoder that expects statsd string format data in the message payload,
// converts that to identical statsd format data in the message fields, in the
// same format that a StatAccumInput w/ `emit_in_fields` set to true would
// use.
type StatsToFieldsDecoder struct {
	runner DecoderRunner
	helper PluginHelper
}

func (d *StatsToFieldsDecoder) Init(config interface{}) error {
	return nil
}

// Implement `WantsDecoderRunner`
func (d *StatsToFieldsDecoder) SetDecoderRunner(dr DecoderRunner) {
	d.runner = dr
}

func (d *StatsToFieldsDecoder) Decode(pack *PipelinePack) (err error) {
	var (
		timestamp uint64
		extras    []*extraStatMessage
	)

	lines := strings.Split(strings.Trim(pack.Message.GetPayload(), "\n"), "\n")
	routerChan := d.runner.Router().InChan()
	for _, line := range lines {
		fields := strings.Fields(line)
		// Sanity check.
		if len(fields) != 3 {
			return fmt.Errorf("malformed statmetric line: '%s'", line)
		}
		// Check timestamp validity.
		var unixTime uint64
		unixTime, err = strconv.ParseUint(fields[2], 0, 32)
		if err != nil {
			return fmt.Errorf("invalid timestamp: '%s'", line)
		}
		// Check value validity.
		var value float64
		if value, err = strconv.ParseFloat(fields[1], 64); err != nil {
			return fmt.Errorf("invalid value: '%s'", line)
		}
		if timestamp != unixTime {
			if timestamp == uint64(0) {
				timestamp = unixTime
			} else {
				// Multiple timestamps in one stat set. Generate a separate
				// message for each one.
				var sMsg *extraStatMessage
				// Check if we've seen this one already.
				found := false
				for _, sMsg = range extras {
					if unixTime == sMsg.timestamp {
						// We've got it.
						found = true
						break
					}
				}
				if !found {
					// No existing message, create one.
					sMsg = &extraStatMessage{
						timestamp: unixTime,
						pack:      d.runner.NewPack(),
					}
					// Copy the original message, but clear out the payload and the fields.
					pack.Message.Copy(sMsg.pack.Message)
					sMsg.pack.Message.Fields = make([]*message.Field, 0, 2)
					sMsg.pack.Message.SetPayload("")
					if err = d.addStatField(sMsg.pack, "timestamp", int64(unixTime)); err != nil {
						return
					}
					extras = append(extras, sMsg)
				}
				// Add field to the extra message.
				if err = d.addStatField(sMsg.pack, fields[0], value); err != nil {
					return
				}
			}
		}
		// Add field to the main message.
		if err = d.addStatField(pack, fields[0], value); err != nil {
			return
		}
	}
	// Add timestamp field to the main message.
	if err = d.addStatField(pack, "timestamp", int64(timestamp)); err != nil {
		return
	}
	if extras != nil {
		for _, sMsg := range extras {
			routerChan <- sMsg.pack
		}
	}

	return
}

func (d *StatsToFieldsDecoder) addStatField(pack *PipelinePack, name string,
	value interface{}) error {

	field, err := message.NewField(name, value, "")
	if err != nil {
		return fmt.Errorf("error adding field '%s': %s", name, err)
	}
	pack.Message.AddField(field)
	return nil
}

// Populated by the init function, this regex matches the MessageFields values
// to interpolate variables from capture groups or other parts of the existing
// message.
var varMatcher *regexp.Regexp

// Common type used to specify a set of values with which to populate a
// message object. The keys represent message fields, the values can be
// interpolated w/ capture parts from a message matcher.
type MessageTemplate map[string]string

// Applies this message template's values to the provided message object,
// interpolating the provided substitutions into the values in the process.
func (mt MessageTemplate) PopulateMessage(msg *message.Message, subs map[string]string) error {
	var val string
	for field, rawVal := range mt {
		val = InterpolateString(rawVal, subs)
		switch field {
		case "Logger":
			msg.SetLogger(val)
		case "Type":
			msg.SetType(val)
		case "Payload":
			msg.SetPayload(val)
		case "Hostname":
			msg.SetHostname(val)
		case "Pid":
			int_part := strings.Split(val, ".")[0]
			pid, err := strconv.ParseInt(int_part, 10, 32)
			if err != nil {
				return err
			}
			msg.SetPid(int32(pid))
		case "Uuid":
			msg.SetUuid([]byte(val))
		default:
			fi := strings.SplitN(field, "|", 2)
			if len(fi) < 2 {
				fi = append(fi, "")
			}
			f, err := message.NewField(fi[0], val, fi[1])
			msg.AddField(f)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

// Given a regular expression, return the string resulting from interpolating
// variables that exist in matchParts
//
// Example input to a formatRegexp: Reported at %Hostname% by %Reporter%
// Assuming there are entries in matchParts for 'Hostname' and 'Reporter', the
// returned string will then be: Reported at Somehost by Jonathon
func InterpolateString(formatRegexp string, subs map[string]string) (newString string) {
	return varMatcher.ReplaceAllStringFunc(formatRegexp,
		func(matchWord string) string {
			// Remove the preceding and trailing %
			m := matchWord[1 : len(matchWord)-1]
			if repl, ok := subs[m]; ok {
				return repl
			}
			return fmt.Sprintf("<%s>", m)
		})
}

// Initialize the varMatcher for use in InterpolateString
func init() {
	varMatcher, _ = regexp.Compile("%\\w+%")
}
