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
	"bytes"
	"crypto/tls"
	"errors"
	"fmt"
	"github.com/mozilla-services/heka/message"
	. "github.com/mozilla-services/heka/pipeline"
	"github.com/mozilla-services/heka/plugins/tcp"
	"github.com/streadway/amqp"
	"strings"
	"sync"
	"time"
)

type AMQPConnection interface {
	Channel() (*amqp.Channel, error)
	Close() error
	NotifyClose(c chan *amqp.Error) chan *amqp.Error
}

type AMQPChannel interface {
	Cancel(consumer string, noWait bool) error
	Close() error
	Confirm(noWait bool) error
	Consume(queue, consumer string, autoAck, exclusive, noLocal, noWait bool, args amqp.Table) (<-chan amqp.Delivery, error)
	ExchangeBind(destination, key, source string, noWait bool, args amqp.Table) error
	ExchangeDeclare(name, kind string, durable, autoDelete, internal, noWait bool, args amqp.Table) error
	ExchangeDelete(name string, ifUnused, noWait bool) error
	ExchangeUnbind(destination, key, source string, noWait bool, args amqp.Table) error
	Flow(active bool) error
	Get(queue string, autoAck bool) (msg amqp.Delivery, ok bool, err error)
	NotifyClose(c chan *amqp.Error) chan *amqp.Error
	NotifyConfirm(ack, nack chan uint64) (chan uint64, chan uint64)
	NotifyFlow(c chan bool) chan bool
	NotifyReturn(c chan amqp.Return) chan amqp.Return
	Publish(exchange, key string, mandatory, immediate bool, msg amqp.Publishing) error
	Qos(prefetchCount, prefetchSize int, global bool) error
	QueueBind(name, key, exchange string, noWait bool, args amqp.Table) error
	QueueDeclare(name string, durable, autoDelete, exclusive, noWait bool, args amqp.Table) (amqp.Queue, error)
	QueueDelete(name string, ifUnused, ifEmpty, noWait bool) (int, error)
	QueueInspect(name string) (amqp.Queue, error)
	QueuePurge(name string, noWait bool) (int, error)
	QueueUnbind(name, key, exchange string, args amqp.Table) error
	Recover(requeue bool) error
	Tx() error
	TxCommit() error
	TxRollback() error
}

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
	// Name of configured decoder instance used to decode the messages.
	Decoder string
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
	// How long a message published to a queue can live before it is discarded (milliseconds).
	// 0 is a valid ttl which mimics "immediate" expiration.
	// Default value is -1 which leaves it undefined.
	QueueTTL int32
	// Optional subsection for TLS configuration of AMQPS connections. If
	// unspecified, the default AMQPS settings will be used.
	Tls tcp.TlsConfig
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
	// Whether messages published should be fully serialized when
	// published. The AMQP input will automatically detect these
	// messages and deserialize them. Defaults to true.
	Serialize bool
	// Optional subsection for TLS configuration of AMQPS connections. If
	// unspecified, the default AMQPS settings will be used.
	Tls tcp.TlsConfig
	// MIME content type for the AMQP header.
	ContentType string
	// Allows for default encoder.
	Encoder string
	// Allows us to use framing by default.
	UseFraming bool `toml:"use_framing"`
}

// Connection tracker that stores the actual AMQP Connection object along
// with its usage waitgroup (tracks if channels are still in use) along with
// a connection waitgroup (for users to wait on so the connection closing
// can be synchronized)
type AMQPConnectionTracker struct {
	conn    AMQPConnection
	usageWg *sync.WaitGroup
	connWg  *sync.WaitGroup
}

// AMQP connection dialer
type AMQPDialer struct {
	tlsConfig *tls.Config
}

// If additional TLS settings were specified, the dialer uses amqp.DialTLS
// instead of amqp.Dial.
func (dialer *AMQPDialer) Dial(url string) (conn AMQPConnection, err error) {
	if dialer.tlsConfig != nil {
		conn, err = amqp.DialTLS(url, dialer.tlsConfig)
	} else {
		conn, err = amqp.Dial(url)
	}
	return
}

// Basic hub that manages AMQP Connections
//
// Since multiple channels should be utilized for a single connection,
// this hub manages the connections, dispensing channels per broker
// config.
type AMQPConnectionHub interface {
	GetChannel(url string, dialer AMQPDialer) (ch AMQPChannel, usageWg, connectionWg *sync.WaitGroup, err error)
	Close(url string, connWg *sync.WaitGroup)
}

// Default AMQP connection hub implementation
type amqpConnectionHub struct {
	connections map[string]*AMQPConnectionTracker
	mutex       *sync.Mutex
}

// The global AMQP Connection Hub
var amqpHub AMQPConnectionHub

// Returns a channel for the specified AMQPBroker
//
// The caller may then wait for the connectionWg to get notified when the
// connection has been torn down.
func (ah *amqpConnectionHub) GetChannel(url string, dialer AMQPDialer) (ch AMQPChannel,
	usageWg, connectionWg *sync.WaitGroup, err error) {
	ah.mutex.Lock()
	defer ah.mutex.Unlock()

	var (
		ok   bool
		trk  *AMQPConnectionTracker
		conn AMQPConnection
	)
	if trk, ok = ah.connections[url]; !ok {
		// Create the connection
		conn, err = dialer.Dial(url)
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
			<-c
			ah.mutex.Lock()
			usageWg.Wait()
			defer func() {
				ah.mutex.Unlock()
				connectionWg.Done()
			}()
			delete(ah.connections, url)
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
func (ah *amqpConnectionHub) Close(url string, connWg *sync.WaitGroup) {
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
	ch AMQPChannel
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
		Encoder:            "ProtobufEncoder",
		UseFraming:         true,
		ContentType:        "application/hekad",
	}
}

func (ao *AMQPOutput) Init(config interface{}) (err error) {
	conf := config.(*AMQPOutputConfig)
	ao.config = conf
	var tlsConf *tls.Config = nil
	if strings.HasPrefix(conf.URL, "amqps://") && &ao.config.Tls != nil {
		if tlsConf, err = tcp.CreateGoTlsConfig(&ao.config.Tls); err != nil {
			return fmt.Errorf("TLS init error: %s", err)
		}
	}

	var dialer = AMQPDialer{tlsConf}
	ch, usageWg, connectionWg, err := amqpHub.GetChannel(conf.URL, dialer)
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
	if or.Encoder() == nil {
		return errors.New("Encoder required.")
	}

	inChan := or.InChan()
	conf := ao.config

	var (
		pack     *PipelinePack
		persist  uint8
		ok       bool = true
		amqpMsg  amqp.Publishing
		outBytes []byte
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
		case pack, ok = <-inChan:
			if !ok {
				break
			}
			if outBytes, err = or.Encode(pack); err != nil {
				or.LogError(fmt.Errorf("Error encoding message: %s", err))
				pack.Recycle()
				continue
			}
			amqpMsg = amqp.Publishing{
				DeliveryMode: persist,
				Timestamp:    time.Now(),
				ContentType:  conf.ContentType,
				Body:         outBytes,
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
	ch      AMQPChannel
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
		QueueTTL:           -1,
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

	var dialer = AMQPDialer{tlsConf}
	ch, usageWg, connWg, err := amqpHub.GetChannel(conf.URL, dialer)
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
	err = ch.ExchangeDeclare(conf.Exchange, conf.ExchangeType,
		conf.ExchangeDurability, conf.ExchangeAutoDelete, false, false,
		nil)
	if err != nil {
		return
	}
	ai.ch = ch
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
	return
}

func (ai *AMQPInput) Run(ir InputRunner, h PluginHelper) (err error) {
	var (
		dRunner DecoderRunner
		decoder Decoder
		pack    *PipelinePack
		e       error
		ok      bool
	)
	defer ai.usageWg.Done()
	packSupply := ir.InChan()

	conf := ai.config

	// Right now we have a kludgey and brittle way of mapping messages to
	// decoders. A (protobuf) decoder is required if a message of type
	// `application/hekad` is received. Any other message type doesn't require
	// a decoder. In either case, the specified decoder must be able to handle
	// the incoming data. This can (and will) be greatly improved once we
	// abstract out the stream parsing code so it can used here w/o having to
	// reimplement the entire stream_type -> stream parser mapping.
	if conf.Decoder != "" {
		if dRunner, ok = h.DecoderRunner(conf.Decoder, fmt.Sprintf("%s-%s", ir.Name(), conf.Decoder)); !ok {
			return fmt.Errorf("Decoder not found: %s", conf.Decoder)
		}
		decoder = dRunner.Decoder()
	}
	header := &message.Header{}

	stream, err := ai.ch.Consume(conf.Queue, "", false, conf.QueueExclusive,
		false, false, nil)
	if err != nil {
		return
	}
readLoop:
	for {
		e = nil
		pack = <-packSupply
		msg, ok := <-stream
		if !ok {
			break readLoop
		}

		if msg.ContentType == "application/hekad" {
			if dRunner == nil {
				pack.Recycle()
				ir.LogError(errors.New("`application/hekad` messages require a decoder."))
			}
			_, msgOk := findMessage(msg.Body, header, &(pack.MsgBytes))
			if msgOk {
				dRunner.InChan() <- pack
			} else {
				pack.Recycle()
				ir.LogError(errors.New("Can't find Heka message."))
			}
			header.Reset()
		} else {
			pack.Message.SetType("amqp")
			pack.Message.SetPayload(string(msg.Body))
			pack.Message.SetTimestamp(msg.Timestamp.UnixNano())
			var packs []*PipelinePack
			if decoder == nil {
				packs = []*PipelinePack{pack}
			} else {
				packs, e = decoder.Decode(pack)
			}
			if packs != nil {
				for _, p := range packs {
					ir.Inject(p)
				}
			} else {
				if e != nil {
					ir.LogError(fmt.Errorf("Couldn't parse AMQP message: %s", msg.Body))
				}
				pack.Recycle()
			}
		}
		msg.Ack(false)
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

// Scans provided byte slice for the next record separator and tries to
// populate provided header and message objects using the scanned data.
// Returns new starting index into the passed buffer, or ok == false if no
// message was able to be extracted.
//
// TODO this code duplicates what is in the StreamParser and should be
// removed.
func findMessage(buf []byte, header *message.Header, msg *[]byte) (pos int, ok bool) {
	pos = bytes.IndexByte(buf, message.RECORD_SEPARATOR)
	if pos != -1 {
		if len(buf)-pos > 1 {
			headerLength := int(buf[pos+1])
			headerEnd := pos + headerLength + 3 // recsep+len+header+unitsep
			if len(buf) >= headerEnd {
				if header.MessageLength != nil || DecodeHeader(buf[pos+2:headerEnd], header) {
					messageEnd := headerEnd + int(header.GetMessageLength())
					if len(buf) >= messageEnd {
						*msg = (*msg)[:messageEnd-headerEnd]
						copy(*msg, buf[headerEnd:messageEnd])
						pos = messageEnd
						ok = true
					} else {
						*msg = (*msg)[:0]
					}
				} else {
					pos, ok = findMessage(buf[pos+1:], header, msg)
				}
			}
		}
	} else {
		pos = len(buf)
	}
	return
}

func init() {
	ach := new(amqpConnectionHub)
	ach.connections = make(map[string]*AMQPConnectionTracker)
	ach.mutex = new(sync.Mutex)
	amqpHub = ach
	RegisterPlugin("AMQPOutput", func() interface{} {
		return new(AMQPOutput)
	})
	RegisterPlugin("AMQPInput", func() interface{} {
		return new(AMQPInput)
	})
}
