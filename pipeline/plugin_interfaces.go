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

// Interface for Heka plugins that can be wired up to the config system.
type Plugin interface {
	// Receives either PluginConfig or custom config struct, populated from
	// the TOML config, and uses that data to initialize the plugin.
	Init(config interface{}) error
}

// Input plugin interface type.
type Input interface {
	// Start listening for / gathering incoming data, populating
	// PipelinePacks, and passing them along to a Decoder or the Router as
	// appropriate.
	Run(ir InputRunner, h PluginHelper) (err error)
	// Called as a signal to the Input to stop listening for / gathering
	// incoming data and to perform any necessary clean-up.
	Stop()
}

// Splitter plugin interface type.
type Splitter interface {
	FindRecord(buf []byte) (bytesRead int, record []byte)
}

// UnframingSplitter is an interface optionally implemented by splitter
// plugins to remove and process any record framing that may have been used by
// the splitter.
type UnframingSplitter interface {
	UnframeRecord(framed []byte, pack *PipelinePack) []byte
}

// Heka Decoder plugin interface.
type Decoder interface {
	// Extract data loaded into the PipelinePack (usually in pack.MsgBytes)
	// and use it to populate pack.Message message objects. If decoding
	// succeeds (i.e. `err` is nil), the original pack will be mutated and
	// returned as the first item in the `packs` return slice. If there is an
	// error, `packs` should be returned as nil.
	// Returning (nil, nil) is valid in cases where the decoding failed but
	// the error should not be logged.
	Decode(pack *PipelinePack) (packs []*PipelinePack, err error)
}

// Heka Filter plugin type.
type OldFilter interface {
	// Starts the filter listening on the FilterRunner's provided input
	// channel. Should not return until shutdown, signaled to the Filter by
	// the closure of the input channel. Should return a non-nil error value
	// only if errors happen during start-up or if there is an unclean
	// shutdown (i.e. not due to an error processing an isolated message, in
	// that case use FilterRunner.LogError).
	Run(r FilterRunner, h PluginHelper) (err error)
}

type MessageProcessor interface {
	ProcessMessage(pack *PipelinePack) (err error)
}

type Filter interface {
	Prepare(r FilterRunner, h PluginHelper) (err error)
	CleanUp()
}

// Heka Encoder plugin interface.
type Encoder interface {
	// Extract data from the provided pack / message and use it to generate a
	// serialized byte stream suitable for writing to disk or over a network.
	Encode(pack *PipelinePack) (output []byte, err error)
}

// Can be implemented by Encoders to tell Heka that the Encoder needs to
// perform some clean-up at shutdown time.
type NeedsStopping interface {
	Stop()
}

// Heka Output plugin type.
type OldOutput interface {
	Run(or OutputRunner, h PluginHelper) (err error)
}

type Output interface {
	Prepare(r OutputRunner, h PluginHelper) (err error)
	CleanUp()
}

type TickerPlugin interface {
	TimerEvent() (err error)
}

// Implemented by the sandbox plugins to allow out-of-band sandbox teardown.
type Destroyable interface {
	StopSB()
	Destroy() error
}
