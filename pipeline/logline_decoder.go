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

package pipeline

import (
	"fmt"
	. "github.com/mozilla-services/heka/message"
	"regexp"
	"strconv"
	"time"
)

type LoglineDecoderConfig struct {
	// Regular expression that describes log line format and capture group
	// values.
	MatchRegex string

	// Maps severity strings to their int version
	SeverityMap map[string]int32

	// Keyed to the message field that should be filled in, the value will be
	// interpolated so it can use capture parts from the message match.
	MessageFields MessageTemplate

	// User specified timestamp layout string, used for parsing a timestamp
	// string into an actual time object. If not specified or it fails to
	// match, all the default time layout's will be tried.
	TimestampLayout string

	// Timezone used for the timestamps in the source text. This option should
	// only be used in cases where the source text timestamps are not in UTC
	// but the timezone used is not specified in the text itself. If the
	// timestamp *is* specified in the source text, you should parse this
	// using the timestamp_layout string. Timezone must be specified as a time
	// delta from UTC that will be understood by Go's duration parsing
	//

	// Duration of time by which each parsed timestamp should be adjusted when
	// decoded. Useful in cases such as when a file's timestamps are not in
	// UTC, but the timezone data is not in the timestamp. (If the timezone
	// data *is* in the timestamp, you can parse this directly w/ the
	// timestamp_layout option.) Must be specified in a way that will be
	// understood by Go's duration parsing (see
	// http://golang.org/pkg/time/#ParseDuration), e.g. "-7h" or "+2h30m".
	TimeAdjustDuration string `toml:"time_adjust_duration"`
}

type LoglineDecoder struct {
	Match           *regexp.Regexp
	SeverityMap     map[string]int32
	MessageFields   MessageTemplate
	TimestampLayout string
	timeDelta       time.Duration
	dRunner         DecoderRunner
}

func (ld *LoglineDecoder) ConfigStruct() interface{} {
	return new(LoglineDecoderConfig)
}

func (ld *LoglineDecoder) Init(config interface{}) (err error) {
	conf := config.(*LoglineDecoderConfig)
	if ld.Match, err = regexp.Compile(conf.MatchRegex); err != nil {
		err = fmt.Errorf("LoglineDecoder: %s", err)
		return
	}
	if ld.Match.NumSubexp() == 0 {
		err = fmt.Errorf("LoglineDecoder regex must contain capture groups")
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
	if conf.TimeAdjustDuration != "" {
		if ld.timeDelta, err = time.ParseDuration(conf.TimeAdjustDuration); err != nil {
			err = fmt.Errorf("LoglineDecoder can't parse timezone: %s", err)
		}
	}
	return
}

// Heka will call this to give us access to the runner.
func (ld *LoglineDecoder) SetDecoderRunner(dr DecoderRunner) {
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
func (ld *LoglineDecoder) Decode(pack *PipelinePack) (err error) {
	// First try to match the regex.
	match, captures := tryMatch(ld.Match, pack.Message.GetPayload())
	if !match {
		return fmt.Errorf("No match")
	}

	if timeStamp, ok := captures["Timestamp"]; ok {
		val, err := ForgivingTimeParse(ld.TimestampLayout, timeStamp)
		if err != nil {
			ld.dRunner.LogError(fmt.Errorf("Don't recognize Timestamp: '%s'", timeStamp))
		}
		// Did we get a year?
		if val.Year() == 0 {
			val = val.AddDate(time.Now().Year(), 0, 0)
		}
		if ld.timeDelta != 0 {
			val = val.Add(ld.timeDelta)
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
