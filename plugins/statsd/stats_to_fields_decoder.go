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

package statsd

import (
	"fmt"
	"github.com/mozilla-services/heka/message"
	. "github.com/mozilla-services/heka/pipeline"
	"strconv"
	"strings"
)

type extraStatMessage struct {
	timestamp uint64
	pack      *PipelinePack
}

// Decoder that expects graphite string format data in the message payload,
// converts that to identical stats data in the message fields, in the same
// format that a StatAccumInput w/ `emit_in_fields` set to true would use.
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

func (d *StatsToFieldsDecoder) Decode(pack *PipelinePack) (packs []*PipelinePack,
	err error) {

	var (
		timestamp uint64
		extras    []*extraStatMessage
	)

	lines := strings.Split(strings.Trim(pack.Message.GetPayload(), "\n"), "\n")
	for _, line := range lines {
		fields := strings.Fields(line)
		// Sanity check.
		if len(fields) != 3 {
			err = fmt.Errorf("malformed statmetric line: '%s'", line)
			return
		}
		// Check timestamp validity.
		var unixTime uint64
		unixTime, err = strconv.ParseUint(fields[2], 0, 32)
		if err != nil {
			err = fmt.Errorf("invalid timestamp: '%s'", line)
			return
		}
		// Check value validity.
		var value float64
		if value, err = strconv.ParseFloat(fields[1], 64); err != nil {
			err = fmt.Errorf("invalid value: '%s'", line)
			return
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

	if extras == nil {
		packs = []*PipelinePack{pack}
	} else {
		packs = make([]*PipelinePack, len(extras)+1)
		packs[0] = pack
		for i, e := range extras {
			packs[i+1] = e.pack
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

func init() {
	RegisterPlugin("StatsToFieldsDecoder", func() interface{} {
		return new(StatsToFieldsDecoder)
	})
}
