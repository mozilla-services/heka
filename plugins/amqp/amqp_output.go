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
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	. "github.com/mozilla-services/heka/pipeline"
	"github.com/mozilla-services/heka/plugins/tcp"
	"github.com/streadway/amqp"
)

// AMQP Output config struct
type AMQPOutputConfig struct {
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
	ContentType string `toml:"content_type"`
	// Allows for default encoder.
	Encoder string
	// Allows us to use framing by default.
	UseFraming bool `toml:"use_framing"`
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
	// Hold a reference to the connection hub.
	amqpHub AMQPConnectionHub
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

	var dialer = NewAMQPDialer(tlsConf)
	if ao.amqpHub == nil {
		ao.amqpHub = GetAmqpHub()
	}
	ch, usageWg, connectionWg, err := ao.amqpHub.GetChannel(conf.URL, dialer)
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
		persist = amqp.Persistent
	} else {
		persist = amqp.Transient
	}

	// Spin up separate goroutine so we can wait for close notifications from
	// the AMQP lib w/o deadlocking on our `AMQPChannel.Publish` call.
	stopChan := make(chan struct{})
	go func() {
		<-ao.closeChan
		close(stopChan)
	}()

	for ok {
		select {
		case <-stopChan:
			ok = false
		case pack, ok = <-inChan:
			if !ok {
				break
			}
			if outBytes, err = or.Encode(pack); err != nil {
				or.UpdateCursor(pack.QueueCursor)
				err = fmt.Errorf("Error encoding message: %s", err)
				pack.Recycle(err)
				continue
			} else if outBytes == nil {
				or.UpdateCursor(pack.QueueCursor)
				pack.Recycle(nil)
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
				err = NewRetryMessageError(err.Error())
				ok = false
			} else {
				or.UpdateCursor(pack.QueueCursor)
			}
			pack.Recycle(err)
		}
	}
	ao.usageWg.Done()
	ao.amqpHub.Close(conf.URL, ao.connWg)
	ao.connWg.Wait()
	<-stopChan
	return
}

func (ao *AMQPOutput) CleanupForRestart() {
	return
}

func init() {
	RegisterPlugin("AMQPOutput", func() interface{} {
		return new(AMQPOutput)
	})
}
