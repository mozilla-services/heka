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
	"strconv"
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
}

type LoglineDecoder struct {
	Matcher         *MatcherSpecification
	SeverityMap     map[string]int32
	MessageFields   MessageTemplate
	TimestampLayout string
	dRunner         DecoderRunner
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

// Heka will call this to give us access to the runner.
func (ld *LoglineDecoder) SetDecoderRunner(dr DecoderRunner) {
	ld.dRunner = dr
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
	if sevStr, ok := captures["Severity"]; ok {
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
	return ld.MessageFields.PopulateMessage(pack.Message, captures, ld.TimestampLayout)
}
