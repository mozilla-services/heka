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
#   Ben Bangert (bbangert@mozilla.com)
#   Rob Miller (rmiller@mozilla.com)
#
# ***** END LICENSE BLOCK *****/

package payload

import (
	"fmt"
	. "github.com/mozilla-services/heka/pipeline"
	"regexp"
	"time"
)

type PayloadRegexDecoderConfig struct {
	// Regular expression that describes log line format and capture group
	// values.
	MatchRegex string `toml:"match_regex"`

	// Maps severity strings to their int version
	SeverityMap map[string]int32 `toml:"severity_map"`

	// Keyed to the message field that should be filled in, the value will be
	// interpolated so it can use capture parts from the message match.
	MessageFields MessageTemplate `toml:"message_fields"`

	// User specified timestamp layout string, used for parsing a timestamp
	// string into an actual time object. If not specified or it fails to
	// match, all the default time layouts will be tried (see
	// http://golang.org/pkg/time/#pkg-constants).
	TimestampLayout string `toml:"timestamp_layout"`

	// Time zone in which the timestamps in the text are presumed to be in.
	// Should be a location name corresponding to a file in the IANA Time Zone
	// database (e.g. "America/Los_Angeles"), as parsed by Go's
	// `time.LoadLocation()` function (see
	// http://golang.org/pkg/time/#LoadLocation). Defaults to "UTC". Not
	// required if valid time zone info is embedded in every parsed timestamp,
	// since those can be parsed as specified in the `timestamp_layout`.
	TimestampLocation string `toml:"timestamp_location"`

	// Whether payloads that do not match the regex should be logged.
	LogErrors bool `toml:"log_errors"`
}

type PayloadRegexDecoder struct {
	Match           *regexp.Regexp
	SeverityMap     map[string]int32
	MessageFields   MessageTemplate
	TimestampLayout string
	tzLocation      *time.Location
	dRunner         DecoderRunner
	logErrors       bool
}

func (ld *PayloadRegexDecoder) ConfigStruct() interface{} {
	return &PayloadRegexDecoderConfig{
		LogErrors: true,
	}
}

func (ld *PayloadRegexDecoder) Init(config interface{}) (err error) {
	conf := config.(*PayloadRegexDecoderConfig)
	if ld.Match, err = regexp.Compile(conf.MatchRegex); err != nil {
		err = fmt.Errorf("PayloadRegexDecoder: %s", err)
		return
	}
	if ld.Match.NumSubexp() == 0 {
		err = fmt.Errorf("PayloadRegexDecoder regex must contain capture groups")
		return
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
		err = fmt.Errorf("PayloadRegexDecoder unknown timestamp_location '%s': %s",
			conf.TimestampLocation, err)
	}
	ld.logErrors = conf.LogErrors
	return
}

// Heka will call this to give us access to the runner.
func (ld *PayloadRegexDecoder) SetDecoderRunner(dr DecoderRunner) {
	ld.dRunner = dr
}

// Matches the given string against the regex and returns the match result
// and captures
func tryMatch(re *regexp.Regexp, s string) (match bool, captures map[string]string) {
	findResults := re.FindStringSubmatch(s)
	if findResults == nil {
		return
	}
	match = true
	captures = make(map[string]string)
	for index, name := range re.SubexpNames() {
		if index == 0 {
			continue
		}
		if name == "" {
			name = fmt.Sprintf("%d", index)
		}
		captures[name] = findResults[index]
	}
	return
}

// Runs the message payload against decoder's regex. If there's a match, the
// message will be populated based on the decoder's message template, with
// capture values interpolated into the message template values.
func (ld *PayloadRegexDecoder) Decode(pack *PipelinePack) (packs []*PipelinePack, err error) {
	// First try to match the regex.
	match, captures := tryMatch(ld.Match, pack.Message.GetPayload())
	if !match {
		if ld.logErrors {
			err = fmt.Errorf("No match: %s", pack.Message.GetPayload())
		}
		return
	}

	pdh := &PayloadDecoderHelper{
		Captures:        captures,
		dRunner:         ld.dRunner,
		TimestampLayout: ld.TimestampLayout,
		TzLocation:      ld.tzLocation,
		SeverityMap:     ld.SeverityMap,
	}

	pdh.DecodeTimestamp(pack)
	pdh.DecodeSeverity(pack)

	// Update the new message fields based on the fields we should
	// change and the capture parts
	if err = ld.MessageFields.PopulateMessage(pack.Message, captures); err == nil {
		packs = []*PipelinePack{pack}
	}
	return
}

func init() {
	RegisterPlugin("PayloadRegexDecoder", func() interface{} {
		return new(PayloadRegexDecoder)
	})
}
