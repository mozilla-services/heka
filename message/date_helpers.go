/***** BEGIN LICENSE BLOCK *****
# This Source Code Form is subject to the terms of the Mozilla Public
# License, v. 2.0. If a copy of the MPL was not distributed with this file,
# You can obtain one at http://mozilla.org/MPL/2.0/.
#
# The Initial Developer of the Original Code is the Mozilla Foundation.
# Portions created by the Initial Developer are Copyright (C) 2012-2014
# the Initial Developer. All Rights Reserved.
#
# Contributor(s):
#   Ben Bangert (bbangert@mozilla.com)
#   Rob Miller (rmiller@mozilla.com)
#
# ***** END LICENSE BLOCK *****/

package message

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

var (
	basicTimeLayouts = map[string]string{
		"ANSIC":       time.ANSIC,
		"UnixDate":    time.UnixDate,
		"RubyDate":    time.RubyDate,
		"RFC822":      time.RFC822,
		"RFC822Z":     time.RFC822Z,
		"RFC850":      time.RFC850,
		"RFC1123":     time.RFC1123,
		"RFC1123Z":    time.RFC1123Z,
		"RFC3339":     time.RFC3339,
		"RFC3339Nano": time.RFC3339Nano,
		"Kitchen":     time.Kitchen,
		"Stamp":       time.Stamp,
		"StampMilli":  time.StampMilli,
		"StampMicro":  time.StampMicro,
		"StampNano":   time.StampNano,
	}
)

// Parse a time with the supplied timeLayout, falling back to all the
// basicTimeLayouts.
func ForgivingTimeParse(timeLayout, inputTime string, loc *time.Location) (time.Time, error) {

	var (
		parsedTime time.Time
		err        error
	)

	if strings.HasPrefix(timeLayout, "Epoch") {
		var (
			parsedInt  uint64
			multiplier int
		)

		switch timeLayout {
		case "Epoch":
			multiplier = 9
		case "EpochMilli":
			multiplier = 6
		case "EpochMicro":
			multiplier = 3
		case "EpochNano":
			multiplier = 0
		default:
			err := fmt.Errorf("Unrecognized `Epoch` time format: %s", timeLayout)
			return parsedTime, err
		}

		i := strings.Index(inputTime, ".")
		if i == -1 {
			// Integer values are easy, we append the right number of 0s and
			// we're done.
			zeroes := strings.Repeat("0", multiplier)
			inputTime = inputTime + zeroes
			parsedInt, err = strconv.ParseUint(inputTime, 10, 64)
		} else {
			// Noninteger need more care, we can't use floats or we'll lose
			// timestamp precision. First calculate the number of decimal
			// digits.
			decDigits := len(inputTime) - i - 1
			if decDigits < multiplier {
				// Pad out zeroes to nanosecond resolution.
				zeroes := strings.Repeat("0", multiplier-decDigits)
				inputTime = inputTime + zeroes
			} else if decDigits > multiplier {
				// Truncate to nanosecond resolution.
				inputTime = inputTime[:len(inputTime)-(decDigits-multiplier)]
			}
			// Finally remove the decimal and parse the value as an integer.
			intStr := fmt.Sprintf("%s%s", inputTime[:i], inputTime[i+1:])
			parsedInt, err = strconv.ParseUint(intStr, 10, 64)
		}
		if err != nil {
			err = fmt.Errorf("Error parsing %s time: %s", timeLayout, err.Error())
			return parsedTime, err
		}
		return time.Unix(0, int64(parsedInt)), nil
	}

	if parsedTime, err = time.ParseInLocation(timeLayout, inputTime, loc); err == nil {
		return parsedTime, nil
	}

	for _, layout := range basicTimeLayouts {
		if parsedTime, err = time.ParseInLocation(layout, inputTime, loc); err == nil {
			return parsedTime, nil
		}
	}
	return parsedTime, err
}
