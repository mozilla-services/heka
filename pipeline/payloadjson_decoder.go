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
#   Victor Ng (vng@mozilla.com)
#
# ***** END LICENSE BLOCK *****/

package pipeline

import (
	"fmt"
	. "github.com/mozilla-services/heka/message"
	"strconv"
	"time"
)

type PayloadJsonDecoderConfig struct {
	// Regular expression that describes log line format and capture group
	// values.
	JsonMap map[string]string `toml:"json_map"`

	// Maps severity strings to their int version
	SeverityMap map[string]int32 `toml:"severity_map"`

	// Keyed to the message field that should be filled in, the value will be
	// interpolated so it can use capture parts from the message match.
	MessageFields MessageTemplate `toml:"message_fields"`

	// User specified timestamp layout string, used for parsing a timestamp
	// string into an actual time object. If not specified or it fails to
	// match, all the default time layout's will be tried.
	TimestampLayout string `toml:"timestamp_layout"`

	// Time zone in which the timestamps in the text are presumed to be in.
	// Should be a location name corresponding to a file in the IANA Time Zone
	// database (e.g. "America/Los_Angeles"), as parsed by Go's
	// `time.LoadLocation()` function (see
	// http://golang.org/pkg/time/#LoadLocation). Defaults to "UTC". Not
	// required if valid time zone info is embedded in every parsed timestamp,
	// since those can be parsed as specified in the `timestamp_layout`.
	TimestampLocation string `toml:"timestamp_location"`
}

type PayloadJsonDecoder struct {
	JsonMap         map[string]string
	SeverityMap     map[string]int32
	MessageFields   MessageTemplate
	TimestampLayout string
	tzLocation      *time.Location
	dRunner         DecoderRunner
}

func (ld *PayloadJsonDecoder) ConfigStruct() interface{} {
	return &PayloadJsonDecoderConfig{
		TimestampLayout: "2012-04-23T18:25:43.511Z",
	}
}

func (ld *PayloadJsonDecoder) Init(config interface{}) (err error) {
	conf := config.(*PayloadJsonDecoderConfig)

	ld.JsonMap = make(map[string]string)
	for capture_name, jp := range conf.JsonMap {
		ld.JsonMap[capture_name] = jp
	}

	ld.SeverityMap = make(map[string]int32)
	ld.MessageFields = make(MessageTemplate)
	if conf.SeverityMap != nil {
		for codeString, codeInt := range conf.SeverityMap {
			ld.SeverityMap[codeString] = codeInt
		}
	}
	if conf.MessageFields != nil {
		for field, action := range conf.MessageFields {
			ld.MessageFields[field] = action
		}
	}
	ld.TimestampLayout = conf.TimestampLayout
	if ld.tzLocation, err = time.LoadLocation(conf.TimestampLocation); err != nil {
		err = fmt.Errorf("PayloadJsonDecoder unknown timestamp_location '%s': %s",
			conf.TimestampLocation, err)
	}
	return
}

// Heka will call this to give us access to the runner.
func (ld *PayloadJsonDecoder) SetDecoderRunner(dr DecoderRunner) {
	ld.dRunner = dr
}

// Matches the given string against the JSONPath and returns the match result
// and captures
func (ld *PayloadJsonDecoder) match(s string) (captures map[string]string) {
	captures = make(map[string]string)

	jp := new(JsonPath)
	jp.SetJsonText(s)

	for capture_group, jpath := range ld.JsonMap {
		node_val, err := jp.Find(jpath)
		if err != nil {
			continue
		}
		captures[capture_group] = node_val
	}
	return
}

// Runs the message payload against decoder's map of JSONPaths. If
// there's a match, the message will be populated based on the
// decoder's message template, with capture values interpolated into
// the message template values.
func (ld *PayloadJsonDecoder) Decode(pack *PipelinePack) (err error) {
	captures := ld.match(pack.Message.GetPayload())

	if timeStamp, ok := captures["Timestamp"]; ok {
		val, err := ForgivingTimeParse(ld.TimestampLayout, timeStamp, ld.tzLocation)
		if err != nil {
			ld.dRunner.LogError(fmt.Errorf("Don't recognize Timestamp: '%s'", timeStamp))
		}
		// If we only get a timestamp, use the current date
		if val.Year() == 0 && val.Month() == 1 && val.Day() == 1 {
			now := time.Now()
			val = val.AddDate(now.Year(), int(now.Month()-1), now.Day()-1)
		} else if val.Year() == 0 {
			// If there's no year, use current year
			val = val.AddDate(time.Now().Year(), 0, 0)
		}
		pack.Message.SetTimestamp(val.UnixNano())
	} else if sevStr, ok := captures["Severity"]; ok {
		// If so, see if we have a mapping for this severity.
		if sevInt, ok := ld.SeverityMap[sevStr]; ok {
			pack.Message.SetSeverity(sevInt)
		} else {
			// No mapping => severity value should be an int.
			sevInt, err := strconv.ParseInt(sevStr, 10, 32)
			if err != nil {
				ld.dRunner.LogError(fmt.Errorf("Don't recognize severity: '%s'", sevStr))
			} else {
				pack.Message.SetSeverity(int32(sevInt))
			}
		}
	}
	// Update the new message fields based on the fields we should
	// change and the capture parts
	return ld.MessageFields.PopulateMessage(pack.Message, captures)
}
