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
#
# ***** END LICENSE BLOCK *****/
package pipeline

import (
	"fmt"
	. "github.com/mozilla-services/heka/message"
	"log"
	"regexp"
	"strconv"
	"time"
)

var varMatcher *regexp.Regexp

type mapOfStrings map[string]string

type TextParserDecoderConfig struct {
	SeverityMap     map[string]int32
	MessageFields   mapOfStrings
	TimestampLayout string
	PayloadMatch    string
}

type TextParserDecoder struct {
	SeverityMap     map[string]int32
	MessageFields   mapOfStrings
	TimestampLayout string
	PayloadMatch    *regexp.Regexp
}

func (t *TextParserDecoder) ConfigStruct() interface{} {
	return new(TextParserDecoderConfig)
}

func (t *TextParserDecoder) Init(config interface{}) (err error) {
	conf := config.(*TextParserDecoderConfig)
	t.SeverityMap = make(map[string]int32)
	t.MessageFields = make(mapOfStrings)
	if conf.SeverityMap != nil {
		for codeString, codeInt := range conf.SeverityMap {
			t.SeverityMap[codeString] = codeInt
		}
	}
	if conf.MessageFields != nil {
		for field, action := range conf.MessageFields {
			t.MessageFields[field] = action
		}
	}

	// Replace helper words with complex regex
	wordMatcher, _ := regexp.Compile("TIMESTAMP")
	newPayload := wordMatcher.ReplaceAllStringFunc(conf.PayloadMatch,
		func(match string) string {
			if match == "TIMESTAMP" {
				return HelperRegexSubs["TIMESTAMP"]
			}
			return match
		})
	t.PayloadMatch, err = regexp.Compile(newPayload)
	if err != nil {
		return
	}
	t.TimestampLayout = conf.TimestampLayout
	varMatcher, _ = regexp.Compile("@[A-Za-z]+")
	return
}

func (t *TextParserDecoder) Decode(pipelinePack *PipelinePack) error {
	matchParts := make(mapOfStrings)
	changeFields := make(mapOfStrings)

	// Copy our message fields to change
	for field, val := range t.MessageFields {
		changeFields[field] = val
	}

	if t.PayloadMatch.String() != "" {
		err := t.ParsePayload(pipelinePack.Message.Payload, matchParts,
			pipelinePack)
		if err != nil {
			return logError(err, pipelinePack)
		}
	}

	if severityString, found := matchParts["Severity"]; found {
		// First see if we have a mapping for this severity
		if sevInt, found := t.SeverityMap[severityString]; found {
			pipelinePack.Message.Severity = &sevInt
		} else {
			sevInt, err := strconv.ParseInt(severityString, 10, 32)
			if err != nil {
				return logError(err, pipelinePack)
			}
			sevInt32 := int32(sevInt)
			pipelinePack.Message.Severity = &sevInt32
		}
	}

	// Copy fields that don't exist in changeFields from our matchParts
	// This allows us to directly set a Timestamp for example, if one was
	// matched, without having to say it again in the config
	for matchField, value := range matchParts {
		if _, present := t.MessageFields[matchField]; !present {
			changeFields[matchField] = value
		}
	}

	err := t.updateMessage(pipelinePack.Message, changeFields, matchParts)
	if err != nil {
		return logError(err, pipelinePack)
	}
	return nil
}

// Update a message based on the populated fields to use for altering it
func (t *TextParserDecoder) updateMessage(message *Message, changeFields,
	matchParts mapOfStrings) error {
	for field, formatRegexp := range changeFields {
		if field == "Timestamp" {
			val, err := ForgivingTimeParse(t.TimestampLayout, formatRegexp)
			if err != nil {
				return err
			}
			// Did we get a year?
			if val.Year() == 0 {
				val = val.AddDate(time.Now().Year(), 0, 0)
			}
			message.SetTimestamp(val.UnixNano())
			continue
		}
		newString := interpolateString(formatRegexp, matchParts) // prob here too
		if field == "Logger" {
			message.SetLogger(newString)
		} else if field == "Type" {
			message.SetType(newString)
		} else if field == "Payload" {
			message.SetPayload(newString)
		} else if field == "Hostname" {
			message.SetHostname(newString)
		} else if field == "Pid" {
			pid, err := strconv.ParseInt(newString, 10, 32)
			if err != nil {
				return err
			}
			message.SetPid(int32(pid))
		} else if field == "Uuid" {
			message.SetUuid([]byte(newString))
		} else {
			field, err := NewField(field, newString, Field_RAW)
			message.AddField(field)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

// Parse a payload and put the matched regular subexpressions into matchParts
//
// This will return an error if there are no subexpressions present
func (t *TextParserDecoder) ParsePayload(payload *string, matchParts mapOfStrings,
	pipelinePack *PipelinePack) error {
	findResults := t.PayloadMatch.FindStringSubmatch(*payload)
	if findResults == nil || len(findResults) < 2 {
		return logError(fmt.Errorf("No regexp matches found in %s", *payload),
			pipelinePack)
	}
	resultLength := len(findResults)
	for index, name := range t.PayloadMatch.SubexpNames() {
		if name == "" {
			continue
		}
		if index > resultLength-1 {
			matchParts[name] = ""
		} else {
			matchParts[name] = findResults[index]
		}
	}
	return nil
}

// Given a regular expression, return the string resulting from interpolating
// variables that exist in matchParts
//
// Example input to a formatRegexp: Reported at @Hostname by @Reporter
// Assuming there are entires in matchParts for 'Hostname' and 'Reporter', the
// returned string will then be: Reported at Somehost by Jonathon
func interpolateString(formatRegexp string, matchParts mapOfStrings) (newString string) {
	return varMatcher.ReplaceAllStringFunc(formatRegexp,
		func(matchWord string) string {
			// Remove the preceeding @
			m := matchWord[1:]
			if repl, found := matchParts[m]; found {
				return repl
			}
			return ""
		})
}

// Log an error in the pipeline pack during decoder processing
func logError(err error, pipelinePack *PipelinePack) error {
	log.Printf("Unable to properly parse message UUID: %s ERROR: %s",
		pipelinePack.Message.GetUuid(), err)
	return err
}
