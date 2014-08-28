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
#
# ***** END LICENSE BLOCK *****/

package message

import (
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
