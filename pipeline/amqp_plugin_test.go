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
#   Rob Miller (rmiller@mozilla.com)
#
# ***** END LICENSE BLOCK *****/

package pipeline

import (
	"code.google.com/p/gomock/gomock"
	"github.com/mozilla-services/heka/message"
	ts "github.com/mozilla-services/heka/testsupport"
	gs "github.com/rafrombrc/gospec/src/gospec"
	"github.com/streadway/amqp"
	"sync"
)

func AMQPPluginSpec(c gs.Context) {
	t := &ts.SimpleT{}
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	config := NewPipelineConfig(nil)
	ith := new(InputTestHelper)
	ith.Msg = getTestMessage()
	ith.Pack = NewPipelinePack(config.inputRecycleChan)

	// set up mock helper, decoder set, and packSupply channel
	ith.MockHelper = NewMockPluginHelper(ctrl)
	ith.MockInputRunner = NewMockInputRunner(ctrl)
	ith.Decoders = make([]DecoderRunner, int(message.Header_JSON+1))
	ith.Decoders[message.Header_PROTOCOL_BUFFER] = NewMockDecoderRunner(ctrl)
	ith.Decoders[message.Header_JSON] = NewMockDecoderRunner(ctrl)
	ith.PackSupply = make(chan *PipelinePack, 1)
	ith.DecodeChan = make(chan *PipelinePack)
	ith.MockDecoderSet = NewMockDecoderSet(ctrl)

	c.Specify("An amqp input", func() {
		// Setup the mock channel
		mch := NewMockAMQPChannel(ctrl)

		// Setup all the mock calls for Init
		mch.EXPECT().ExchangeDeclare("", "", false, true, false, false,
			gomock.Any()).Return(nil)
		mch.EXPECT().QueueDeclare("", false, true, false, false,
			gomock.Any()).Return(amqp.Queue{}, nil)
		mch.EXPECT().QueueBind("", "test", "", false, gomock.Any()).Return(nil)
		mch.EXPECT().Qos(2, 0, false).Return(nil)

		// Our two user/conn waitgroups
		ug := new(sync.WaitGroup)
		cg := new(sync.WaitGroup)

		// Setup the mock amqpHub with the mock chan return
		aqh := NewMockAMQPConnectionHub(ctrl)
		aqh.EXPECT().GetChannel("").Return(mch, ug, cg, nil)
		var oldHub AMQPConnectionHub
		oldHub = amqpHub
		amqpHub = aqh
		defer func() {
			amqpHub = oldHub
		}()

		amqpInput := new(AMQPInput)
		defaultConfig := amqpInput.ConfigStruct().(*AMQPInputConfig)
		defaultConfig.URL = ""
		defaultConfig.Exchange = ""
		defaultConfig.ExchangeType = ""
		defaultConfig.RoutingKey = "test"
		err := amqpInput.Init(defaultConfig)
		c.Assume(err, gs.IsNil)
		c.Expect(amqpInput.ch, gs.Equals, mch)

	})
}
