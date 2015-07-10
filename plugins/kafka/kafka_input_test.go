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
#   Rob Miller (rmiller@mozilla.com)
#
# ***** END LICENSE BLOCK *****/

package kafka

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	. "github.com/mozilla-services/heka/pipeline"
	"github.com/mozilla-services/heka/pipelinemock"
	plugins_ts "github.com/mozilla-services/heka/plugins/testsupport"
	"github.com/rafrombrc/gomock/gomock"
	"github.com/rafrombrc/sarama"
)

func TestEmptyInputAddress(t *testing.T) {
	pConfig := NewPipelineConfig(nil)
	ki := new(KafkaInput)
	ki.SetPipelineConfig(pConfig)
	config := ki.ConfigStruct().(*KafkaInputConfig)
	err := ki.Init(config)

	errmsg := "addrs must have at least one entry"
	if err.Error() != errmsg {
		t.Errorf("Expected: %s, received: %s", errmsg, err)
	}
}

func TestInvalidOffsetMethod(t *testing.T) {
	pConfig := NewPipelineConfig(nil)
	ki := new(KafkaInput)
	ki.SetName("test")
	ki.SetPipelineConfig(pConfig)

	config := ki.ConfigStruct().(*KafkaInputConfig)
	config.Addrs = append(config.Addrs, "localhost:5432")
	config.OffsetMethod = "last"
	err := ki.Init(config)

	errmsg := "invalid offset_method: last"
	if err.Error() != errmsg {
		t.Errorf("Expected: %s, received: %s", errmsg, err)
	}
}

func TestReceivePayloadMessage(t *testing.T) {
	b1 := sarama.NewMockBroker(t, 1)
	b2 := sarama.NewMockBroker(t, 2)
	ctrl := gomock.NewController(t)
	tmpDir, tmpErr := ioutil.TempDir("", "kafkainput-tests")
	if tmpErr != nil {
		t.Errorf("Unable to create a temporary directory: %s", tmpErr)
	}

	defer func() {
		if err := os.RemoveAll(tmpDir); err != nil {
			t.Errorf("Cleanup failed: %s", err)
		}
		ctrl.Finish()
	}()

	topic := "test"
	mdr := new(sarama.MetadataResponse)
	mdr.AddBroker(b2.Addr(), b2.BrokerID())
	mdr.AddTopicPartition(topic, 0, 2)
	b1.Returns(mdr)

	or := new(sarama.OffsetResponse)
	or.AddTopicPartition(topic, 0, 0)
	b2.Returns(or)

	fr := new(sarama.FetchResponse)
	fr.AddMessage(topic, 0, nil, sarama.ByteEncoder([]byte{0x41, 0x42}), 0)
	b2.Returns(fr)

	pConfig := NewPipelineConfig(nil)
	pConfig.Globals.BaseDir = tmpDir
	ki := new(KafkaInput)
	ki.SetName(topic)
	ki.SetPipelineConfig(pConfig)
	config := ki.ConfigStruct().(*KafkaInputConfig)
	config.Addrs = append(config.Addrs, b1.Addr())
	config.Topic = topic

	ith := new(plugins_ts.InputTestHelper)
	ith.Pack = NewPipelinePack(pConfig.InputRecycleChan())
	ith.MockHelper = pipelinemock.NewMockPluginHelper(ctrl)
	ith.MockInputRunner = pipelinemock.NewMockInputRunner(ctrl)

	ith.MockSplitterRunner = pipelinemock.NewMockSplitterRunner(ctrl)

	err := ki.Init(config)
	if err != nil {
		t.Fatalf("%s", err)
	}

	ith.MockInputRunner.EXPECT().NewSplitterRunner("").Return(ith.MockSplitterRunner)
	ith.MockSplitterRunner.EXPECT().UseMsgBytes().Return(false)
	ith.MockSplitterRunner.EXPECT().Done()

	decChan := make(chan func(*PipelinePack), 1)
	decCall := ith.MockSplitterRunner.EXPECT().SetPackDecorator(gomock.Any())
	decCall.Do(func(dec func(pack *PipelinePack)) {
		decChan <- dec
	})

	bytesChan := make(chan []byte, 1)
	splitCall := ith.MockSplitterRunner.EXPECT().SplitBytes(gomock.Any(), nil)
	splitCall.Do(func(recd []byte, del Deliverer) {
		bytesChan <- recd
	})

	errChan := make(chan error)
	go func() {
		errChan <- ki.Run(ith.MockInputRunner, ith.MockHelper)
	}()

	recd := <-bytesChan
	if string(recd) != "AB" {
		t.Errorf("Invalid Payload Expected: AB received: %s", string(recd))
	}

	packDec := <-decChan
	packDec(ith.Pack)
	if ith.Pack.Message.GetType() != "heka.kafka" {
		t.Errorf("Invalid Type %s", ith.Pack.Message.GetType())
	}

	// There is a hang on the consumer close with the mock broker
	// closing the brokers before the consumer works around the issue
	// and is good enough for this test.
	b1.Close()
	b2.Close()

	ki.Stop()
	err = <-errChan
	if err != nil {
		t.Fatal(err)
	}

	filename := filepath.Join(tmpDir, "kafka", "test.test.0.offset.bin")
	if o, err := readCheckpoint(filename); err != nil {
		t.Errorf("Could not read the checkpoint file: %s", filename)
	} else {
		if o != 1 {
			t.Errorf("Incorrect offset Expected: 1 Received: %d", o)
		}
	}
}

func TestReceiveProtobufMessage(t *testing.T) {
	b1 := sarama.NewMockBroker(t, 1)
	b2 := sarama.NewMockBroker(t, 2)
	ctrl := gomock.NewController(t)
	tmpDir, tmpErr := ioutil.TempDir("", "kafkainput-tests")
	if tmpErr != nil {
		t.Errorf("Unable to create a temporary directory: %s", tmpErr)
	}

	defer func() {
		if err := os.RemoveAll(tmpDir); err != nil {
			t.Errorf("Cleanup failed: %s", err)
		}
		ctrl.Finish()
	}()

	topic := "test"
	mdr := new(sarama.MetadataResponse)
	mdr.AddBroker(b2.Addr(), b2.BrokerID())
	mdr.AddTopicPartition(topic, 0, 2)
	b1.Returns(mdr)

	or := new(sarama.OffsetResponse)
	or.AddTopicPartition(topic, 0, 0)
	b2.Returns(or)

	fr := new(sarama.FetchResponse)
	fr.AddMessage(topic, 0, nil, sarama.ByteEncoder([]byte{0x41, 0x42}), 0)
	b2.Returns(fr)

	pConfig := NewPipelineConfig(nil)
	pConfig.Globals.BaseDir = tmpDir
	ki := new(KafkaInput)
	ki.SetName(topic)
	ki.SetPipelineConfig(pConfig)
	config := ki.ConfigStruct().(*KafkaInputConfig)
	config.Addrs = append(config.Addrs, b1.Addr())
	config.Topic = topic

	ith := new(plugins_ts.InputTestHelper)
	ith.Pack = NewPipelinePack(pConfig.InputRecycleChan())
	ith.MockHelper = pipelinemock.NewMockPluginHelper(ctrl)
	ith.MockInputRunner = pipelinemock.NewMockInputRunner(ctrl)

	ith.MockSplitterRunner = pipelinemock.NewMockSplitterRunner(ctrl)

	err := ki.Init(config)
	if err != nil {
		t.Fatalf("%s", err)
	}

	ith.MockInputRunner.EXPECT().NewSplitterRunner("").Return(ith.MockSplitterRunner)
	ith.MockSplitterRunner.EXPECT().UseMsgBytes().Return(true)
	ith.MockSplitterRunner.EXPECT().Done()

	bytesChan := make(chan []byte, 1)
	splitCall := ith.MockSplitterRunner.EXPECT().SplitBytes(gomock.Any(), nil)
	splitCall.Do(func(recd []byte, del Deliverer) {
		bytesChan <- recd
	})

	errChan := make(chan error)
	go func() {
		errChan <- ki.Run(ith.MockInputRunner, ith.MockHelper)
	}()

	recd := <-bytesChan
	if string(recd) != "AB" {
		t.Errorf("Invalid MsgBytes Expected: AB received: %s", string(recd))
	}

	// There is a hang on the consumer close with the mock broker
	// closing the brokers before the consumer works around the issue
	// and is good enough for this test.
	b1.Close()
	b2.Close()

	ki.Stop()
	err = <-errChan
	if err != nil {
		t.Fatal(err)
	}
}
