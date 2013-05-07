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
#
# ***** END LICENSE BLOCK *****/

package pipeline

import (
	"github.com/mozilla-services/heka/message"
	"github.com/streadway/amqp"
	"sync"
	"time"
)

// Base AMQP config struct used for Input/Output AMQP Plugins
type AMQPConfig struct {
	// AMQP URL. Spec: http://www.rabbitmq.com/uri-spec.html
	// Ex: amqp://USERNAME:PASSWORD@HOSTNAME:PORT/
	URL string
	// Exchange name
	Exchange string
	// Type of exchange, options are: fanout, direct, topic, headers
	ExchangeType string
	// Whether the exchange should be durable or not
	// Defaults to non-durable
	ExchangeDurability bool
	// Whether the exchange is deleted when all queues have finished
	// Defaults to auto-delete
	ExchangeAutoDelete bool
	// Routing key for the message to send, or when used for consumer
	// the routing key to bind the queue to the exchange with
	// Defaults to empty string
	RoutingKey string
	// Whether messages published should be marked as persistent or
	// transient. Defaults to non-persistent.
	Persistent bool
	// How many messages should be pre-fetched before message acks
	// See http://www.rabbitmq.com/blog/2012/04/25/rabbitmq-performance-measurements-part-2/
	// for benchmarks showing the impact of low prefetch counts
	// Defaults to 2
	PrefetchCount int
	// Name of the queue to consume from, an empty string will have the
	// broker generate a name
	Queue string
	// Whether the queue is durable or not
	// Defaults to non-durable
	QueueDurability bool
	// Whether the queue is exclusive or not
	// Defaults to non-exclusive
	QueueExclusive bool
	// Whether the queue is deleted when the last consumer un-subscribes
	// Defaults to auto-delete
	QueueAutoDelete bool
}

func NewAMQPConfig() *AMQPConfig {
	return &AMQPConfig{
		ExchangeDurability: false,
		ExchangeAutoDelete: true,
		Persistent:         false,
		PrefetchCount:      2,
		RoutingKey:         "",
		Queue:              "",
		QueueDurability:    false,
		QueueExclusive:     false,
		QueueAutoDelete:    true,
	}
}

type AMQPConnectionTracker struct {
	conn  *amqp.Connection
	inuse int
}

// Basic hub that manages AMQP Connections
//
// Since multiple channels should be utilized for a single connection,
// this hub manages the connections, dispensing channels per broker
// config.
type AMQPConnectionHub struct {
	connections map[string]*AMQPConnectionTracker
	mutex       *sync.Mutex
}

// The global AMQP Connection Hub
var amqpHub *AMQPConnectionHub

// Returns a connection for the specified AMQPBroker, if the existing
// connection is nil
func (ah *AMQPConnectionHub) GetChannel(url string) (ch *amqp.Channel, err error) {
	ah.mutex.Lock()
	defer ah.mutex.Unlock()
	var ok bool
	var trk *AMQPConnectionTracker
	var conn *amqp.Connection
	if trk, ok = ah.connections[url]; !ok {
		// Create the connection
		conn, err = amqp.Dial(url)
		if err != nil {
			return
		}
		ah.connections[url] = &AMQPConnectionTracker{conn, 1}

		// Register our listener to remove this connection if its closed
		errChan := make(chan *amqp.Error)
		go func(c <-chan *amqp.Error) {
			select {
			case <-c:
				ah.mutex.Lock()
				defer ah.mutex.Unlock()
				delete(ah.connections, url)
			}
		}(conn.NotifyClose(errChan))
	} else {
		trk.inuse += 1
		conn = trk.conn
	}
	ch, err = conn.Channel()
	return
}

// Closes the connection as long as no on else is using it
func (ah *AMQPConnectionHub) CloseConnection(url string) {
	ah.mutex.Lock()
	defer ah.mutex.Unlock()
	trk, ok := ah.connections[url]
	if !ok {
		return
	}
	trk.inuse -= 1
	if trk.inuse == 0 {
		trk.conn.Close()
	}
	return
}

type AMQPOutput struct {
	config *AMQPConfig
	ch     *amqp.Channel
}

func (ao *AMQPOutput) ConfigStruct() interface{} {
	return NewAMQPConfig()
}

func (ao *AMQPOutput) Init(config interface{}) (err error) {
	conf := config.(*AMQPConfig)
	ao.config = conf
	ch, err := amqpHub.GetChannel(conf.URL)
	if err != nil {
		return
	}
	err = ch.ExchangeDeclare(conf.Exchange, conf.ExchangeType,
		conf.ExchangeDurability, conf.ExchangeAutoDelete, false, false,
		nil)
	if err != nil {
		return
	}
	ao.ch = ch
	return
}

func (ao *AMQPOutput) Run(or OutputRunner, h PluginHelper) (err error) {
	inChan := or.InChan()
	conf := ao.config

	var (
		pack    *PipelinePack
		msg     *message.Message
		persist uint8
	)
	if conf.Persistent {
		persist = uint8(1)
	} else {
		persist = uint8(0)
	}

	for plc := range inChan {
		pack = plc.Pack
		msg = pack.Message
		amqpMsg := amqp.Publishing{
			DeliveryMode: persist,
			Timestamp:    time.Now(),
			ContentType:  "text/plain",
			Body:         []byte(msg.GetPayload()),
		}
		err = ao.ch.Publish(conf.Exchange, conf.RoutingKey,
			false, false, amqpMsg)
		if err != nil {
			return
		}
		pack.Recycle()
	}
	err = ao.ch.Close()
	amqpHub.CloseConnection(ao.config.URL)
	return
}

type AMQPInput struct {
	config *AMQPConfig
	ch     *amqp.Channel
}

func (ai *AMQPInput) ConfigStruct() interface{} {
	return NewAMQPConfig()
}

func (ai *AMQPInput) Init(config interface{}) (err error) {
	conf := config.(*AMQPConfig)
	ai.config = conf
	ch, err := amqpHub.GetChannel(conf.URL)
	if err != nil {
		return
	}
	err = ch.ExchangeDeclare(conf.Exchange, conf.ExchangeType,
		conf.ExchangeDurability, conf.ExchangeAutoDelete, false, false,
		nil)
	if err != nil {
		return
	}
	ai.ch = ch
	_, err = ch.QueueDeclare(conf.Queue, conf.QueueDurability,
		conf.QueueAutoDelete, conf.QueueExclusive, false, nil)
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
	return
}

func (ai *AMQPInput) Run(ir InputRunner, h PluginHelper) (err error) {
	var pack *PipelinePack
	conf := ai.config
	stream, err := ai.ch.Consume(conf.Queue, "", false, conf.QueueExclusive,
		false, false, nil)
	if err != nil {
		return
	}
readLoop:
	for {
		pack = <-ir.InChan()
		select {
		case msg, ok := <-stream:
			if !ok {
				break readLoop
			}
			pack.Decoded = true
			pack.Message.SetTimestamp(msg.Timestamp.UnixNano())
			pack.Message.SetPayload(string(msg.Body))
			pack.Message.SetType("amqp")
			ir.Inject(pack)
			msg.Ack(false)
		}
	}
	return
}

func (ai *AMQPInput) Stop() {
	ai.ch.Close()
	amqpHub.CloseConnection(ai.config.URL)
}

func init() {
	amqpHub = new(AMQPConnectionHub)
	amqpHub.connections = make(map[string]*AMQPConnectionTracker)
	amqpHub.mutex = new(sync.Mutex)
}
