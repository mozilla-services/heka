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

package message

import (
	"regexp"
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

	dateMatchStrings = []string{
		// Mon Jan _2 15:04:05 2006
		"SDAY SMONTH\\s{1,2}\\d{1,2} \\d{2}:\\d{2}:\\d{2} \\d{4}",
		// Mon Jan _2 15:04:05 MST 2006
		"SDAY SMONTH\\s{1,2}\\d{1,2} \\d{2}:\\d{2}:\\d{2} \\w{3} \\d{4}",
		// Mon Jan 02 15:04:05 -0700 2006
		"SDAY SMONTH \\d{2} \\d{2}:\\d{2}:\\d{2} -\\d{4} \\d{4}",
		// 02 Jan 06 15:04 MST
		"SDAY SMONTH \\d{2} \\d{2}:\\d{2} \\w{3}",
		// 02 Jan 06 15:04 -0700
		"\\d{2} SMONTH \\d{2} \\d{2}:\\d{2} \\w{3} -\\d{4}",
		// Monday, 02-Jan-06 15:04:05 MST
		"DAY \\d{2}-SMONTH-\\d{2} \\d{2}:\\d{2}:\\d{2} \\w{3}",
		// Mon, 02 Jan 2006 15:04:05 MST
		"SDAY \\d{2} SMONTH \\d{4} \\d{2}:\\d{2}:\\d{2} \\w{3}",
		// Mon, 02 Jan 2006 15:04:05 -0700
		"SDAY \\d{2} SMONTH \\d{4} \\d{2}:\\d{2}:\\d{2} \\w{3} -\\d{4}",
		// 2006-01-02T15:04:05Z07:00
		"\\d{4}-\\d{2}-\\d{2}T\\d{2}:\\d{2}:\\d{2}Z\\d{2}:\\d{2}",
		// 2006-01-02T15:04:05.999999999Z07:00
		"\\d{4}-\\d{2}-\\d{2}T\\d{2}:\\d{2}:\\d+Z\\d{2}:\\d{2}",
		// 3:04PM - Kitchen format.... really? Kitchen? sigh.
		"\\d{1,2}:\\d{2}[AP]M",
		// Jan _2 15:04:05
		"SMONTH\\s{1,2}\\d{1,2} \\d{2}:\\d{2}:\\d{2}",
		// Jan _2 15:04:05.000
		"SMONTH\\s{1,2}\\d{1,2} \\d{2}:\\d{2}:\\d{2}\\.\\d{3}",
		// Jan _2 15:04:05.000000
		"SMONTH\\s{1,2}\\d{1,2} \\d{2}:\\d{2}:\\d{2}\\.\\d{6}",
		// Jan _2 15:04:05.000000000
		"SMONTH\\s{1,2}\\d{1,2} \\d{2}:\\d{2}:\\d{2}\\.\\d{9}",
	}

	// We have to duplicate this, cause time doesn't export them. Blech.
	longDayNames = []string{
		"Sunday",
		"Monday",
		"Tuesday",
		"Wednesday",
		"Thursday",
		"Friday",
		"Saturday",
	}

	shortDayNames = []string{
		"Sun",
		"Mon",
		"Tue",
		"Wed",
		"Thu",
		"Fri",
		"Sat",
	}

	shortMonthNames = []string{
		"---",
		"Jan",
		"Feb",
		"Mar",
		"Apr",
		"May",
		"Jun",
		"Jul",
		"Aug",
		"Sep",
		"Oct",
		"Nov",
		"Dec",
	}

	// A mapping that returns a complex regular expression string for
	// a commonly matched portion rather than having to construct one.
	//
	// Currently HelperRegexSubs has the following keys upon startup that
	// may be used:
	//     TIMESTAMP  -  A complex regular expression string that matches
	//                   any of the Go time const layouts.
	HelperRegexSubs map[string]string
)

// Parse a time with the supplied timeLayout, falling back to all the
// basicTimeLayouts.
func ForgivingTimeParse(timeLayout, inputTime string, loc *time.Location) (
	parsedTime time.Time, err error) {

	parsedTime, err = time.ParseInLocation(timeLayout, inputTime, loc)
	if err == nil {
		return
	}
	for _, layout := range basicTimeLayouts {
		parsedTime, err = time.ParseInLocation(layout, inputTime, loc)
		if err == nil {
			return
		}
	}
	return
}

func init() {
	HelperRegexSubs = make(map[string]string)

	smonths := "(?:" + strings.Join(shortMonthNames, "|") + ")"
	sdays := "(?:" + strings.Join(shortDayNames, "|") + ")"
	days := "(?:" + strings.Join(longDayNames, "|") + ")"

	newMatchStrings := make([]string, 0, 15)
	replaceShorts, _ := regexp.Compile("(SDAY|DAY|SMONTH)")
	for _, dateStr := range dateMatchStrings {
		newStr := replaceShorts.ReplaceAllStringFunc(dateStr,
			func(match string) string {
				switch match {
				case "SDAY":
					return sdays
				case "DAY":
					return days
				case "SMONTH":
					return smonths
				}
				return match
			})
		newMatchStrings = append(newMatchStrings, "(?:"+newStr+")")
	}
	tsRegexString := "(?P<Timestamp>" + strings.Join(newMatchStrings, "|") + ")"
	HelperRegexSubs["TIMESTAMP"] = tsRegexString
}
