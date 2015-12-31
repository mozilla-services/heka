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
	"syscall"
)

// Define it since it is not defined for Windows.

// Note that you will need to manually send signal 10 to hekad as
// SIGUSR1 isn't defined on Windows.

const SIGUSR1 = syscall.Signal(0xa)
const SIGUSR2 = syscall.Signal(0xb)
