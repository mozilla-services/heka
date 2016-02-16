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
#   Matt Moyer (moyer@simple.com)
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

	"github.com/Shopify/sarama"
	"github.com/mozilla-services/heka/message"
	"github.com/mozilla-services/heka/pipeline"
	"github.com/mozilla-services/heka/plugins/tcp"
)

type KafkaInputConfig struct {
	Splitter string

	// Client Config
	Id                         string
	Addrs                      []string
	MetadataRetries            int    `toml:"metadata_retries"`
	WaitForElection            uint32 `toml:"wait_for_election"`
	BackgroundRefreshFrequency uint32 `toml:"background_refresh_frequency"`

	// TLS Config
	UseTls bool `toml:"use_tls"`
	Tls    tcp.TlsConfig

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
	saramaConfig       *sarama.Config
	consumer           sarama.Consumer
	partitionConsumer  sarama.PartitionConsumer
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

	k.saramaConfig = sarama.NewConfig()
	k.saramaConfig.ClientID = k.config.Id
	k.saramaConfig.Metadata.Retry.Max = k.config.MetadataRetries
	k.saramaConfig.Metadata.Retry.Backoff = time.Duration(k.config.WaitForElection) * time.Millisecond
	k.saramaConfig.Metadata.RefreshFrequency = time.Duration(k.config.BackgroundRefreshFrequency) * time.Millisecond

	k.saramaConfig.Net.TLS.Enable = k.config.UseTls
	if k.config.UseTls {
		if k.saramaConfig.Net.TLS.Config, err = tcp.CreateGoTlsConfig(&k.config.Tls); err != nil {
			return fmt.Errorf("TLS init error: %s", err)
		}
	}

	k.saramaConfig.Net.MaxOpenRequests = k.config.MaxOpenRequests
	k.saramaConfig.Net.DialTimeout = time.Duration(k.config.DialTimeout) * time.Millisecond
	k.saramaConfig.Net.ReadTimeout = time.Duration(k.config.ReadTimeout) * time.Millisecond
	k.saramaConfig.Net.WriteTimeout = time.Duration(k.config.WriteTimeout) * time.Millisecond

	k.saramaConfig.Consumer.Fetch.Default = k.config.DefaultFetchSize
	k.saramaConfig.Consumer.Fetch.Min = k.config.MinFetchSize
	k.saramaConfig.Consumer.Fetch.Max = k.config.MaxMessageSize
	k.saramaConfig.Consumer.MaxWaitTime = time.Duration(k.config.MaxWaitTime) * time.Millisecond
	k.checkpointFilename = k.pConfig.Globals.PrependBaseDir(filepath.Join("kafka",
		fmt.Sprintf("%s.%s.%d.offset.bin", k.name, k.config.Topic, k.config.Partition)))

	var offset int64
	switch k.config.OffsetMethod {
	case "Manual":
		if fileExists(k.checkpointFilename) {
			if offset, err = readCheckpoint(k.checkpointFilename); err != nil {
				return fmt.Errorf("readCheckpoint %s", err)
			}
		} else {
			if err = os.MkdirAll(filepath.Dir(k.checkpointFilename), 0766); err != nil {
				return err
			}
			offset = sarama.OffsetOldest
		}
	case "Newest":
		offset = sarama.OffsetNewest
		if fileExists(k.checkpointFilename) {
			if err = os.Remove(k.checkpointFilename); err != nil {
				return err
			}
		}
	case "Oldest":
		offset = sarama.OffsetOldest
		if fileExists(k.checkpointFilename) {
			if err = os.Remove(k.checkpointFilename); err != nil {
				return err
			}
		}
	default:
		return fmt.Errorf("invalid offset_method: %s", k.config.OffsetMethod)
	}

	k.saramaConfig.ChannelBufferSize = k.config.EventBufferSize

	k.consumer, err = sarama.NewConsumer(k.config.Addrs, k.saramaConfig)
	if err != nil {
		return err
	}
	k.partitionConsumer, err = k.consumer.ConsumePartition(k.config.Topic, k.config.Partition, offset)
	return err
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
		k.partitionConsumer.Close()
		k.consumer.Close()
		if k.checkpointFile != nil {
			k.checkpointFile.Close()
		}
		sRunner.Done()
	}()
	k.ir = ir
	k.stopChan = make(chan bool)

	var (
		hostname = k.pConfig.Hostname()
		event    *sarama.ConsumerMessage
		cError   *sarama.ConsumerError
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

	eventChan := k.partitionConsumer.Messages()
	cErrChan := k.partitionConsumer.Errors()
	for {
		select {
		case event, ok = <-eventChan:
			if !ok {
				return nil
			}
			atomic.AddInt64(&k.processMessageCount, 1)
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
					return err
				}
			}

		case cError, ok = <-cErrChan:
			if !ok {
				// Don't exit until the eventChan is closed.
				ok = true
				continue
			}
			if cError.Err == sarama.ErrOffsetOutOfRange {
				ir.LogError(fmt.Errorf(
					"removing the out of range checkpoint file and stopping"))
				if k.checkpointFile != nil {
					k.checkpointFile.Close()
					k.checkpointFile = nil
				}
				if err := os.Remove(k.checkpointFilename); err != nil {
					ir.LogError(err)
				}
				return err
			}
			atomic.AddInt64(&k.processMessageFailures, 1)
			ir.LogError(cError.Err)

		case <-k.stopChan:
			return nil
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
