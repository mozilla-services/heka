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
	"code.google.com/p/go-uuid/uuid"
	"code.google.com/p/gomock/gomock"
	"github.com/mozilla-services/heka/client"
	"github.com/mozilla-services/heka/message"
	ts "github.com/mozilla-services/heka/testsupport"
	gs "github.com/rafrombrc/gospec/src/gospec"
	"github.com/streadway/amqp"
	"sync"
	"time"
)

func AMQPPluginSpec(c gs.Context) {
	t := &ts.SimpleT{}
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	config := NewPipelineConfig(nil)

	// Our two user/conn waitgroups
	ug := new(sync.WaitGroup)
	cg := new(sync.WaitGroup)

	// Setup the mock channel
	mch := NewMockAMQPChannel(ctrl)

	// Setup the mock amqpHub with the mock chan return
	aqh := NewMockAMQPConnectionHub(ctrl)
	aqh.EXPECT().GetChannel("").Return(mch, ug, cg, nil)
	var oldHub AMQPConnectionHub
	oldHub = amqpHub
	amqpHub = aqh
	defer func() {
		amqpHub = oldHub
	}()

	c.Specify("An amqp input", func() {

		// Setup all the mock calls for Init
		mch.EXPECT().ExchangeDeclare("", "", false, true, false, false,
			gomock.Any()).Return(nil)
		mch.EXPECT().QueueDeclare("", false, true, false, false,
			gomock.Any()).Return(amqp.Queue{}, nil)
		mch.EXPECT().QueueBind("", "test", "", false, gomock.Any()).Return(nil)
		mch.EXPECT().Qos(2, 0, false).Return(nil)

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

		ith.MockInputRunner.EXPECT().InChan().Return(ith.PackSupply)
		ith.MockHelper.EXPECT().DecoderSet().Times(2).Return(ith.MockDecoderSet)
		encCall := ith.MockDecoderSet.EXPECT().ByEncodings()
		encCall.Return(ith.Decoders, nil)

		c.Specify("with a valid setup and no decoders", func() {

			amqpInput := new(AMQPInput)
			defaultConfig := amqpInput.ConfigStruct().(*AMQPInputConfig)
			defaultConfig.URL = ""
			defaultConfig.Exchange = ""
			defaultConfig.ExchangeType = ""
			defaultConfig.RoutingKey = "test"
			err := amqpInput.Init(defaultConfig)
			c.Assume(err, gs.IsNil)
			c.Expect(amqpInput.ch, gs.Equals, mch)

			c.Specify("consumes a message", func() {
				// Create a channel to send data to the input
				// Drop a message on there and close the channel
				streamChan := make(chan amqp.Delivery, 1)
				ack := ts.NewMockAcknowledger(ctrl)
				ack.EXPECT().Ack(gomock.Any(), false)
				streamChan <- amqp.Delivery{
					ContentType:  "text/plain",
					Body:         []byte("This is a message"),
					Timestamp:    time.Now(),
					Acknowledger: ack,
				}
				mch.EXPECT().Consume("", "", false, false, false, false,
					gomock.Any()).Return(streamChan, nil)

				// Expect the injected packet
				ith.MockInputRunner.EXPECT().Inject(gomock.Any())

				// Increase the usage since Run decrements it on close
				ug.Add(1)

				ith.PackSupply <- ith.Pack
				go func() {
					amqpInput.Run(ith.MockInputRunner, ith.MockHelper)
				}()
				ith.PackSupply <- ith.Pack
				c.Expect(ith.Pack.Message.GetType(), gs.Equals, "amqp")
				c.Expect(ith.Pack.Message.GetPayload(), gs.Equals, "This is a message")
				close(streamChan)
			})

			c.Specify("consumes a serialized message", func() {
				encoder := client.NewProtobufEncoder(nil)
				streamChan := make(chan amqp.Delivery, 1)

				msg := new(message.Message)
				msg.SetUuid(uuid.NewRandom())
				msg.SetTimestamp(time.Now().UnixNano())
				msg.SetType("logfile")
				msg.SetLogger("/a/nice/path")
				msg.SetSeverity(int32(0))
				msg.SetEnvVersion("0.2")
				msg.SetPid(0)
				msg.SetPayload("This is a message")
				msg.SetHostname("TestHost")

				msgBody := make([]byte, 0, 500)
				_ = encoder.EncodeMessageStream(msg, &msgBody)

				ack := ts.NewMockAcknowledger(ctrl)
				ack.EXPECT().Ack(gomock.Any(), false)

				streamChan <- amqp.Delivery{
					ContentType:  "application/hekad",
					Body:         msgBody,
					Timestamp:    time.Now(),
					Acknowledger: ack,
				}
				mch.EXPECT().Consume("", "", false, false, false, false,
					gomock.Any()).Return(streamChan, nil)

				// Expect the decoded pack
				mockDecoderRunner := ith.Decoders[message.Header_PROTOCOL_BUFFER].(*MockDecoderRunner)
				mockDecoderRunner.EXPECT().InChan().Return(ith.DecodeChan)

				// Increase the usage since Run decrements it on close
				ug.Add(1)

				ith.PackSupply <- ith.Pack
				go func() {
					amqpInput.Run(ith.MockInputRunner, ith.MockHelper)
				}()
				packRef := <-ith.DecodeChan
				c.Expect(ith.Pack, gs.Equals, packRef)
				// Ignore leading 5 bytes of encoded message as thats the header
				c.Expect(string(packRef.MsgBytes), gs.Equals, string(msgBody[5:]))
				ith.PackSupply <- ith.Pack
				close(streamChan)
			})
		})

		c.Specify("with a valid setup using decoders", func() {
			amqpInput := new(AMQPInput)
			defaultConfig := amqpInput.ConfigStruct().(*AMQPInputConfig)
			defaultConfig.URL = ""
			defaultConfig.Exchange = ""
			defaultConfig.ExchangeType = ""
			defaultConfig.RoutingKey = "test"
			defaultConfig.Decoders = []string{"defaultDecode"}
			err := amqpInput.Init(defaultConfig)
			c.Assume(err, gs.IsNil)
			c.Expect(amqpInput.ch, gs.Equals, mch)

			c.Specify("consumes a message", func() {
				// Create a channel to send data to the input
				// Drop a message on there and close the channel
				streamChan := make(chan amqp.Delivery, 1)
				ack := ts.NewMockAcknowledger(ctrl)
				ack.EXPECT().Ack(gomock.Any(), false)
				streamChan <- amqp.Delivery{
					ContentType:  "text/plain",
					Body:         []byte("This is a message"),
					Timestamp:    time.Now(),
					Acknowledger: ack,
				}
				mch.EXPECT().Consume("", "", false, false, false, false,
					gomock.Any()).Return(streamChan, nil)

				// Make the mock decoder runner and decoder for the 'defaultDecode'
				dmr := NewMockDecoderRunner(ctrl)
				mdec := NewMockDecoder(ctrl)
				ith.MockDecoderSet.EXPECT().ByName("defaultDecode").Return(dmr, true)
				dmr.EXPECT().Decoder().Return(mdec)
				mdec.EXPECT().Decode(ith.Pack).Return(nil)

				// Expect the injected packet
				ith.MockInputRunner.EXPECT().Inject(gomock.Any())

				// Increase the usage since Run decrements it on close
				ug.Add(1)

				ith.PackSupply <- ith.Pack
				go func() {
					amqpInput.Run(ith.MockInputRunner, ith.MockHelper)
				}()
				ith.PackSupply <- ith.Pack
				c.Expect(ith.Pack.Message.GetType(), gs.Equals, "amqp")
				c.Expect(ith.Pack.Message.GetPayload(), gs.Equals, "This is a message")
				close(streamChan)
			})
		})
	})

	c.Specify("An amqp output", func() {
		oth := NewOutputTestHelper(ctrl)
		pConfig := NewPipelineConfig(nil)

		amqpOutput := new(AMQPOutput)
		defaultConfig := amqpOutput.ConfigStruct().(*AMQPOutputConfig)
		defaultConfig.URL = ""
		defaultConfig.Exchange = ""
		defaultConfig.ExchangeType = ""
		defaultConfig.RoutingKey = "test"

		closeChan := make(chan *amqp.Error)

		inChan := make(chan *PipelinePack, 1)

		mch.EXPECT().NotifyClose(gomock.Any()).Return(closeChan)
		mch.EXPECT().ExchangeDeclare("", "", false, true, false, false,
			gomock.Any()).Return(nil)

		// Increase the usage since Run decrements it on close
		ug.Add(1)

		// Expect the close and the InChan calls
		aqh.EXPECT().Close("", cg)
		oth.MockOutputRunner.EXPECT().InChan().Return(inChan)

		msg := getTestMessage()
		pack := NewPipelinePack(pConfig.inputRecycleChan)
		pack.Message = msg
		pack.Decoded = true

		c.Specify("publishes a plain message", func() {
			defaultConfig.Serialize = false

			err := amqpOutput.Init(defaultConfig)
			c.Assume(err, gs.IsNil)
			c.Expect(amqpOutput.ch, gs.Equals, mch)

			mch.EXPECT().Publish("", "test", false, false, gomock.Any()).Return(nil)
			inChan <- pack
			close(inChan)

			go func() {
				amqpOutput.Run(oth.MockOutputRunner, oth.MockHelper)
			}()

			ug.Wait()

		})

		c.Specify("publishes a serialized message", func() {
			err := amqpOutput.Init(defaultConfig)
			c.Assume(err, gs.IsNil)
			c.Expect(amqpOutput.ch, gs.Equals, mch)

			mch.EXPECT().Publish("", "test", false, false, gomock.Any()).Return(nil)
			inChan <- pack
			close(inChan)

			go func() {
				amqpOutput.Run(oth.MockOutputRunner, oth.MockHelper)
			}()
			ug.Wait()
		})
	})
}
