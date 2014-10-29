/***** BEGIN LICENSE BLOCK *****
# This Source Code Form is subject to the terms of the Mozilla Public
# License, v. 2.0. If a copy of the MPL was not distributed with this file,
# You can obtain one at http://mozilla.org/MPL/2.0/.
#
# The Initial Developer of the Original Code is the Mozilla Foundation.
# Portions created by the Initial Developer are Copyright (C) 2014
# the Initial Developer. All Rights Reserved.
#
# Contributor(s):
#   Rob Miller (rmiller@mozilla.com)
#
# ***** END LICENSE BLOCK *****/

package message

import (
	"testing"
)

func TestEpochInt(t *testing.T) {
	ts, err := ForgivingTimeParse("Epoch", "1414448234", nil)
	if err != nil {
		t.Error("Error parsing Epoch time")
	}
	if ts.Unix() != 1414448234 {
		t.Errorf("Wrong Epoch time: %d", ts.Unix())
	}
}

func TestEpochFloat(t *testing.T) {
	ts, err := ForgivingTimeParse("Epoch", "1414448234.638504391", nil)
	if err != nil {
		t.Error("Error parsing Epoch time w/ float")
	}
	if ts.UnixNano() != 1414448234638504391 {
		t.Errorf("Wrong Epoch time w/ float: %d", ts.UnixNano())
	}
}

func TestEpochMilliInt(t *testing.T) {
	ts, err := ForgivingTimeParse("EpochMilli", "1414448234638", nil)
	if err != nil {
		t.Error("Error parsing EpochMilli time")
	}
	if ts.UnixNano() != 1414448234638000000 {
		t.Errorf("Wrong EpochMilli time: %d", ts.UnixNano())
	}
}

func TestEpochMilliFloat(t *testing.T) {
	ts, err := ForgivingTimeParse("EpochMilli", "1414448234638.504391", nil)
	if err != nil {
		t.Error("Error parsing EpochMilli time w/ float")
	}
	if ts.UnixNano() != 1414448234638504391 {
		t.Errorf("Wrong EpochMilli time w/ float: %d", ts.UnixNano())
	}
}

func TestEpochMilliFloatTooPrecise(t *testing.T) {
	_, err := ForgivingTimeParse("EpochMilli", "1414448234638.5043911", nil)
	if err == nil {
		t.Error("EpochMilli allowed finer than nanosecond precision")
	}
}

func TestEpochMicroInt(t *testing.T) {
	ts, err := ForgivingTimeParse("EpochMicro", "1414448234638504", nil)
	if err != nil {
		t.Error("Error parsing EpochMicro time")
	}
	if ts.UnixNano() != 1414448234638504000 {
		t.Errorf("Wrong EpochMicro time: %d", ts.UnixNano())
	}
}

func TestEpochMicroFloat(t *testing.T) {
	ts, err := ForgivingTimeParse("EpochMicro", "1414448234638504.391", nil)
	if err != nil {
		t.Error("Error parsing EpochMicro time w/ float")
	}
	if ts.UnixNano() != 1414448234638504391 {
		t.Errorf("Wrong EpochMicro time w/ float: %d", ts.UnixNano())
	}
}

func TestEpochNanoInt(t *testing.T) {
	ts, err := ForgivingTimeParse("EpochNano", "1414448234638504391", nil)
	if err != nil {
		t.Error("Error parsing EpochNano time")
	}
	if ts.UnixNano() != 1414448234638504391 {
		t.Errorf("Wrong EpochNano time: %d", ts.UnixNano())
	}
}

func TestEpochNanoFloat(t *testing.T) {
	_, err := ForgivingTimeParse("EpochNano", "1414448234638504391.1", nil)
	if err == nil {
		t.Error("EpochNano allowed float value")
	}
}
