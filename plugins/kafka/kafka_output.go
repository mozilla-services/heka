/***** BEGIN LICENSE BLOCK *****
# This Source Code Form is subject to the terms of the Mozilla Public
# License, v. 2.0. If a copy of the MPL was not distributed with this file,
# You can obtain one at http://mozilla.org/MPL/2.0/.
#
# The Initial Developer of the Original Code is the Mozilla Foundation.
# Portions created by the Initial Developer are Copyright (C) 2014
# the Initial Developer. All Rights Reserved.
#
# Contributor(s):
#   Mike Trinkala (trink@mozilla.com)
#
# ***** END LICENSE BLOCK *****/

package kafka

import (
	"errors"
	"fmt"
	"github.com/Shopify/sarama"
	"github.com/mozilla-services/heka/message"
	"github.com/mozilla-services/heka/pipeline"
	"os"
	"regexp"
	"strconv"
	"sync/atomic"
	"time"
)

type KafkaOutputConfig struct {
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

	// Producer Config
	Partitioner string // Random, RoundRobin, Hash
	// Hash/Topic variables are restricted to the Type", "Logger", "Hostname" and "Payload" headers.
	// Field variables are unrestricted.
	HashVariable  string `toml:"hash_variable"`  // HashPartitioner key is extracted from a message variable
	TopicVariable string `toml:"topic_variable"` // Topic extracted from a message variable
	Topic         string // Static topic

	RequiredAcks               string `toml:"required_acks"` // NoResponse, WaitForLocal, WaitForAll
	Timeout                    uint32
	CompressionCodec           string `toml:"compression_codec"` // None, GZIP, Snappy
	MaxBufferTime              uint32 `toml:"max_buffer_time"`
	MaxBufferedBytes           uint32 `toml:"max_buffered_bytes"`
	BackPressureThresholdBytes uint32 `toml:"back_pressure_threshold_bytes"`
}

var fieldRegex = regexp.MustCompile("^Fields\\[([^\\]]*)\\](?:\\[(\\d+)\\])?(?:\\[(\\d+)\\])?$")

type messageVariable struct {
	header bool
	name   string
	fi     int
	ai     int
}

type KafkaOutput struct {
	processMessageCount    int64
	processMessageFailures int64

	hashVariable  *messageVariable
	topicVariable *messageVariable
	config        *KafkaOutputConfig
	cconfig       *sarama.ClientConfig
	pconfig       *sarama.ProducerConfig
	client        *sarama.Client
	producer      *sarama.Producer
}

func (k *KafkaOutput) ConfigStruct() interface{} {
	hn, _ := os.Hostname()
	return &KafkaOutputConfig{
		Id:                         hn,
		MetadataRetries:            3,
		WaitForElection:            250,
		BackgroundRefreshFrequency: 10 * 60 * 1000,
		MaxOpenRequests:            4,
		DialTimeout:                60 * 1000,
		ReadTimeout:                60 * 1000,
		WriteTimeout:               60 * 1000,
		Partitioner:                "Random",
		RequiredAcks:               "WaitForLocal",
		CompressionCodec:           "None",
		MaxBufferTime:              1,
		MaxBufferedBytes:           1,
		BackPressureThresholdBytes: 50 * 1024 * 1024,
	}
}

func verifyMessageVariable(key string) *messageVariable {
	switch key {
	case "Type", "Logger", "Hostname", "Payload":
		return &messageVariable{header: true, name: key}
	default:
		matches := fieldRegex.FindStringSubmatch(key)
		if len(matches) == 4 {
			mvar := &messageVariable{header: false, name: matches[1]}
			if len(matches[2]) > 0 {
				if parsedInt, err := strconv.ParseInt(matches[2], 10, 32); err == nil {
					mvar.fi = int(parsedInt)
				} else {
					return nil
				}
			}
			if len(matches[3]) > 0 {
				if parsedInt, err := strconv.ParseInt(matches[3], 10, 32); err == nil {
					mvar.ai = int(parsedInt)
				} else {
					return nil
				}
			}
			return mvar
		}
		return nil
	}
}

func getFieldAsString(msg *message.Message, mvar *messageVariable) string {
	var field *message.Field
	if mvar.fi != 0 {
		fields := msg.FindAllFields(mvar.name)
		if mvar.fi >= len(fields) {
			return ""
		}
		field = fields[mvar.fi]
	} else {
		if field = msg.FindFirstField(mvar.name); field == nil {
			return ""
		}
	}
	switch field.GetValueType() {
	case message.Field_STRING:
		if mvar.ai >= len(field.ValueString) {
			return ""
		}
		return field.ValueString[mvar.ai]
	case message.Field_BYTES:
		if mvar.ai >= len(field.ValueBytes) {
			return ""
		}
		return string(field.ValueBytes[mvar.ai])
	case message.Field_INTEGER:
		if mvar.ai >= len(field.ValueInteger) {
			return ""
		}
		return fmt.Sprintf("%d", field.ValueInteger[mvar.ai])
	case message.Field_DOUBLE:
		if mvar.ai >= len(field.ValueDouble) {
			return ""
		}
		return fmt.Sprintf("%g", field.ValueDouble[mvar.ai])
	case message.Field_BOOL:
		if mvar.ai >= len(field.ValueBool) {
			return ""
		}
		return fmt.Sprintf("%t", field.ValueBool[mvar.ai])
	}
	return ""
}

func getMessageVariable(msg *message.Message, mvar *messageVariable) string {
	if mvar.header {
		switch mvar.name {
		case "Type":
			return msg.GetType()
		case "Logger":
			return msg.GetLogger()
		case "Hostname":
			return msg.GetHostname()
		case "Payload":
			return msg.GetPayload()
		default:
			return ""
		}
	} else {
		return getFieldAsString(msg, mvar)
	}
}

func (k *KafkaOutput) Init(config interface{}) (err error) {
	k.config = config.(*KafkaOutputConfig)
	if len(k.config.Addrs) == 0 {
		return errors.New("addrs must have at least one entry")
	}

	k.cconfig = sarama.NewClientConfig()
	k.cconfig.MetadataRetries = k.config.MetadataRetries
	k.cconfig.WaitForElection = time.Duration(k.config.WaitForElection) * time.Millisecond
	k.cconfig.BackgroundRefreshFrequency = time.Duration(k.config.BackgroundRefreshFrequency) * time.Millisecond

	k.cconfig.DefaultBrokerConf = sarama.NewBrokerConfig()
	k.cconfig.DefaultBrokerConf.MaxOpenRequests = k.config.MaxOpenRequests
	k.cconfig.DefaultBrokerConf.DialTimeout = time.Duration(k.config.DialTimeout) * time.Millisecond
	k.cconfig.DefaultBrokerConf.ReadTimeout = time.Duration(k.config.ReadTimeout) * time.Millisecond
	k.cconfig.DefaultBrokerConf.WriteTimeout = time.Duration(k.config.WriteTimeout) * time.Millisecond

	k.pconfig = sarama.NewProducerConfig()

	switch k.config.Partitioner {
	case "Random":
		k.pconfig.Partitioner = sarama.NewRandomPartitioner()
		if len(k.config.HashVariable) > 0 {
			return fmt.Errorf("hash_variable should not be set for the %s partitioner", k.config.Partitioner)
		}
	case "RoundRobin":
		k.pconfig.Partitioner = new(sarama.RoundRobinPartitioner)
		if len(k.config.HashVariable) > 0 {
			return fmt.Errorf("hash_variable should not be set for the %s partitioner", k.config.Partitioner)
		}
	case "Hash":
		k.pconfig.Partitioner = sarama.NewHashPartitioner()
		if k.hashVariable = verifyMessageVariable(k.config.HashVariable); k.hashVariable == nil {
			return fmt.Errorf("invalid hash_variable: %s", k.config.HashVariable)
		}
	default:
		return fmt.Errorf("invalid partitioner: %s", k.config.Partitioner)
	}

	if len(k.config.Topic) == 0 {
		if k.topicVariable = verifyMessageVariable(k.config.TopicVariable); k.topicVariable == nil {
			return fmt.Errorf("invalid topic_variable: %s", k.config.TopicVariable)
		}
	} else if len(k.config.TopicVariable) > 0 {
		return errors.New("topic and topic_variable cannot both be set")
	}

	switch k.config.RequiredAcks {
	case "NoResponse":
		k.pconfig.RequiredAcks = sarama.NoResponse
	case "WaitForLocal":
		k.pconfig.RequiredAcks = sarama.WaitForLocal
	case "WaitForAll":
		k.pconfig.RequiredAcks = sarama.WaitForAll
	default:
		return fmt.Errorf("invalid required_acks: %s", k.config.RequiredAcks)
	}

	k.pconfig.Timeout = time.Duration(k.config.Timeout) * time.Millisecond

	switch k.config.CompressionCodec {
	case "None":
		k.pconfig.Compression = sarama.CompressionNone
	case "GZIP":
		k.pconfig.Compression = sarama.CompressionGZIP
	case "Snappy":
		k.pconfig.Compression = sarama.CompressionSnappy
	default:
		return fmt.Errorf("invalid compression_codec: %s", k.config.CompressionCodec)
	}

	k.pconfig.MaxBufferedBytes = k.config.MaxBufferedBytes
	k.pconfig.MaxBufferTime = time.Duration(k.config.MaxBufferTime) * time.Millisecond
	k.pconfig.BackPressureThresholdBytes = k.config.BackPressureThresholdBytes

	k.client, err = sarama.NewClient(k.config.Id, k.config.Addrs, k.cconfig)
	if err != nil {
		return
	}
	k.producer, err = sarama.NewProducer(k.client, k.pconfig)
	return
}

func (k *KafkaOutput) Run(or pipeline.OutputRunner, h pipeline.PluginHelper) (err error) {
	defer func() {
		k.producer.Close()
		k.client.Close()
	}()

	if or.Encoder() == nil {
		return errors.New("Encoder required.")
	}

	inChan := or.InChan()
	errChan := k.producer.Errors()

	var (
		ok    = true
		pack  *pipeline.PipelinePack
		topic = k.config.Topic
		key   sarama.Encoder
	)

	for ok {
		select {
		case pack, ok = <-inChan:
			if !ok {
				break
			}
			atomic.AddInt64(&k.processMessageCount, 1)

			if k.topicVariable != nil {
				topic = getMessageVariable(pack.Message, k.topicVariable)
			}
			if k.hashVariable != nil {
				key = sarama.StringEncoder(getMessageVariable(pack.Message, k.hashVariable))
			}

			if msgBytes, err := or.Encode(pack); msgBytes == nil || err == nil {
				err = k.producer.QueueMessage(topic, key, sarama.ByteEncoder(msgBytes))
				if err != nil {
					atomic.AddInt64(&k.processMessageFailures, 1)
					or.LogError(err)
				}
			} else {
				atomic.AddInt64(&k.processMessageFailures, 1)
				or.LogError(err)
			}
			pack.Recycle()
			break

		case err = <-errChan:
			break
		}
	}
	return
}

func (k *KafkaOutput) ReportMsg(msg *message.Message) error {
	message.NewInt64Field(msg, "ProcessMessageCount",
		atomic.LoadInt64(&k.processMessageCount), "count")
	message.NewInt64Field(msg, "ProcessMessageFailures",
		atomic.LoadInt64(&k.processMessageFailures), "count")
	return nil
}

func (k *KafkaOutput) CleanupForRestart() {
	return
}

func init() {
	pipeline.RegisterPlugin("KafkaOutput", func() interface{} {
		return new(KafkaOutput)
	})
}
