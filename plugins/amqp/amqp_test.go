/***** BEGIN LICENSE BLOCK *****
# This Source Code Form is subject to the terms of the Mozilla Public
# License, v. 2.0. If a copy of the MPL was not distributed with this file,
# You can obtain one at http://mozilla.org/MPL/2.0/.
#
# The Initial Developer of the Original Code is the Mozilla Foundation.
# Portions created by the Initial Developer are Copyright (C) 2012-2014
# the Initial Developer. All Rights Reserved.
#
# Contributor(s):
#   Ben Bangert (bbangert@mozilla.com)
#   Rob Miller (rmiller@mozilla.com)
#
# ***** END LICENSE BLOCK *****/

package amqp

import (
	"code.google.com/p/go-uuid/uuid"
	"code.google.com/p/gomock/gomock"
	"github.com/mozilla-services/heka/client"
	"github.com/mozilla-services/heka/message"
	. "github.com/mozilla-services/heka/pipeline"
	pipeline_ts "github.com/mozilla-services/heka/pipeline/testsupport"
	. "github.com/mozilla-services/heka/pipelinemock"
	"github.com/mozilla-services/heka/plugins"
	plugins_ts "github.com/mozilla-services/heka/plugins/testsupport"
	gs "github.com/rafrombrc/gospec/src/gospec"
	"github.com/streadway/amqp"
	"sync"
	"testing"
	"time"
)

func TestAllSpecs(t *testing.T) {
	r := gs.NewRunner()
	r.Parallel = false

	r.AddSpec(AMQPPluginSpec)

	gs.MainGoTest(r, t)
}

func AMQPPluginSpec(c gs.Context) {
	t := &pipeline_ts.SimpleT{}
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
	aqh.EXPECT().GetChannel("", AMQPDialer{}).Return(mch, ug, cg, nil)
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

		ith := new(plugins_ts.InputTestHelper)
		ith.Msg = pipeline_ts.GetTestMessage()
		ith.Pack = NewPipelinePack(config.InputRecycleChan())

		// set up mock helper, decoder set, and packSupply channel
		ith.MockHelper = NewMockPluginHelper(ctrl)
		ith.MockInputRunner = NewMockInputRunner(ctrl)
		mockDRunner := NewMockDecoderRunner(ctrl)
		ith.PackSupply = make(chan *PipelinePack, 1)
		ith.DecodeChan = make(chan *PipelinePack)

		ith.MockInputRunner.EXPECT().InChan().Return(ith.PackSupply)

		c.Specify("with a valid setup and no decoder", func() {
			amqpInput := new(AMQPInput)
			defaultConfig := amqpInput.ConfigStruct().(*AMQPInputConfig)
			defaultConfig.URL = ""
			defaultConfig.Exchange = ""
			defaultConfig.ExchangeType = ""
			defaultConfig.RoutingKey = "test"
			defaultConfig.QueueTTL = 300000
			err := amqpInput.Init(defaultConfig)
			c.Assume(err, gs.IsNil)
			c.Expect(amqpInput.ch, gs.Equals, mch)

			c.Specify("consumes a message", func() {

				// Create a channel to send data to the input
				// Drop a message on there and close the channel
				streamChan := make(chan amqp.Delivery, 1)
				ack := plugins_ts.NewMockAcknowledger(ctrl)
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
		})

		c.Specify("with a valid setup using a decoder", func() {
			decoderName := "defaultDecoder"
			amqpInput := new(AMQPInput)
			defaultConfig := amqpInput.ConfigStruct().(*AMQPInputConfig)
			defaultConfig.URL = ""
			defaultConfig.Exchange = ""
			defaultConfig.ExchangeType = ""
			defaultConfig.RoutingKey = "test"
			defaultConfig.Decoder = decoderName
			defaultConfig.QueueTTL = 300000
			err := amqpInput.Init(defaultConfig)
			c.Assume(err, gs.IsNil)
			c.Expect(amqpInput.ch, gs.Equals, mch)

			// Mock up our default decoder runner and decoder.
			ith.MockInputRunner.EXPECT().Name().Return("AMQPInput")
			decCall := ith.MockHelper.EXPECT().DecoderRunner(decoderName, "AMQPInput-defaultDecoder")
			decCall.Return(mockDRunner, true)
			mockDecoder := NewMockDecoder(ctrl)
			mockDRunner.EXPECT().Decoder().Return(mockDecoder)

			c.Specify("consumes a message", func() {
				packs := []*PipelinePack{ith.Pack}
				mockDecoder.EXPECT().Decode(ith.Pack).Return(packs, nil)

				// Create a channel to send data to the input
				// Drop a message on there and close the channel
				streamChan := make(chan amqp.Delivery, 1)
				ack := plugins_ts.NewMockAcknowledger(ctrl)
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

				ack := plugins_ts.NewMockAcknowledger(ctrl)
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
				mockDRunner.EXPECT().InChan().Return(ith.DecodeChan)

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
	})

	c.Specify("An amqp output", func() {
		oth := plugins_ts.NewOutputTestHelper(ctrl)
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

		msg := pipeline_ts.GetTestMessage()
		pack := NewPipelinePack(pConfig.InputRecycleChan())
		pack.Message = msg
		pack.Decoded = true

		c.Specify("publishes a plain message", func() {
			encoder := new(plugins.PayloadEncoder)
			econfig := encoder.ConfigStruct().(*plugins.PayloadEncoderConfig)
			econfig.AppendNewlines = false
			encoder.Init(econfig)
			payloadBytes, err := encoder.Encode(pack)

			defaultConfig.Encoder = "PayloadEncoder"
			defaultConfig.ContentType = "text/plain"
			oth.MockOutputRunner.EXPECT().Encoder().Return(encoder)
			oth.MockOutputRunner.EXPECT().Encode(pack).Return(payloadBytes, nil)

			err = amqpOutput.Init(defaultConfig)
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
			encoder := new(ProtobufEncoder)
			encoder.Init(nil)
			protoBytes, err := encoder.Encode(pack)
			c.Expect(err, gs.IsNil)
			oth.MockOutputRunner.EXPECT().Encoder().Return(encoder)
			oth.MockOutputRunner.EXPECT().Encode(pack).Return(protoBytes, nil)

			err = amqpOutput.Init(defaultConfig)
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
