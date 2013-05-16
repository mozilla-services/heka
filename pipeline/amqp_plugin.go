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
	"fmt"
	"github.com/mozilla-services/heka/message"
	"github.com/streadway/amqp"
	"sync"
	"time"
)

// AMQP Input config struct
type AMQPInputConfig struct {
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
	// Names of configured `LoglineDecoder` instances used to decode this
	//  message off the input. The message will be de-serialized per its
	// content-type before this decoder is run.
	Decoders []string
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

// AMQP Output config struct
type AMQPOutputConfig struct {
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
}

type AMQPConnectionTracker struct {
	conn    *amqp.Connection
	usageWg *sync.WaitGroup
	connWg  *sync.WaitGroup
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

// Returns a channel for the specified AMQPBroker
//
// The caller may then wait for the connectionWg to get notified when the
// connection has been torn down.
func (ah *AMQPConnectionHub) GetChannel(url string) (ch *amqp.Channel,
	usageWg, connectionWg *sync.WaitGroup, err error) {
	ah.mutex.Lock()
	defer ah.mutex.Unlock()

	var (
		ok   bool
		trk  *AMQPConnectionTracker
		conn *amqp.Connection
	)
	if trk, ok = ah.connections[url]; !ok {
		// Create the connection
		conn, err = amqp.Dial(url)
		if err != nil {
			return
		}
		connectionWg = new(sync.WaitGroup)
		connectionWg.Add(1)
		usageWg = new(sync.WaitGroup)
		ah.connections[url] = &AMQPConnectionTracker{conn, usageWg, connectionWg}

		// Register our listener to remove this connection if its closed
		// after all the channels using it have been closed
		errChan := make(chan *amqp.Error)
		go func(c <-chan *amqp.Error) {
			select {
			case <-c:
				ah.mutex.Lock()
				usageWg.Wait()
				defer func() {
					ah.mutex.Unlock()
					connectionWg.Done()
				}()
				delete(ah.connections, url)
			}
		}(conn.NotifyClose(errChan))
	} else {
		conn = trk.conn
		connectionWg = trk.connWg
		usageWg = trk.usageWg
	}
	ch, err = conn.Channel()
	if err == nil {
		usageWg.Add(1)
	}
	return
}

// Close the underlying connection
//
// The connection will not be closed if the connection waitgroup passed
// in does not match the one created with this connection, at which point
// the call will be ignored as its for an older connection that has
// already been removed
func (ah *AMQPConnectionHub) Close(url string, connWg *sync.WaitGroup) {
	trk, ok := ah.connections[url]
	if !ok {
		return
	}
	if trk.connWg != connWg {
		return
	}
	trk.conn.Close()
}

type AMQPOutput struct {
	// Hold a copy of the config used
	config *AMQPOutputConfig
	// The AMQP Channel created upon Init
	ch *amqp.Channel
	// closeChan gets sent an error should the channel close so that
	// we can immediately exit the output
	closeChan chan *amqp.Error
	usageWg   *sync.WaitGroup
	// connWg tracks whether the connection is no longer in use
	// and is used as a barrier to ensure all users of the connection
	// are done before we finish
	connWg *sync.WaitGroup
}

func (ao *AMQPOutput) ConfigStruct() interface{} {
	return &AMQPOutputConfig{
		ExchangeDurability: false,
		ExchangeAutoDelete: true,
		RoutingKey:         "",
		Persistent:         false,
	}
}

func (ao *AMQPOutput) Init(config interface{}) (err error) {
	conf := config.(*AMQPOutputConfig)
	ao.config = conf
	ch, usageWg, connectionWg, err := amqpHub.GetChannel(conf.URL)
	if err != nil {
		return
	}
	ao.connWg = connectionWg
	ao.usageWg = usageWg
	closeChan := make(chan *amqp.Error)
	ao.closeChan = ch.NotifyClose(closeChan)
	err = ch.ExchangeDeclare(conf.Exchange, conf.ExchangeType,
		conf.ExchangeDurability, conf.ExchangeAutoDelete, false, false,
		nil)
	if err != nil {
		usageWg.Done()
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
		plc     *PipelineCapture
		msg     *message.Message
		persist uint8
		ok      bool = true
	)
	if conf.Persistent {
		persist = uint8(1)
	} else {
		persist = uint8(0)
	}

	for ok {
		select {
		case <-ao.closeChan:
			ok = false
		case plc, ok = <-inChan:
			if !ok {
				break
			}
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
				ok = false
			} else {
				pack.Recycle()
			}
		}
	}
	ao.usageWg.Done()
	amqpHub.Close(conf.URL, ao.connWg)
	ao.connWg.Wait()
	return
}

func (ao *AMQPOutput) CleanupForRestart() {
	amqpHub.Close(ao.config.URL, ao.connWg)
	ao.connWg.Wait()
	return
}

type AMQPInput struct {
	config  *AMQPInputConfig
	ch      *amqp.Channel
	usageWg *sync.WaitGroup
	connWg  *sync.WaitGroup
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
	}
}

func (ai *AMQPInput) Init(config interface{}) (err error) {
	conf := config.(*AMQPInputConfig)
	ai.config = conf
	ch, usageWg, connWg, err := amqpHub.GetChannel(conf.URL)
	if err != nil {
		return
	}
	defer func() {
		if err != nil {
			usageWg.Done()
		}
	}()
	ai.connWg = connWg
	ai.usageWg = usageWg
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
	var (
		dRunner DecoderRunner
		pack    *PipelinePack
		e       error
		ok      bool
	)
	defer ai.usageWg.Done()
	packSupply := ir.InChan()

	conf := ai.config
	dSet := h.DecoderSet()
	decoders := make([]Decoder, len(conf.Decoders))
	for i, name := range conf.Decoders {
		if dRunner, ok = dSet.ByName(name); !ok {
			return fmt.Errorf("Decoder not found: %s", name)
		}
		decoders[i] = dRunner.Decoder()
	}

	stream, err := ai.ch.Consume(conf.Queue, "", false, conf.QueueExclusive,
		false, false, nil)
	if err != nil {
		return
	}
readLoop:
	for {
		pack = <-packSupply
		select {
		case msg, ok := <-stream:
			if !ok {
				break readLoop
			}
			pack.Message.SetType("amqp")
			pack.Message.SetPayload(string(msg.Body))
			pack.Message.SetTimestamp(msg.Timestamp.UnixNano())
			for _, decoder := range decoders {
				if e = decoder.Decode(pack); e == nil {
					break
				}
			}
			pack.Decoded = true
			if e == nil {
				ir.Inject(pack)
			} else {
				ir.LogError(fmt.Errorf("Couldn't parse AMQP message: %s", msg))
				pack.Recycle()
			}
			e = nil
			msg.Ack(false)
		}
	}
	return
}

func (ai *AMQPInput) CleanupForRestart() {
	amqpHub.Close(ai.config.URL, ai.connWg)
	ai.connWg.Wait()
}

func (ai *AMQPInput) Stop() {
	ai.ch.Close()
	amqpHub.Close(ai.config.URL, ai.connWg)
	ai.connWg.Wait()
}

func init() {
	amqpHub = new(AMQPConnectionHub)
	amqpHub.connections = make(map[string]*AMQPConnectionTracker)
	amqpHub.mutex = new(sync.Mutex)
}
