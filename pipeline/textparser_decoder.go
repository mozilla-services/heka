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

type mapOfStrings map[string]string

type TextParserDecoderConfig struct {
	SeverityMap     map[string]int32
	MessageFields   map[string]string
	TimestampLayout string
	PayloadMatch    string
}

type TextParserDecoder struct {
	SeverityMap     map[string]int32
	MessageFields   map[string]string
	TimestampLayout string
	PayloadMatch    *regexp.Regexp
}

func (t *TextParserDecoder) ConfigStruct() interface{} {
	return new(TextParserDecoderConfig)
}

var varMatcher *regexp.Regexp

func (t *TextParserDecoder) Init(config interface{}) (err error) {
	conf := config.(TextParserDecoderConfig)
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
	t.PayloadMatch, err = regexp.Compile(conf.PayloadMatch)
	varMatcher, _ = regexp.Compile("@[A-Za-z]+")
	return
}

func (t *TextParserDecoder) Decode(pipelinePack *PipelinePack) error {
	matchParts := make(mapOfStrings)
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
	for field, formatRegexp := range t.MessageFields {
		if field == "Timestamp" {
			val, err := time.Parse(t.TimestampLayout, formatRegexp)
			if err != nil {
				return logError(err, pipelinePack)
			}
			pipelinePack.Message.SetTimestamp(val.UnixNano())
			continue
		}
		newString := interpolateString(formatRegexp, matchParts)
		if field == "Logger" {
			pipelinePack.Message.SetLogger(newString)
		} else if field == "Type" {
			pipelinePack.Message.SetType(newString)
		} else if field == "Payload" {
			pipelinePack.Message.SetPayload(newString)
		} else if field == "Hostname" {
			pipelinePack.Message.SetHostname(newString)
		} else if field == "Pid" {
			pid, err := strconv.ParseInt(newString, 10, 32)
			if err != nil {
				return logError(err, pipelinePack)
			}
			pipelinePack.Message.SetPid(int32(pid))
		} else if field == "Uuid" {
			pipelinePack.Message.SetUuid([]byte(newString))
		} else {
			field, err := NewField(field, newString, Field_RAW)
			if err != nil {
				return logError(err, pipelinePack)
			}
			pipelinePack.Message.AddField(field)
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
	log.Printf("Unable to properly parse message UUID: %s. Error: %s",
		pipelinePack.Message.GetUuid(), err)
	return err
}
