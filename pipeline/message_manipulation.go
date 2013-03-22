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
	"reflect"
	"regexp"
	"strconv"
)

// A set of extracted matches from a regular expression
type MatchSet map[string]string

// Used to match a field and optionally populate a MatchSet
//
// Returns whether the data matches or not
type Matcher func(data string, set MatchSet) bool

// Represents all the fields that can be matched in a message
//
// This type is intended to be used with config that needs to match
// a specific message
// A criteria string beginning with a ~ is interpreted as an indicator
// that the string should be considered a regular expression.
type MatchCriteriaLayout struct {
	Logger   string
	Payload  string
	Type     string
	Hostname string
}

// MessageMatcher holds a compiled MatchCriteriaLayout for matching
// messages and extracting portions
type MessageMatcher map[string]Matcher

func NewMessageMatcher(matchLayout *MatchCriteriaLayout) (MessageMatcher, error) {
	matcher := make(map[string]Matcher)
	val := reflect.ValueOf(matchLayout).Elem()
	typeOfVal := val.Type()
	for fieldIndex := 0; fieldIndex < val.NumField(); fieldIndex++ {
		fieldVal := val.Field(fieldIndex).String()
		if fn, err := newMatcher(fieldVal); err != nil {
			return nil, err
		} else {
			matcher[typeOfVal.Field(fieldIndex).Name] = fn
		}
	}
	return matcher, nil
}

// Creates and returns a Matcher
func newMatcher(field string) (Matcher, error) {
	var fn Matcher
	if len(field) < 1 {
		fn = func(data string, set MatchSet) bool { return true }
	} else if field[0] == '~' {
		// Replace helper words with complex regex
		wordMatcher, _ := regexp.Compile("TIMESTAMP")
		fixedField := wordMatcher.ReplaceAllStringFunc(field[1:],
			func(match string) string {
				if match == "TIMESTAMP" {
					return HelperRegexSubs["TIMESTAMP"]
				}
				return match
			})

		regex, err := regexp.Compile(fixedField)
		if err != nil {
			return nil, fmt.Errorf("Unable to create match regex for string: %s",
				field[1:])
		}

		fn = func(data string, set MatchSet) bool {
			findResults := regex.FindStringSubmatch(data)
			if findResults == nil || len(findResults) < 2 {
				return false
			}
			resultLength := len(findResults)
			for index, name := range regex.SubexpNames() {
				if name == "" {
					continue
				}
				if index > resultLength-1 {
					set[name] = ""
				} else {
					set[name] = findResults[index]
				}
			}
			return true
		}
	} else {
		fn = func(data string, set MatchSet) bool {
			return data == field
		}
	}
	return fn, nil
}

func stringIt(value interface{}) (s string, ok bool) {
	ok = true

	// Now attempt the common conversions
	switch i := value.(type) {
	case nil:
		s = ""
	case int64:
		s = strconv.FormatInt(i, 10)
	case bool:
		s = strconv.FormatBool(i)
	case []byte:
		s = string(i)
	case float64:
		s = strconv.FormatFloat(i, 'g', -1, 64)
	case string:
		s = i
	default:
		ok = false
	}
	return
}

// Determines if a message matches the configured criteria
//
// In the event a message does match, the MatchSet contains all the
// portions that were captured
func (m MessageMatcher) Match(message *Message) (MatchSet, bool) {
	var fieldName string
	var matcher Matcher
	set := map[string]string{
		"Logger":   message.GetLogger(),
		"Payload":  message.GetPayload(),
		"Type":     message.GetType(),
		"Hostname": message.GetHostname(),
		"Pid":      string(message.GetPid()),
		"Uuid":     message.GetUuidString(),
		"Severity": string(message.GetSeverity()),
	}

	matched := true
	for fieldName, matcher = range m {
		switch fieldName {
		case "Logger":
			matched = matcher(message.GetLogger(), set)
		case "Payload":
			matched = matcher(message.GetPayload(), set)
		case "Type":
			matched = matcher(message.GetType(), set)
		case "Hostname":
			matched = matcher(message.GetHostname(), set)
		case "Severity":
			matched = matcher(string(message.GetSeverity()), set)
		default:
			// Search fields
			if val, ok := message.GetFieldValue(fieldName); ok {
				if result, ok := stringIt(val); ok {
					matched = matcher(result, set)
				} else {
					matched = false
				}
			} else {
				matched = false
			}
		}
		if !matched {
			return nil, false
		}
	}
	return set, true
}
