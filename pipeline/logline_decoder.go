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
func (mt MessageTemplate) PopulateMessage(msg *Message, subs map[string]string,
	timeFormat string) error {

	var val string
	for field, rawVal := range mt {
		if field == "Timestamp" {
			val, err := ForgivingTimeParse(timeFormat, rawVal)
			if err != nil {
				return err
			}
			// Did we get a year?
			if val.Year() == 0 {
				val = val.AddDate(time.Now().Year(), 0, 0)
			}
			msg.SetTimestamp(val.UnixNano())
			continue
		}

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
			pid, err := strconv.ParseInt(val, 10, 32)
			if err != nil {
				return err
			}
			msg.SetPid(int32(pid))
		case "Uuid":
			msg.SetUuid([]byte(val))
		default:
			field, err := NewField(field, val, Field_RAW)
			msg.AddField(field)
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
}

type LoglineDecoder struct {
	Matcher         *MatcherSpecification
	SeverityMap     map[string]int32
	MessageFields   MessageTemplate
	TimestampLayout string
}

func (ld *LoglineDecoder) ConfigStruct() interface{} {
	return new(LoglineDecoderConfig)
}

func (ld *LoglineDecoder) Init(config interface{}) (err error) {
	conf := config.(*LoglineDecoderConfig)
	spec := fmt.Sprintf("Payload =~ %s", conf.MatchRegex)
	if ld.Matcher, err = CreateMatcherSpecification(spec); err != nil {
		err = fmt.Errorf("LoglineDecoder regex error: %s", err)
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
	return
}

// Runs the message payload against decoder's regex. If there's a match, the
// message will be populated based on the decoder's message template, with
// capture values interpolated into the message template values.
func (ld *LoglineDecoder) Decode(pack *PipelinePack) (err error) {
	// First try to match the regex.
	match, captures := ld.Matcher.Match(pack.Message)
	if !match {
		return fmt.Errorf("No match")
	}

	// Was a severity string captured?
	if severityString, ok := captures["Severity"]; ok {
		// If so, see if we have a mapping for this severity.
		if sevInt, ok := ld.SeverityMap[severityString]; ok {
			pack.Message.SetSeverity(sevInt)
		} else {
			// No mapping => severity value should be an int.
			sevInt, err := strconv.ParseInt(severityString, 10, 32)
			if err != nil {
				return fmt.Errorf("Can't parse `Severity`")
			}
			pack.Message.SetSeverity(int32(sevInt))
		}
	}

	// Update the new message fields based on the fields we should
	// change and the capture parts
	return ld.MessageFields.PopulateMessage(pack.Message, captures, ld.TimestampLayout)
}

// Initialize the varMatcher for use in InterpolateString
func init() {
	varMatcher, _ = regexp.Compile("%[A-Za-z]+%")
}
