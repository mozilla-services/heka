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
#   Victor Ng (vng@mozilla.com)
#
# ***** END LICENSE BLOCK *****/

package payload

import (
	"fmt"
	. "github.com/mozilla-services/heka/message"
	. "github.com/mozilla-services/heka/pipeline"
	"strconv"
	"time"
)

type PayloadDecoderHelper struct {
	Captures        map[string]string
	dRunner         DecoderRunner
	TimestampLayout string
	TzLocation      *time.Location
	SeverityMap     map[string]int32
}

/*
Timestamps strings are decoded using the TimestampLayout and written
back to the Message as nanoseconds into the Timestamp field.
In the case that a timestamp string is not in the capture map, no
timestamp is written and the default of 0 is used.
*/
func (pdh *PayloadDecoderHelper) DecodeTimestamp(pack *PipelinePack) {
	if timeStamp, ok := pdh.Captures["Timestamp"]; ok {
		val, err := ForgivingTimeParse(pdh.TimestampLayout, timeStamp, pdh.TzLocation)
		if err != nil {
			pdh.dRunner.LogError(fmt.Errorf("Don't recognize Timestamp: '%s'", timeStamp))
		}
		// If we only get a timestamp, use the current date
		if val.Year() == 0 && val.Month() == 1 && val.Day() == 1 {
			now := time.Now()
			val = val.AddDate(now.Year(), int(now.Month()-1), now.Day()-1)
		} else if val.Year() == 0 {
			// If there's no year, use current year
			val = val.AddDate(time.Now().Year(), 0, 0)
		}
		pack.Message.SetTimestamp(val.UnixNano())
	}
}

/*
Severity values for an error may be encoded into the captures map as
stringified integers.  The DecodeSeverity function will decode those
values and write them back into the severity field of the Message.
In the event no severity is found, a default value of 0 is used.
*/
func (pdh *PayloadDecoderHelper) DecodeSeverity(pack *PipelinePack) {
	if sevStr, ok := pdh.Captures["Severity"]; ok {
		// If so, see if we have a mapping for this severity.
		if sevInt, ok := pdh.SeverityMap[sevStr]; ok {
			pack.Message.SetSeverity(sevInt)
		} else {
			// No mapping => severity value should be an int.
			sevInt, err := strconv.ParseInt(sevStr, 10, 32)
			if err != nil {
				pdh.dRunner.LogError(fmt.Errorf("Don't recognize severity: '%s'", sevStr))
			} else {
				pack.Message.SetSeverity(int32(sevInt))
			}
		}
		// Delete from the captures map so we don't try to set severity again
		// in PopulateMessage.
		delete(pdh.Captures, "Severity")
	}
}
