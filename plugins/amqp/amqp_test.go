/***** BEGIN LICENSE BLOCK *****
# This Source Code Form is subject to the terms of the Mozilla Public
# License, v. 2.0. If a copy of the MPL was not distributed with this file,
# You can obtain one at http://mozilla.org/MPL/2.0/.
#
# The Initial Developer of the Original Code is the Mozilla Foundation.
# Portions created by the Initial Developer are Copyright (C) 2012-2015
# the Initial Developer. All Rights Reserved.
#
# Contributor(s):
#   Ben Bangert (bbangert@mozilla.com)
#   Rob Miller (rmiller@mozilla.com)
#
# ***** END LICENSE BLOCK *****/

package amqp

import (
	"sync"
	"testing"
	"time"

	"github.com/mozilla-services/heka/client"
	"github.com/mozilla-services/heka/message"
	. "github.com/mozilla-services/heka/pipeline"
	pipeline_ts "github.com/mozilla-services/heka/pipeline/testsupport"
	. "github.com/mozilla-services/heka/pipelinemock"
	"github.com/mozilla-services/heka/plugins"
	plugins_ts "github.com/mozilla-services/heka/plugins/testsupport"
	"github.com/pborman/uuid"
	"github.com/rafrombrc/gomock/gomock"
	gs "github.com/rafrombrc/gospec/src/gospec"
	"github.com/streadway/amqp"
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

	// Our two user/conn waitgroups.
	ug := new(sync.WaitGroup)
	cg := new(sync.WaitGroup)

	// Setup the mock channel.
	mch := NewMockAMQPChannel(ctrl)

	// Setup the mock amqpHub with the mock chan return.
	aqh := NewMockAMQPConnectionHub(ctrl)
	aqh.EXPECT().GetChannel("", AMQPDialer{}).Return(mch, ug, cg, nil)

	errChan := make(chan error, 1)
	bytesChan := make(chan []byte, 1)

	c.Specify("An amqp input", func() {
		// Setup all the mock calls for Init.
		mch.EXPECT().ExchangeDeclare("", "", false, true, false, false,
			gomock.Any()).Return(nil)
		mch.EXPECT().QueueDeclare("", false, true, false, false,
			gomock.Any()).Return(amqp.Queue{}, nil)
		mch.EXPECT().QueueBind("", "test", "", false, gomock.Any()).Return(nil)
		mch.EXPECT().Qos(2, 0, false).Return(nil)

		ith := new(plugins_ts.InputTestHelper)
		ith.Msg = pipeline_ts.GetTestMessage()
		ith.Pack = NewPipelinePack(config.InputRecycleChan())

		// Set up relevant mocks.
		ith.MockHelper = NewMockPluginHelper(ctrl)
		ith.MockInputRunner = NewMockInputRunner(ctrl)
		ith.MockSplitterRunner = NewMockSplitterRunner(ctrl)
		ith.PackSupply = make(chan *PipelinePack, 1)

		ith.MockInputRunner.EXPECT().NewSplitterRunner("").Return(ith.MockSplitterRunner)

		amqpInput := new(AMQPInput)
		amqpInput.amqpHub = aqh
		config := amqpInput.ConfigStruct().(*AMQPInputConfig)
		config.URL = ""
		config.Exchange = ""
		config.ExchangeType = ""
		config.RoutingKey = "test"
		config.QueueTTL = 300000

		err := amqpInput.Init(config)
		c.Assume(err, gs.IsNil)
		c.Expect(amqpInput.ch, gs.Equals, mch)

		c.Specify("consumes a text message", func() {
			// Create a channel to send data to the input. Drop a message on
			// there and close the channel.
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

			// Increase the usage since Run decrements it on close.
			ug.Add(1)

			splitCall := ith.MockSplitterRunner.EXPECT().SplitBytes(gomock.Any(),
				nil)
			splitCall.Do(func(recd []byte, del Deliverer) {
				bytesChan <- recd
			})
			ith.MockSplitterRunner.EXPECT().UseMsgBytes().Return(false)
			ith.MockSplitterRunner.EXPECT().SetPackDecorator(gomock.Any())
			ith.MockSplitterRunner.EXPECT().Done()
			go func() {
				err := amqpInput.Run(ith.MockInputRunner, ith.MockHelper)
				errChan <- err
			}()

			msgBytes := <-bytesChan
			c.Expect(string(msgBytes), gs.Equals, "This is a message")
			close(streamChan)
			err = <-errChan
		})

		c.Specify("consumes a protobuf encoded message", func() {
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

			// Increase the usage since Run decrements it on close.
			ug.Add(1)

			splitCall := ith.MockSplitterRunner.EXPECT().SplitBytes(gomock.Any(),
				nil)
			splitCall.Do(func(recd []byte, del Deliverer) {
				bytesChan <- recd
			})
			ith.MockSplitterRunner.EXPECT().UseMsgBytes().Return(true)
			ith.MockSplitterRunner.EXPECT().Done()
			go func() {
				err := amqpInput.Run(ith.MockInputRunner, ith.MockHelper)
				errChan <- err
			}()

			msgBytes := <-bytesChan
			c.Expect(string(msgBytes), gs.Equals, string(msgBody))
			close(streamChan)
			err = <-errChan
			c.Expect(err, gs.IsNil)
		})
	})

	c.Specify("An amqp output", func() {
		oth := plugins_ts.NewOutputTestHelper(ctrl)
		pConfig := NewPipelineConfig(nil)

		amqpOutput := new(AMQPOutput)
		amqpOutput.amqpHub = aqh
		config := amqpOutput.ConfigStruct().(*AMQPOutputConfig)
		config.URL = ""
		config.Exchange = ""
		config.ExchangeType = ""
		config.RoutingKey = "test"

		closeChan := make(chan *amqp.Error)

		inChan := make(chan *PipelinePack, 1)

		mch.EXPECT().NotifyClose(gomock.Any()).Return(closeChan)
		mch.EXPECT().ExchangeDeclare("", "", false, true, false, false,
			gomock.Any()).Return(nil)

		// Increase the usage since Run decrements it on close.
		ug.Add(1)

		// Expect the close and the InChan calls.
		aqh.EXPECT().Close("", cg)
		oth.MockOutputRunner.EXPECT().InChan().Return(inChan)

		msg := pipeline_ts.GetTestMessage()
		pack := NewPipelinePack(pConfig.InputRecycleChan())
		pack.Message = msg
		pack.QueueCursor = "queuecursor"
		oth.MockOutputRunner.EXPECT().UpdateCursor(pack.QueueCursor)

		c.Specify("publishes a plain message", func() {
			encoder := new(plugins.PayloadEncoder)
			econfig := encoder.ConfigStruct().(*plugins.PayloadEncoderConfig)
			econfig.AppendNewlines = false
			encoder.Init(econfig)
			payloadBytes, err := encoder.Encode(pack)

			config.Encoder = "PayloadEncoder"
			config.ContentType = "text/plain"
			oth.MockOutputRunner.EXPECT().Encoder().Return(encoder)
			oth.MockOutputRunner.EXPECT().Encode(pack).Return(payloadBytes, nil)

			err = amqpOutput.Init(config)
			c.Assume(err, gs.IsNil)
			c.Expect(amqpOutput.ch, gs.Equals, mch)

			mch.EXPECT().Publish("", "test", false, false, gomock.Any()).Return(nil)
			inChan <- pack
			close(inChan)
			close(closeChan)

			go func() {
				err := amqpOutput.Run(oth.MockOutputRunner, oth.MockHelper)
				errChan <- err
			}()

			ug.Wait()
			err = <-errChan
			c.Expect(err, gs.IsNil)
		})

		c.Specify("publishes a serialized message", func() {
			encoder := new(ProtobufEncoder)
			encoder.SetPipelineConfig(pConfig)
			encoder.Init(nil)
			protoBytes, err := encoder.Encode(pack)
			c.Expect(err, gs.IsNil)
			oth.MockOutputRunner.EXPECT().Encoder().Return(encoder)
			oth.MockOutputRunner.EXPECT().Encode(pack).Return(protoBytes, nil)

			err = amqpOutput.Init(config)
			c.Assume(err, gs.IsNil)
			c.Expect(amqpOutput.ch, gs.Equals, mch)

			mch.EXPECT().Publish("", "test", false, false, gomock.Any()).Return(nil)
			inChan <- pack
			close(inChan)
			close(closeChan)

			go func() {
				err := amqpOutput.Run(oth.MockOutputRunner, oth.MockHelper)
				errChan <- err
			}()
			ug.Wait()
			err = <-errChan
			c.Expect(err, gs.IsNil)
		})
	})
}
