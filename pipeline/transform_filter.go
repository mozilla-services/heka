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
	"regexp"
	"strconv"
	"time"
)

var varMatcher *regexp.Regexp

type MatchSet map[string]string

type TransformFilterConfig struct {
	SeverityMap     map[string]int32
	MessageFields   MatchSet
	TimestampLayout string
}

type TransformFilter struct {
	SeverityMap     map[string]int32
	MessageFields   MatchSet
	TimestampLayout string
	basicFields     []string
}

func (t *TransformFilter) ConfigStruct() interface{} {
	return new(TransformFilterConfig)
}

func (t *TransformFilter) Init(config interface{}) (err error) {
	conf := config.(*TransformFilterConfig)
	t.SeverityMap = make(map[string]int32)
	t.MessageFields = make(MatchSet)
	t.basicFields = []string{"Timestamp", "Logger", "Type", "Hostname",
		"Payload", "Pid", "Uuid"}
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

	t.TimestampLayout = conf.TimestampLayout
	return
}

func (t *TransformFilter) Run(fr FilterRunner, h PluginHelper) (err error) {
	inChan := fr.InChan()

	var (
		pack     *PipelinePack
		newPack  *PipelinePack
		captures map[string]string
	)

	errMsg := "Can't parse message UUID: %s ERROR: %s"

	for plc := range inChan {
		pack = plc.Pack
		captures = plc.Captures
		newPack = h.PipelinePack(plc.Pack.MsgLoopCount)

		changeFields := make(MatchSet)

		// Copy our message fields to change
		for field, val := range t.MessageFields {
			changeFields[field] = val
		}

		if severityString, ok := captures["Severity"]; ok {
			// First see if we have a mapping for this severity
			if sevInt, ok := t.SeverityMap[severityString]; ok {
				newPack.Message.SetSeverity(sevInt)
			} else {
				// Otherwise, assume the severity located will be an int
				sevInt, err := strconv.ParseInt(severityString, 10, 32)
				if err != nil {
					fr.LogError(fmt.Errorf(errMsg, pack.Message.GetUuid(), err))
					pack.Recycle()
					newPack.Recycle()
					continue
				}
				sevInt32 := int32(sevInt)
				newPack.Message.SetSeverity(sevInt32)
			}
		}

		// Only copy basic fields into the changeFields
	basicFieldMatch:
		for _, matchField := range t.basicFields {
			// Does it exist in our captured parts?
			value := captures[matchField]
			if value == "" {
				continue basicFieldMatch
			}
			if _, present := t.MessageFields[matchField]; !present {
				changeFields[matchField] = value
			}
		}

		err := t.updateMessage(newPack.Message, changeFields, captures)
		if err != nil {
			fr.LogError(fmt.Errorf(errMsg, pack.Message.GetUuid(), err))
			pack.Recycle()
			newPack.Recycle()
			continue
		}

		fr.Inject(newPack)
		pack.Recycle()
	}

	return

}

// Update a message based on the populated fields to use for altering it.
func (t *TransformFilter) updateMessage(message *Message, changeFields,
	matchParts MatchSet) error {
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

		newString := InterpolateString(formatRegexp, matchParts)
		switch field {
		case "Logger":
			message.SetLogger(newString)
		case "Type":
			message.SetType(newString)
		case "Payload":
			message.SetPayload(newString)
		case "Hostname":
			message.SetHostname(newString)
		case "Pid":
			pid, err := strconv.ParseInt(newString, 10, 32)
			if err != nil {
				return err
			}
			message.SetPid(int32(pid))
		case "Uuid":
			message.SetUuid([]byte(newString))
		default:
			field, err := NewField(field, newString, Field_RAW)
			message.AddField(field)
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
// Example input to a formatRegexp: Reported at @Hostname by @Reporter
// Assuming there are entries in matchParts for 'Hostname' and 'Reporter', the
// returned string will then be: Reported at Somehost by Jonathon
func InterpolateString(formatRegexp string, matchParts MatchSet) (newString string) {
	return varMatcher.ReplaceAllStringFunc(formatRegexp,
		func(matchWord string) string {
			// Remove the preceeding @
			m := matchWord[1:]
			if repl, ok := matchParts[m]; ok {
				return repl
			}
			return fmt.Sprintf("<%s>", m)
		})
}
