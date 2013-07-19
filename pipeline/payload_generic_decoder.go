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

package pipeline

import (
	"fmt"
	. "github.com/mozilla-services/heka/message"
	"strconv"
	"time"
)

type GenericDecoder struct {
	Captures        map[string]string
	dRunner         DecoderRunner
	TimestampLayout string
	TzLocation      *time.Location
	SeverityMap     map[string]int32
}

func (gd *GenericDecoder) DecodeTimestamp(pack *PipelinePack) {
	if timeStamp, ok := gd.Captures["Timestamp"]; ok {
		val, err := ForgivingTimeParse(gd.TimestampLayout, timeStamp, gd.TzLocation)
		if err != nil {
			gd.dRunner.LogError(fmt.Errorf("Don't recognize Timestamp: '%s'", timeStamp))
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

func (gd *GenericDecoder) DecodeSeverity(pack *PipelinePack) {
	if sevStr, ok := gd.Captures["Severity"]; ok {
		// If so, see if we have a mapping for this severity.
		if sevInt, ok := gd.SeverityMap[sevStr]; ok {
			pack.Message.SetSeverity(sevInt)
		} else {
			// No mapping => severity value should be an int.
			sevInt, err := strconv.ParseInt(sevStr, 10, 32)
			if err != nil {
				gd.dRunner.LogError(fmt.Errorf("Don't recognize severity: '%s'", sevStr))
			} else {
				pack.Message.SetSeverity(int32(sevInt))
			}
		}
	}
}
