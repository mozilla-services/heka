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
	"code.google.com/p/gomock/gomock"
	gs "github.com/orfjackal/gospec/src/gospec"
	mocks "heka/pipeline/mocks"
)

func getIncrPipelinePack() *PipelinePack {
	pipelinePack := getTestPipelinePack()

	fields := make(map[string]interface{})
	pipelinePack.Message.Fields = fields

	// Force the message to be a statsd increment message
	pipelinePack.Message.Logger = "thenamespace"
	pipelinePack.Message.Fields["name"] = "myname"
	pipelinePack.Message.Fields["rate"] = float64(30.0)
	pipelinePack.Message.Fields["type"] = "counter"
	pipelinePack.Message.Payload = "-1"
	return pipelinePack
}

func OutputsSpec(c gs.Context) {
	c.Specify("A StatsdOutput", func() {

		t := new(SimpleT)
		ctrl := gomock.NewController(t)

		// Setup of the pipelinePack in here
		pipelinePack := getIncrPipelinePack()

		mockClient := mocks.NewMockStatsdClient(ctrl)
		mockClient.EXPECT().IncrementSampledCounter("myname", -1, float32(30))
		statsdOutput := NewStatsdOutput(mockClient)
		statsdOutput.Deliver(pipelinePack)
	})

	c.Specify("statsdoutput doesn't crash on missing rate", func() {

		t := &SimpleT{}
		ctrl := gomock.NewController(t)

		// Setup of the pipelinePack in here
		pipelinePack := getIncrPipelinePack()

		pipelinePack.Message.Fields["rate"] = nil

		mockClient := mocks.NewMockStatsdClient(ctrl)
		statsdOutput := NewStatsdOutput(mockClient)
		statsdOutput.Deliver(pipelinePack)

		// Finish() will only run successfully if all calls have been
		// expected
		ctrl.Finish()
	})

	c.Specify("statsdoutput doesn't crash on missing name", func() {

		t := &SimpleT{}
		ctrl := gomock.NewController(t)

		// Setup of the pipelinePack in here
		pipelinePack := getIncrPipelinePack()

		pipelinePack.Message.Fields["name"] = nil

		mockClient := mocks.NewMockStatsdClient(ctrl)
		statsdOutput := NewStatsdOutput(mockClient)
		statsdOutput.Deliver(pipelinePack)

		// Finish() will only run successfully if all calls have been
		// expected
		ctrl.Finish()
	})

	c.Specify("statsdoutput doesn't crash on incomplete fields dict", func() {

		t := &SimpleT{}
		ctrl := gomock.NewController(t)

		// Setup of the pipelinePack in here
		pipelinePack := getIncrPipelinePack()

		// Clear the Fields map, let gc clean it up
		pipelinePack.Message.Fields = make(map[string]interface{})

		mockClient := mocks.NewMockStatsdClient(ctrl)
		statsdOutput := NewStatsdOutput(mockClient)
		statsdOutput.Deliver(pipelinePack)

		// Finish() will only run successfully if all calls have been
		// expected
		ctrl.Finish()
	})

	c.Specify("statsdoutput doesn't crash on invalid msg.Type", func() {
		t := &SimpleT{}
		ctrl := gomock.NewController(t)

		// Setup of the pipelinePack in here
		pipelinePack := getIncrPipelinePack()

		pipelinePack.Message.Type = "garbage"

		mockClient := mocks.NewMockStatsdClient(ctrl)
		statsdOutput := NewStatsdOutput(mockClient)
		statsdOutput.Deliver(pipelinePack)

		// Finish() will only run successfully if all calls have been
		// expected
		ctrl.Finish()
	})

}
