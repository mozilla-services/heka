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
#
# ***** END LICENSE BLOCK *****/

package pipeline

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/mozilla-services/heka/message"
	"github.com/pborman/uuid"
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
func (mt MessageTemplate) PopulateMessage(msg *message.Message, subs map[string]string) error {
	var val string
	for field, rawVal := range mt {
		if subs == nil {
			val = rawVal
		} else {
			val = InterpolateString(rawVal, subs)
		}
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
			intPart := strings.Split(val, ".")[0]
			pid, err := strconv.ParseInt(intPart, 10, 32)
			if err != nil {
				return err
			}
			msg.SetPid(int32(pid))
		case "Severity":
			severity, err := strconv.ParseInt(val, 10, 32)
			if err != nil {
				return err
			}
			msg.SetSeverity(int32(severity))
		case "Uuid":
			if len(val) == message.UUID_SIZE {
				msg.SetUuid([]byte(val))
			} else {
				if uuidBytes := uuid.Parse(val); uuidBytes == nil {
					return errors.New("Invalid UUID string.")
				} else {
					msg.SetUuid(uuidBytes)
				}
			}
		default:
			fi := strings.SplitN(field, "|", 2)
			if len(fi) < 2 {
				fi = append(fi, "")
			}
			f, err := message.NewField(fi[0], val, fi[1])
			msg.AddField(f)
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

// Initialize the varMatcher for use in InterpolateString
func init() {
	varMatcher, _ = regexp.Compile("%\\w+%")
}
