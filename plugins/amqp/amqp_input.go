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
	"crypto/tls"
	"fmt"
	"strings"
	"sync"

	. "github.com/mozilla-services/heka/pipeline"
	"github.com/mozilla-services/heka/plugins/tcp"
	"github.com/streadway/amqp"
)

// AMQP Input config struct
type AMQPInputConfig struct {
	// AMQP URL. Spec: http://www.rabbitmq.com/uri-spec.html
	// Ex: amqp://USERNAME:PASSWORD@HOSTNAME:PORT/
	URL string
	// Exchange name
	Exchange string
	// Type of exchange, options are: fanout, direct, topic, headers
	ExchangeType string `toml:"exchange_type"`
	// Whether the exchange should be durable or not
	// Defaults to non-durable
	ExchangeDurability bool `toml:"exchange_durability"`
	// Whether the exchange is deleted when all queues have finished
	// Defaults to auto-delete
	ExchangeAutoDelete bool `toml:"exchange_auto_delete"`
	// Routing key for the message to send, or when used for consumer
	// the routing key to bind the queue to the exchange with
	// Defaults to empty string
	RoutingKey string `toml:"routing_key"`
	// How many messages should be pre-fetched before message acks
	// See http://www.rabbitmq.com/blog/2012/04/25/rabbitmq-performance-measurements-part-2/
	// for benchmarks showing the impact of low prefetch counts
	// Defaults to 2
	PrefetchCount int `toml:"prefetch_count"`
	// Name of the queue to consume from, an empty string will have the
	// broker generate a name
	Queue string
	// Whether the queue is durable or not
	// Defaults to non-durable
	QueueDurability bool `toml:"queue_durability"`
	// Whether the queue is exclusive or not
	// Defaults to non-exclusive
	QueueExclusive bool `toml:"queue_exclusive"`
	// Whether the queue is deleted when the last consumer un-subscribes
	// Defaults to auto-delete
	QueueAutoDelete bool `toml:"queue_auto_delete"`
	// How long a message published to a queue can live before it is discarded (milliseconds).
	// 0 is a valid ttl which mimics "immediate" expiration.
	// Default value is -1 which leaves it undefined.
	QueueTTL int32 `toml:"queue_ttl"`
	// Optional subsection for TLS configuration of AMQPS connections. If
	// unspecified, the default AMQPS settings will be used.
	Tls tcp.TlsConfig
	// Specify whether the user is read-only. The exchange and queue must
	// already exist. Defaults to false.
	ReadOnly bool `toml:"read_only"`
}

type AMQPInput struct {
	config  *AMQPInputConfig
	ch      AMQPChannel
	usageWg *sync.WaitGroup
	connWg  *sync.WaitGroup
	amqpHub AMQPConnectionHub
}

func (ai *AMQPInput) ConfigStruct() interface{} {
	return &AMQPInputConfig{
		ExchangeDurability: false,
		ExchangeAutoDelete: true,
		RoutingKey:         "",
		PrefetchCount:      2,
		Queue:              "",
		QueueDurability:    false,
		QueueExclusive:     false,
		QueueAutoDelete:    true,
		QueueTTL:           -1,
		ReadOnly:           false,
	}
}

func (ai *AMQPInput) Init(config interface{}) (err error) {
	conf := config.(*AMQPInputConfig)
	ai.config = conf
	var tlsConf *tls.Config = nil
	if strings.HasPrefix(conf.URL, "amqps://") && &ai.config.Tls != nil {
		if tlsConf, err = tcp.CreateGoTlsConfig(&ai.config.Tls); err != nil {
			return fmt.Errorf("TLS init error: %s", err)
		}
	}

	if ai.amqpHub == nil {
		ai.amqpHub = GetAmqpHub()
	}
	var dialer = NewAMQPDialer(tlsConf)
	ch, usageWg, connWg, err := ai.amqpHub.GetChannel(conf.URL, dialer)
	if err != nil {
		return
	}

	var args amqp.Table

	ttl := conf.QueueTTL

	if ttl != -1 {
		args = amqp.Table{"x-message-ttl": int32(ttl)}
	}

	defer func() {
		if err != nil {
			usageWg.Done()
		}
	}()
	ai.connWg = connWg
	ai.usageWg = usageWg

	// Only declare the exchange and queue if the user is not read only.
	if !conf.ReadOnly {
		err = ch.ExchangeDeclare(conf.Exchange, conf.ExchangeType,
			conf.ExchangeDurability, conf.ExchangeAutoDelete, false, false,
			nil)
		if err != nil {
			return
		}

		_, err = ch.QueueDeclare(conf.Queue, conf.QueueDurability,
			conf.QueueAutoDelete, conf.QueueExclusive, false, args)
		if err != nil {
			return
		}

		err = ch.QueueBind(conf.Queue, conf.RoutingKey, conf.Exchange, false, nil)
		if err != nil {
			return
		}
		err = ch.Qos(conf.PrefetchCount, 0, false)
		if err != nil {
			return
		}
	}
	ai.ch = ch
	return
}

func (ai *AMQPInput) packDecorator(pack *PipelinePack) {
	pack.Message.SetType("amqp")
}

func (ai *AMQPInput) Run(ir InputRunner, h PluginHelper) (err error) {
	var (
		n   int
		e   error
		msg amqp.Delivery
		ok  bool
	)

	stream, err := ai.ch.Consume(ai.config.Queue, "", false, ai.config.QueueExclusive,
		false, false, nil)
	if err != nil {
		return
	}

	sRunner := ir.NewSplitterRunner("")
	if !sRunner.UseMsgBytes() {
		sRunner.SetPackDecorator(ai.packDecorator)
	}

	defer func() {
		ai.usageWg.Done()
		sRunner.Done()
	}()

	for {
		e = nil
		if msg, ok = <-stream; !ok {
			break
		}

		n, e = sRunner.SplitBytes(msg.Body, nil)
		if e != nil {
			ir.LogError(fmt.Errorf("processing message of type %s: %s", msg.Type, e.Error()))
		}
		if n > 0 && n != len(msg.Body) {
			ir.LogError(fmt.Errorf("extra data in message of type %s dropped", msg.Type))
		}
		msg.Ack(false)
	}
	return nil
}

func (ai *AMQPInput) CleanupForRestart() {
	ai.amqpHub.Close(ai.config.URL, ai.connWg)
	ai.connWg.Wait()
}

func (ai *AMQPInput) Stop() {
	ai.ch.Close()
	ai.amqpHub.Close(ai.config.URL, ai.connWg)
	ai.connWg.Wait()
}

func init() {
	RegisterPlugin("AMQPInput", func() interface{} {
		return new(AMQPInput)
	})
}
