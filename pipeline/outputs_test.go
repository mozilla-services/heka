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
#   Victor Ng (vng@mozilla.com)
#
# ***** END LICENSE BLOCK *****/
package pipeline

import (
	gs "github.com/orfjackal/gospec/src/gospec"
)

func OutputsSpec(c gs.Context) {
	// TODO: add stuff here

	c.Specify("A StatsdOutput", func() {

		pipelinePack := getTestPipelinePack()

		fields := make(map[string]interface{})
		pipelinePack.Message.Fields = fields

		// Force the message to be a statsd increment message
		pipelinePack.Message.Logger = "thenamespace"
		pipelinePack.Message.Fields["name"] = "myname"
		pipelinePack.Message.Fields["rate"] = "30"
		pipelinePack.Message.Fields["type"] = "counter"
		pipelinePack.Message.Payload = "-1"

		statsdOutput := NewStatsdOutput(NewStatsdClient("localhost:5565"))
		statsdOutput.Deliver(pipelinePack)
	})
}
