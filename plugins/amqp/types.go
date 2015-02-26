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
	"github.com/streadway/amqp"
	"sync"
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

// Connection tracker that stores the actual AMQP Connection object along
// with its usage waitgroup (tracks if channels are still in use) along with
// a connection waitgroup (for users to wait on so the connection closing
// can be synchronized)
type AMQPConnectionTracker struct {
	conn    AMQPConnection
	usageWg *sync.WaitGroup
	connWg  *sync.WaitGroup
}

// NewAMQPDialer exposes factory for AMQPDialers
func NewAMQPDialer(tlsConfig *tls.Config) AMQPDialer {
	return AMQPDialer{tlsConfig}
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
	GetChannel(url string, dialer AMQPDialer) (ch AMQPChannel,
		usageWg, connectionWg *sync.WaitGroup, err error)
	Close(url string, connWg *sync.WaitGroup)
}

// Default AMQP connection hub implementation
type amqpConnectionHub struct {
	connections map[string]*AMQPConnectionTracker
	mutex       *sync.Mutex
}

// The global AMQP Connection Hub
var amqpHub AMQPConnectionHub

func newAmqpHub() AMQPConnectionHub {
	ach := new(amqpConnectionHub)
	ach.connections = make(map[string]*AMQPConnectionTracker)
	ach.mutex = new(sync.Mutex)
	return ach
}

func GetAmqpHub() AMQPConnectionHub {
	if amqpHub == nil {
		amqpHub = newAmqpHub()
	}
	return amqpHub
}

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
	ah.mutex.Lock()
	defer ah.mutex.Unlock()

	trk, ok := ah.connections[url]
	if !ok {
		return
	}
	if trk.connWg != connWg {
		return
	}
	trk.conn.Close()
}
