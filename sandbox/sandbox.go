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
#   Mike Trinkala (trink@mozilla.com)
#
# ***** END LICENSE BLOCK *****/

package sandbox

/*
#include "sandbox.h"
*/
import "C"

const (
	STATUS_UNKNOWN    = C.STATUS_UNKNOWN
	STATUS_RUNNING    = C.STATUS_RUNNING
	STATUS_TERMINATED = C.STATUS_TERMINATED

	USAGE_LIMIT   = C.USAGE_LIMIT
	USAGE_CURRENT = C.USAGE_CURRENT
	USAGE_MAXIMUM = C.USAGE_MAXIMUM
)

type Sandbox interface {
	// Sandbox control
	Init() error
	Destroy()

	// Sandbox state
	Status() int
	LastError() string
	Memory(usage int) int
	Instructions(usage int) int

	// Plugin functions
	ProcessMessage(msg string) int
	TimerEvent() int

	// Go callbacks
	Print(s string)
	SendMessage(msg string)
}
