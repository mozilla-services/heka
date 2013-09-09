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
	byEncoding  []DecoderRunner
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
}

type dRunner struct {
	pRunnerBase
	inChan chan *PipelinePack
	uuid   string
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

func (dr *dRunner) LogError(err error) {
	log.Printf("Decoder '%s' error: %s", dr.name, err)
}

func (dr *dRunner) LogMessage(msg string) {
	log.Printf("Decoder '%s': %s", dr.name, msg)
}

// Any decoder that needs access to its DecoderRunner  can implement this
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
