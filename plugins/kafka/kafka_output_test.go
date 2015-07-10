/***** BEGIN LICENSE BLOCK *****
# This Source Code Form is subject to the terms of the Mozilla Public
# License, v. 2.0. If a copy of the MPL was not distributed with this file,
# You can obtain one at http://mozilla.org/MPL/2.0/.
#
# The Initial Developer of the Original Code is the Mozilla Foundation.
# Portions created by the Initial Developer are Copyright (C) 2014-2015
# the Initial Developer. All Rights Reserved.
#
# Contributor(s):
#   Mike Trinkala (trink@mozilla.com)
#
# ***** END LICENSE BLOCK *****/

package kafka

import (
	"sync/atomic"
	"testing"

	"github.com/mozilla-services/heka/message"
	. "github.com/mozilla-services/heka/pipeline"
	pipeline_ts "github.com/mozilla-services/heka/pipeline/testsupport"
	"github.com/mozilla-services/heka/plugins"
	plugins_ts "github.com/mozilla-services/heka/plugins/testsupport"
	"github.com/rafrombrc/gomock/gomock"
	"github.com/rafrombrc/sarama"
)

func TestVerifyMessageInvalidVariables(t *testing.T) {
	tests := []string{
		"Timestamp",
		"Uuid",
		"Severity",
		"EnvVersion",
		"Fields",
		"Field[foo]",
		"Fields[foo] ",
		"Fields[foo][a]",
		"Fields[foo][1][b]",
	}

	for _, v := range tests {
		mvar := verifyMessageVariable(v)
		if mvar != nil {
			t.Errorf("verification passed %s", v)
			return
		}
	}
}

func TestGetMessageVariable(t *testing.T) {
	field, _ := message.NewField("foo", "bar", "")
	msg := &message.Message{}
	msg.SetType("TEST")
	msg.SetLogger("GoSpec")
	msg.SetHostname("example.com")
	msg.SetPayload("xxx yyy")
	msg.AddField(field)
	data := []byte("data")
	field1, _ := message.NewField("bytes", data, "")
	field2, _ := message.NewField("int", int64(999), "")
	field2.AddValue(int64(1024))
	field3, _ := message.NewField("double", float64(99.9), "")
	field4, _ := message.NewField("bool", true, "")
	field5, _ := message.NewField("foo", "alternate", "")
	msg.AddField(field1)
	msg.AddField(field2)
	msg.AddField(field3)
	msg.AddField(field4)
	msg.AddField(field5)

	tests := []string{
		"Type",
		"Logger",
		"Hostname",
		"Payload",
		"Fields[foo]",
		"Fields[bytes]",
		"Fields[int]",
		"Fields[double]",
		"Fields[bool]",
		"Fields[foo][1]",
		"Fields[int][0][1]",
	}
	results := []string{
		"TEST",
		"GoSpec",
		"example.com",
		"xxx yyy",
		"bar",
		"data",
		"999",
		"99.9",
		"true",
		"alternate",
		"1024",
	}

	for i, v := range tests {
		mvar := verifyMessageVariable(v)
		if mvar == nil {
			t.Errorf("verification failed %s", v)
			return
		}
		s := getMessageVariable(msg, mvar)
		if s != results[i] {
			t.Errorf("%s Expected: %s Received: %s", v, results[i], s)
		}
	}
}

func TestEmptyAddress(t *testing.T) {
	pConfig := NewPipelineConfig(nil)
	ko := new(KafkaOutput)
	ko.SetPipelineConfig(pConfig)
	config := ko.ConfigStruct().(*KafkaOutputConfig)
	err := ko.Init(config)

	errmsg := "addrs must have at least one entry"
	if err.Error() != errmsg {
		t.Errorf("Expected: %s, received: %s", errmsg, err)
	}
}

func TestInvalidPartitioner(t *testing.T) {
	pConfig := NewPipelineConfig(nil)
	ko := new(KafkaOutput)
	ko.SetPipelineConfig(pConfig)
	config := ko.ConfigStruct().(*KafkaOutputConfig)
	config.Addrs = append(config.Addrs, "localhost:5432")
	config.Partitioner = "widget"
	err := ko.Init(config)

	errmsg := "invalid partitioner: widget"
	if err.Error() != errmsg {
		t.Errorf("Expected: %s, received: %s", errmsg, err)
	}
}

func TestRandomPartitionerWithHash(t *testing.T) {
	pConfig := NewPipelineConfig(nil)
	ko := new(KafkaOutput)
	ko.SetPipelineConfig(pConfig)
	config := ko.ConfigStruct().(*KafkaOutputConfig)
	config.Addrs = append(config.Addrs, "localhost:5432")
	config.Topic = "test"
	config.Partitioner = "Random"
	config.HashVariable = "Type"
	err := ko.Init(config)

	errmsg := "hash_variable should not be set for the Random partitioner"
	if err.Error() != errmsg {
		t.Errorf("Expected: %s, received: %s", errmsg, err)
	}
}

func TestHashPartitionerWithInvalidHashVariable(t *testing.T) {
	pConfig := NewPipelineConfig(nil)
	ko := new(KafkaOutput)
	ko.SetPipelineConfig(pConfig)
	config := ko.ConfigStruct().(*KafkaOutputConfig)
	config.Addrs = append(config.Addrs, "localhost:5432")
	config.Topic = "test"
	config.Partitioner = "Hash"
	config.HashVariable = "bogus"
	err := ko.Init(config)

	errmsg := "invalid hash_variable: bogus"
	if err.Error() != errmsg {
		t.Errorf("Expected: %s, received: %s", errmsg, err)
	}
}

func TestRoundRobinPartitionerWithHash(t *testing.T) {
	pConfig := NewPipelineConfig(nil)
	ko := new(KafkaOutput)
	ko.SetPipelineConfig(pConfig)
	config := ko.ConfigStruct().(*KafkaOutputConfig)
	config.Addrs = append(config.Addrs, "localhost:5432")
	config.Topic = "test"
	config.Partitioner = "RoundRobin"
	config.HashVariable = "Type"
	err := ko.Init(config)

	errmsg := "hash_variable should not be set for the RoundRobin partitioner"
	if err.Error() != errmsg {
		t.Errorf("Expected: %s, received: %s", errmsg, err)
	}
}

func TestNoTopic(t *testing.T) {
	pConfig := NewPipelineConfig(nil)
	ko := new(KafkaOutput)
	ko.SetPipelineConfig(pConfig)
	config := ko.ConfigStruct().(*KafkaOutputConfig)
	config.Addrs = append(config.Addrs, "localhost:5432")
	err := ko.Init(config)

	errmsg := "invalid topic_variable: "
	if err.Error() != errmsg {
		t.Errorf("Expected: %s, received: %s", errmsg, err)
	}
}

func TestInvalidTopicVariable(t *testing.T) {
	pConfig := NewPipelineConfig(nil)
	ko := new(KafkaOutput)
	ko.SetPipelineConfig(pConfig)
	config := ko.ConfigStruct().(*KafkaOutputConfig)
	config.Addrs = append(config.Addrs, "localhost:5432")
	config.TopicVariable = "bogus"
	err := ko.Init(config)

	errmsg := "invalid topic_variable: bogus"
	if err.Error() != errmsg {
		t.Errorf("Expected: %s, received: %s", errmsg, err)
	}
}

func TestConflictingTopic(t *testing.T) {
	pConfig := NewPipelineConfig(nil)
	ko := new(KafkaOutput)
	ko.SetPipelineConfig(pConfig)
	config := ko.ConfigStruct().(*KafkaOutputConfig)
	config.Addrs = append(config.Addrs, "localhost:5432")
	config.Topic = "test"
	config.TopicVariable = "Type"
	err := ko.Init(config)

	errmsg := "topic and topic_variable cannot both be set"
	if err.Error() != errmsg {
		t.Errorf("Expected: %s, received: %s", errmsg, err)
	}
}

func TestInvalidRequiredAcks(t *testing.T) {
	pConfig := NewPipelineConfig(nil)
	ko := new(KafkaOutput)
	ko.SetPipelineConfig(pConfig)
	config := ko.ConfigStruct().(*KafkaOutputConfig)
	config.Addrs = append(config.Addrs, "localhost:5432")
	config.Topic = "test"
	config.RequiredAcks = "whenever"
	err := ko.Init(config)

	errmsg := "invalid required_acks: whenever"
	if err.Error() != errmsg {
		t.Errorf("Expected: %s, received: %s", errmsg, err)
	}
}

func TestInvalidCompressionCodec(t *testing.T) {
	pConfig := NewPipelineConfig(nil)
	ko := new(KafkaOutput)
	ko.SetPipelineConfig(pConfig)
	config := ko.ConfigStruct().(*KafkaOutputConfig)
	config.Addrs = append(config.Addrs, "localhost:5432")
	config.Topic = "test"
	config.CompressionCodec = "squash"
	err := ko.Init(config)

	errmsg := "invalid compression_codec: squash"
	if err.Error() != errmsg {
		t.Errorf("Expected: %s, received: %s", errmsg, err)
	}
}

func TestSendMessage(t *testing.T) {
	ctrl := gomock.NewController(t)
	b1 := sarama.NewMockBroker(t, 1)
	b2 := sarama.NewMockBroker(t, 2)

	defer func() {
		b1.Close()
		b2.Close()
		ctrl.Finish()
	}()

	topic := "test"
	globals := DefaultGlobals()
	pConfig := NewPipelineConfig(globals)

	mdr := new(sarama.MetadataResponse)
	mdr.AddBroker(b2.Addr(), b2.BrokerID())
	mdr.AddTopicPartition(topic, 0, 2)
	b1.Returns(mdr)

	pr := new(sarama.ProduceResponse)
	pr.AddTopicPartition(topic, 0, sarama.NoError)
	b2.Returns(pr)

	ko := new(KafkaOutput)
	ko.SetPipelineConfig(pConfig)
	config := ko.ConfigStruct().(*KafkaOutputConfig)
	config.Addrs = append(config.Addrs, b1.Addr())
	config.Topic = topic
	err := ko.Init(config)
	if err != nil {
		t.Fatal(err)
	}
	oth := plugins_ts.NewOutputTestHelper(ctrl)
	encoder := new(plugins.PayloadEncoder)
	encoder.Init(encoder.ConfigStruct().(*plugins.PayloadEncoderConfig))

	inChan := make(chan *PipelinePack, 1)

	msg := pipeline_ts.GetTestMessage()
	pack := NewPipelinePack(pConfig.InputRecycleChan())
	pack.Message = msg

	outStr := "Write me out to the network"
	newpack := NewPipelinePack(nil)
	newpack.Message = msg

	inChanCall := oth.MockOutputRunner.EXPECT().InChan().AnyTimes()
	inChanCall.Return(inChan)
	oth.MockOutputRunner.EXPECT().UsesBuffering().Return(false)

	errChan := make(chan error)
	startOutput := func() {
		go func() {
			err := ko.Run(oth.MockOutputRunner, oth.MockHelper)
			errChan <- err
		}()
	}

	oth.MockOutputRunner.EXPECT().Encoder().Return(encoder)
	oth.MockOutputRunner.EXPECT().Encode(pack).Return(encoder.Encode(pack))

	pack.Message.SetPayload(outStr)
	startOutput()

	msgcount := atomic.LoadInt64(&ko.processMessageCount)
	if msgcount != 0 {
		t.Errorf("Invalid starting processMessageCount %d", msgcount)
	}
	msgcount = atomic.LoadInt64(&ko.processMessageFailures)
	if msgcount != 0 {
		t.Errorf("Invalid starting processMessageFailures %d", msgcount)
	}

	inChan <- pack
	close(inChan)
	err = <-errChan
	if err != nil {
		t.Errorf("Error running output %s", err)
	}

	msgcount = atomic.LoadInt64(&ko.processMessageCount)
	if msgcount != 1 {
		t.Errorf("Invalid ending processMessageCount %d", msgcount)
	}
	msgcount = atomic.LoadInt64(&ko.processMessageFailures)
	if msgcount != 0 {
		t.Errorf("Invalid ending processMessageFailures %d", msgcount)
	}
}
