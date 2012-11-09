package pipeline

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

/*
This module provides a customized JSON Marshaler that will serialize
timestamps without encoding the timezone data.
*/

import (
	"time"
)

// The timezone information has been stripped as 
// everything should be encoded to UTC time
const (
	TimeFormat           = "2006-01-02T15:04:05.000000"
	TimeFormatFullSecond = "2006-01-02T15:04:05"
)

type UTCTimestamp struct {
	Timestamp time.Time
}

func (self *UTCTimestamp) Format(format string) string {
	return self.Timestamp.Format(format)
}

// IsZero reports whether t represents the zero time instant,
// January 1, year 1, 00:00:00 UTC.
func (self *UTCTimestamp) IsZero() bool {
	return self.Timestamp.IsZero()
}

func (self *UTCTimestamp) String() string {
	return "<hekatime: " + self.Format(TimeFormat) + ">"
}

func (self *UTCTimestamp) Marshal(v interface{}) ([]byte, error) {
	return []byte(v.(time.Time).Format(TimeFormat)), nil
}
