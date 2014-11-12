/***** BEGIN LICENSE BLOCK *****
# This Source Code Form is subject to the terms of the Mozilla Public
# License, v. 2.0. If a copy of the MPL was not distributed with this file,
# You can obtain one at http://mozilla.org/MPL/2.0/.
#
# The Initial Developer of the Original Code is the Mozilla Foundation.
# Portions created by the Initial Developer are Copyright (C) 2013-2014
# the Initial Developer. All Rights Reserved.
#
# Contributor(s):
#   Tanguy Leroux (tlrx.dev@gmail.com)
#   Rob Miller (rmiller@mozilla.com)
#
# ***** END LICENSE BLOCK *****/

package elasticsearch

import (
	"bytes"
	"errors"
	"fmt"
	. "github.com/mozilla-services/heka/pipeline"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// Output plugin that index messages to an elasticsearch cluster.
// Largely based on FileOutput plugin.
type ElasticSearchOutput struct {
	flushInterval uint32
	flushCount    int
	batchChan     chan []byte
	backChan      chan []byte
	// The BulkIndexer used to index documents
	bulkIndexer BulkIndexer

	// Specify an overall timeout value in milliseconds for bulk request to complete.
	// Default is 0 (infinite)
	http_timeout uint32
	//Disable both TCP and HTTP keepalives
	http_disable_keepalives bool
	// Specify a resolve and connect timeout value in milliseconds for bulk request.
	// It's always included in overall request timeout (see 'http_timeout' option).
	// Default is 0 (infinite)
	connect_timeout uint32
}

// ConfigStruct for ElasticSearchOutput plugin.
type ElasticSearchOutputConfig struct {
	// Interval at which accumulated messages should be bulk indexed to
	// ElasticSearch, in milliseconds (default 1000, i.e. 1 second).
	FlushInterval uint32 `toml:"flush_interval"`
	// Number of messages that triggers a bulk indexation to ElasticSearch
	// (default to 10)
	FlushCount int `toml:"flush_count"`
	// ElasticSearch server address. This address also defines the Bulk
	// indexing mode. For example, "http://localhost:9200" defines a server
	// accessible on localhost and the indexing will be done with the HTTP
	// Bulk API, whereas "udp://192.168.1.14:9700" defines a server accessible
	// on the local network and the indexing will be done with the UDP Bulk
	// API. (default to "http://localhost:9200")
	Server string
	// Overall timeout
	HTTPTimeout uint32 `toml:"http_timeout"`
	// Disable both TCP and HTTP keepalives
	HTTPDisableKeepalives bool `toml:"http_disable_keepalives"`
	// Resolve and connect timeout only
	ConnectTimeout uint32 `toml:"connect_timeout"`
}

func (o *ElasticSearchOutput) ConfigStruct() interface{} {
	return &ElasticSearchOutputConfig{
		FlushInterval:         1000,
		FlushCount:            10,
		Server:                "http://localhost:9200",
		HTTPTimeout:           0,
		HTTPDisableKeepalives: false,
		ConnectTimeout:        0,
	}
}

func (o *ElasticSearchOutput) Init(config interface{}) (err error) {
	conf := config.(*ElasticSearchOutputConfig)
	o.flushInterval = conf.FlushInterval
	o.flushCount = conf.FlushCount
	o.batchChan = make(chan []byte)
	o.backChan = make(chan []byte, 2)
	o.http_timeout = conf.HTTPTimeout
	o.http_disable_keepalives = conf.HTTPDisableKeepalives
	o.connect_timeout = conf.ConnectTimeout
	var serverUrl *url.URL
	if serverUrl, err = url.Parse(conf.Server); err == nil {
		switch strings.ToLower(serverUrl.Scheme) {
		case "http", "https":
			o.bulkIndexer = NewHttpBulkIndexer(strings.ToLower(serverUrl.Scheme), serverUrl.Host,
				o.flushCount, o.http_timeout, o.http_disable_keepalives, o.connect_timeout)
		case "udp":
			o.bulkIndexer = NewUDPBulkIndexer(serverUrl.Host, o.flushCount)
		default:
			err = errors.New("Server URL must specify one of `udp`, `http`, or `https`.")
		}
	} else {
		err = fmt.Errorf("Unable to parse ElasticSearch server URL [%s]: %s", conf.Server, err)
	}
	return
}

func (o *ElasticSearchOutput) Run(or OutputRunner, h PluginHelper) (err error) {
	if or.Encoder() == nil {
		return errors.New("Encoder must be specified.")
	}
	var wg sync.WaitGroup
	wg.Add(2)
	go o.receiver(or, &wg)
	go o.committer(or, &wg)
	wg.Wait()
	return
}

// Runs in a separate goroutine, accepting incoming messages, buffering output
// data until the ticker triggers the buffered data should be put onto the
// committer channel.
func (o *ElasticSearchOutput) receiver(or OutputRunner, wg *sync.WaitGroup) {
	var (
		pack     *PipelinePack
		e        error
		count    int
		outBytes []byte
	)
	ok := true
	ticker := time.Tick(time.Duration(o.flushInterval) * time.Millisecond)
	outBatch := make([]byte, 0, 10000)
	inChan := or.InChan()

	for ok {
		select {
		case pack, ok = <-inChan:
			if !ok {
				// Closed inChan => we're shutting down, flush data
				if len(outBatch) > 0 {
					o.batchChan <- outBatch
				}
				close(o.batchChan)
				break
			}
			outBytes, e = or.Encode(pack)
			pack.Recycle()
			if e != nil {
				or.LogError(e)
			} else if outBytes != nil {
				outBatch = append(outBatch, outBytes...)
				if count = count + 1; o.bulkIndexer.CheckFlush(count, len(outBatch)) {
					if len(outBatch) > 0 {
						// This will block until the other side is ready to accept
						// this batch, so we can't get too far ahead.
						o.batchChan <- outBatch
						outBatch = <-o.backChan
						count = 0
					}
				}
			}
		case <-ticker:
			if len(outBatch) > 0 {
				// This will block until the other side is ready to accept
				// this batch, freeing us to start on the next one.
				o.batchChan <- outBatch
				outBatch = <-o.backChan
				count = 0
			}
		}
	}
	wg.Done()
}

// Runs in a separate goroutine, waits for buffered data on the committer
// channel, bulk index it out to the elasticsearch cluster, and puts the now
// empty buffer on the return channel for reuse.
func (o *ElasticSearchOutput) committer(or OutputRunner, wg *sync.WaitGroup) {
	initBatch := make([]byte, 0, 10000)
	o.backChan <- initBatch
	var outBatch []byte

	for outBatch = range o.batchChan {
		if err := o.bulkIndexer.Index(outBatch); err != nil {
			or.LogError(err)
		}
		outBatch = outBatch[:0]
		o.backChan <- outBatch
	}
	wg.Done()
}

// A BulkIndexer is used to index documents in ElasticSearch
type BulkIndexer interface {
	// Index documents
	Index(body []byte) error
	// Check if a flush is needed
	CheckFlush(count int, length int) bool
}

// A HttpBulkIndexer uses the HTTP REST Bulk Api of ElasticSearch
// in order to index documents
type HttpBulkIndexer struct {
	// Protocol (http or https).
	Protocol string
	// Host name and port number (default to "localhost:9200").
	Domain string
	// Maximum number of documents.
	MaxCount int
	// Internal HTTP Client.
	client *http.Client
}

func NewHttpBulkIndexer(protocol string, domain string, maxCount int,
	httpTimeout uint32, httpDisableKeepalives bool, connectTimeout uint32) *HttpBulkIndexer {

	tr := &http.Transport{
		DisableKeepAlives: httpDisableKeepalives,
		Dial: func(network, address string) (net.Conn, error) {
			return net.DialTimeout(network, address, time.Duration(connectTimeout)*time.Millisecond)
		},
	}

	client := &http.Client{
		Transport: tr,
		Timeout:   time.Duration(httpTimeout) * time.Millisecond,
	}
	return &HttpBulkIndexer{
		Protocol: protocol,
		Domain:   domain,
		MaxCount: maxCount,
		client:   client,
	}
}

func (h *HttpBulkIndexer) CheckFlush(count int, length int) bool {
	if count >= h.MaxCount {
		return true
	}
	return false
}

func (h *HttpBulkIndexer) Index(body []byte) error {
	url := fmt.Sprintf("%s://%s%s", h.Protocol, h.Domain, "/_bulk")

	// Creating ElasticSearch Bulk HTTP request
	request, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("can't create bulk request: %s", err.Error())
	}
	request.Header.Add("Accept", "application/json")
	response, err := h.client.Do(request)
	if err != nil {
		return fmt.Errorf("HTTP request failed: %s", err.Error())
	}
	if response != nil {
		defer response.Body.Close()
		if response.StatusCode > 304 {
			return fmt.Errorf("HTTP response error status: %s", response.Status)
		}
		if _, err = ioutil.ReadAll(response.Body); err != nil {
			return fmt.Errorf("can't read HTTP response body: %s", err.Error())
		}
	}
	return nil
}

// A UDPBulkIndexer uses the Bulk UDP Api of ElasticSearch
// in order to index documents
type UDPBulkIndexer struct {
	// Host name and port number (default to "localhost:9700")
	Domain string
	// Maximum number of documents
	MaxCount int
	// Max. length of UDP packets
	MaxLength int
	// Internal UDP Address
	address *net.UDPAddr
	// Internal UDP Client
	client *net.UDPConn
}

func NewUDPBulkIndexer(domain string, maxCount int) *UDPBulkIndexer {
	return &UDPBulkIndexer{Domain: domain, MaxCount: maxCount, MaxLength: 65000}
}

func (u *UDPBulkIndexer) CheckFlush(count int, length int) bool {
	if length >= u.MaxLength {
		return true
	} else if count >= u.MaxCount {
		return true
	}
	return false
}

func (u *UDPBulkIndexer) Index(body []byte) error {
	var err error
	if u.address == nil {
		if u.address, err = net.ResolveUDPAddr("udp", u.Domain); err != nil {
			return fmt.Errorf("Error resolving UDP address [%s]: %s", u.Domain, err)
		}
	}
	if u.client == nil {
		if u.client, err = net.DialUDP("udp", nil, u.address); err != nil {
			return fmt.Errorf("Error creating UDP client: %s", err)
		}
	}
	if u.address != nil {
		if _, err = u.client.Write(body[:]); err != nil {
			return fmt.Errorf("Error writing data to UDP server: %s", err)
		}
	} else {
		return fmt.Errorf("Error writing data to UDP server, address not found")
	}
	return nil
}

func init() {
	RegisterPlugin("ElasticSearchOutput", func() interface{} {
		return new(ElasticSearchOutput)
	})
}
