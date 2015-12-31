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
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"

	"github.com/mozilla-services/heka/message"
	"github.com/mozilla-services/heka/pipeline"
	"github.com/rafrombrc/sarama"
)

type KafkaInputConfig struct {
	Splitter string

	// Client Config
	Id                         string
	Addrs                      []string
	MetadataRetries            int    `toml:"metadata_retries"`
	WaitForElection            uint32 `toml:"wait_for_election"`
	BackgroundRefreshFrequency uint32 `toml:"background_refresh_frequency"`

	// Broker Config
	MaxOpenRequests int    `toml:"max_open_reqests"`
	DialTimeout     uint32 `toml:"dial_timeout"`
	ReadTimeout     uint32 `toml:"read_timeout"`
	WriteTimeout    uint32 `toml:"write_timeout"`

	// Consumer Config
	Topic            string
	Partition        int32
	Group            string
	DefaultFetchSize int32  `toml:"default_fetch_size"`
	MinFetchSize     int32  `toml:"min_fetch_size"`
	MaxMessageSize   int32  `toml:"max_message_size"`
	MaxWaitTime      uint32 `toml:"max_wait_time"`
	OffsetMethod     string `toml:"offset_method"` // Manual, Newest, Oldest
	EventBufferSize  int    `toml:"event_buffer_size"`
}

type KafkaInput struct {
	processMessageCount    int64
	processMessageFailures int64

	config             *KafkaInputConfig
	clientConfig       *sarama.ClientConfig
	consumerConfig     *sarama.ConsumerConfig
	client             *sarama.Client
	consumer           *sarama.Consumer
	pConfig            *pipeline.PipelineConfig
	ir                 pipeline.InputRunner
	checkpointFile     *os.File
	stopChan           chan bool
	name               string
	checkpointFilename string
}

func (k *KafkaInput) ConfigStruct() interface{} {
	hn := k.pConfig.Hostname()
	return &KafkaInputConfig{
		Splitter:                   "NullSplitter",
		Id:                         hn,
		MetadataRetries:            3,
		WaitForElection:            250,
		BackgroundRefreshFrequency: 10 * 60 * 1000,
		MaxOpenRequests:            4,
		DialTimeout:                60 * 1000,
		ReadTimeout:                60 * 1000,
		WriteTimeout:               60 * 1000,
		DefaultFetchSize:           1024 * 32,
		MinFetchSize:               1,
		MaxWaitTime:                250,
		OffsetMethod:               "Manual",
		EventBufferSize:            16,
	}
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	if err == nil {
		return true
	}
	return false
}

func (k *KafkaInput) writeCheckpoint(offset int64) (err error) {
	if k.checkpointFile == nil {
		if k.checkpointFile, err = os.OpenFile(k.checkpointFilename,
			os.O_WRONLY|os.O_SYNC|os.O_CREATE|os.O_TRUNC, 0644); err != nil {
			return
		}
	}
	k.checkpointFile.Seek(0, 0)
	err = binary.Write(k.checkpointFile, binary.LittleEndian, &offset)
	return
}

func readCheckpoint(filename string) (offset int64, err error) {
	file, err := os.Open(filename)
	if err != nil {
		return
	}
	defer file.Close()
	err = binary.Read(file, binary.LittleEndian, &offset)
	return
}

func (k *KafkaInput) SetPipelineConfig(pConfig *pipeline.PipelineConfig) {
	k.pConfig = pConfig
}

func (k *KafkaInput) SetName(name string) {
	k.name = name
}

func (k *KafkaInput) Init(config interface{}) (err error) {
	k.config = config.(*KafkaInputConfig)
	if len(k.config.Addrs) == 0 {
		return errors.New("addrs must have at least one entry")
	}
	if len(k.config.Group) == 0 {
		k.config.Group = k.config.Id
	}

	k.clientConfig = sarama.NewClientConfig()
	k.clientConfig.MetadataRetries = k.config.MetadataRetries
	k.clientConfig.WaitForElection = time.Duration(k.config.WaitForElection) * time.Millisecond
	k.clientConfig.BackgroundRefreshFrequency = time.Duration(k.config.BackgroundRefreshFrequency) * time.Millisecond

	k.clientConfig.DefaultBrokerConf = sarama.NewBrokerConfig()
	k.clientConfig.DefaultBrokerConf.MaxOpenRequests = k.config.MaxOpenRequests
	k.clientConfig.DefaultBrokerConf.DialTimeout = time.Duration(k.config.DialTimeout) * time.Millisecond
	k.clientConfig.DefaultBrokerConf.ReadTimeout = time.Duration(k.config.ReadTimeout) * time.Millisecond
	k.clientConfig.DefaultBrokerConf.WriteTimeout = time.Duration(k.config.WriteTimeout) * time.Millisecond

	k.consumerConfig = sarama.NewConsumerConfig()
	k.consumerConfig.DefaultFetchSize = k.config.DefaultFetchSize
	k.consumerConfig.MinFetchSize = k.config.MinFetchSize
	k.consumerConfig.MaxMessageSize = k.config.MaxMessageSize
	k.consumerConfig.MaxWaitTime = time.Duration(k.config.MaxWaitTime) * time.Millisecond
	k.checkpointFilename = k.pConfig.Globals.PrependBaseDir(filepath.Join("kafka",
		fmt.Sprintf("%s.%s.%d.offset.bin", k.name, k.config.Topic, k.config.Partition)))

	switch k.config.OffsetMethod {
	case "Manual":
		k.consumerConfig.OffsetMethod = sarama.OffsetMethodManual
		if fileExists(k.checkpointFilename) {
			if k.consumerConfig.OffsetValue, err = readCheckpoint(k.checkpointFilename); err != nil {
				return fmt.Errorf("readCheckpoint %s", err)
			}
		} else {
			if err = os.MkdirAll(filepath.Dir(k.checkpointFilename), 0766); err != nil {
				return
			}
			k.consumerConfig.OffsetMethod = sarama.OffsetMethodOldest
		}
	case "Newest":
		k.consumerConfig.OffsetMethod = sarama.OffsetMethodNewest
		if fileExists(k.checkpointFilename) {
			if err = os.Remove(k.checkpointFilename); err != nil {
				return
			}
		}
	case "Oldest":
		k.consumerConfig.OffsetMethod = sarama.OffsetMethodOldest
		if fileExists(k.checkpointFilename) {
			if err = os.Remove(k.checkpointFilename); err != nil {
				return
			}
		}
	default:
		return fmt.Errorf("invalid offset_method: %s", k.config.OffsetMethod)
	}

	k.consumerConfig.EventBufferSize = k.config.EventBufferSize

	k.client, err = sarama.NewClient(k.config.Id, k.config.Addrs, k.clientConfig)
	if err != nil {
		return
	}
	k.consumer, err = sarama.NewConsumer(k.client, k.config.Topic, k.config.Partition, k.config.Group, k.consumerConfig)
	return
}

func (k *KafkaInput) addField(pack *pipeline.PipelinePack, name string,
	value interface{}, representation string) {

	if field, err := message.NewField(name, value, representation); err == nil {
		pack.Message.AddField(field)
	} else {
		k.ir.LogError(fmt.Errorf("can't add '%s' field: %s", name, err.Error()))
	}
}

func (k *KafkaInput) Run(ir pipeline.InputRunner, h pipeline.PluginHelper) (err error) {
	sRunner := ir.NewSplitterRunner("")

	defer func() {
		k.consumer.Close()
		k.client.Close()
		if k.checkpointFile != nil {
			k.checkpointFile.Close()
		}
		sRunner.Done()
	}()
	k.ir = ir
	k.stopChan = make(chan bool)

	var (
		hostname = k.pConfig.Hostname()
		event    *sarama.ConsumerEvent
		ok       bool
		n        int
	)

	packDec := func(pack *pipeline.PipelinePack) {
		pack.Message.SetType("heka.kafka")
		pack.Message.SetLogger(k.name)
		pack.Message.SetHostname(hostname)
		k.addField(pack, "Key", event.Key, "")
		k.addField(pack, "Topic", event.Topic, "")
		k.addField(pack, "Partition", event.Partition, "")
		k.addField(pack, "Offset", event.Offset, "")
	}
	if !sRunner.UseMsgBytes() {
		sRunner.SetPackDecorator(packDec)
	}

	for {
		select {
		case event, ok = <-k.consumer.Events():
			if !ok {
				return
			}
			atomic.AddInt64(&k.processMessageCount, 1)
			if event.Err != nil {
				if event.Err == sarama.OffsetOutOfRange {
					ir.LogError(fmt.Errorf(
						"removing the out of range checkpoint file and stopping"))
					if k.checkpointFile != nil {
						k.checkpointFile.Close()
						k.checkpointFile = nil
					}
					if err := os.Remove(k.checkpointFilename); err != nil {
						ir.LogError(err)
					}
					return
				}
				atomic.AddInt64(&k.processMessageFailures, 1)
				ir.LogError(event.Err)
				break
			}
			if n, err = sRunner.SplitBytes(event.Value, nil); err != nil {
				ir.LogError(fmt.Errorf("processing message from topic %s: %s",
					event.Topic, err))
			}
			if n > 0 && n != len(event.Value) {
				ir.LogError(fmt.Errorf("extra data dropped in message from topic %s",
					event.Topic))
			}

			if k.config.OffsetMethod == "Manual" {
				if err = k.writeCheckpoint(event.Offset + 1); err != nil {
					return
				}
			}

		case <-k.stopChan:
			return
		}
	}
}

func (k *KafkaInput) Stop() {
	close(k.stopChan)
}

func (k *KafkaInput) ReportMsg(msg *message.Message) error {
	message.NewInt64Field(msg, "ProcessMessageCount",
		atomic.LoadInt64(&k.processMessageCount), "count")
	message.NewInt64Field(msg, "ProcessMessageFailures",
		atomic.LoadInt64(&k.processMessageFailures), "count")
	return nil
}

func (k *KafkaInput) CleanupForRestart() {
	return
}

func init() {
	pipeline.RegisterPlugin("KafkaInput", func() interface{} {
		return new(KafkaInput)
	})
}
