/***** BEGIN LICENSE BLOCK *****
# This Source Code Form is subject to the terms of the Mozilla Public
# License, v. 2.0. If a copy of the MPL was not distributed with this file,
# You can obtain one at http://mozilla.org/MPL/2.0/.
#
# The Initial Developer of the Original Code is the Mozilla Foundation.
# Portions created by the Initial Developer are Copyright (C) 2013-2015
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
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/mozilla-services/heka/message"
	. "github.com/mozilla-services/heka/pipeline"
	"github.com/mozilla-services/heka/plugins/tcp"
)

type ESBatch struct {
	queueCursor string
	count       int64
	batch       []byte
}

type MsgPack struct {
	bytes       []byte
	queueCursor string
}

// Output plugin that index messages to an elasticsearch cluster.
// Largely based on FileOutput plugin.
type ElasticSearchOutput struct {
	sentMessageCount int64
	dropMessageCount int64
	count            int64
	backChan         chan []byte
	recvChan         chan MsgPack
	batchChan        chan ESBatch // Chan to pass completed batches
	outBatch         []byte
	queueCursor      string
	bulkIndexer      BulkIndexer // The BulkIndexer used to index documents
	conf             *ElasticSearchOutputConfig
	or               OutputRunner
	outputBlock      *RetryHelper
	pConfig          *PipelineConfig
	reportLock       sync.Mutex
	stopChan         chan bool
	flushTicker      *time.Ticker
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
	// Optional subsection for TLS configuration of ElasticSearch connections. If
	// unspecified, the default ElasticSearch settings will be used.
	Tls tcp.TlsConfig
	// Optional ElasticSearch username for HTTP authentication. This is useful
	// if you have put your ElasticSearch cluster behind a proxy like nginx.
	// and turned on authentication.
	Username string `toml:"username"`
	// Optional password for HTTP authentication.
	Password string `toml:"password"`
	// Specify an overall timeout value in milliseconds for bulk request to complete.
	// Default is 0 (infinite)
	HTTPTimeout uint32 `toml:"http_timeout"`
	// Disable both TCP and HTTP keepalives
	HTTPDisableKeepalives bool `toml:"http_disable_keepalives"`
	// Specify a resolve and connect timeout value in milliseconds for bulk request.
	// It's always included in overall request timeout (see 'http_timeout' option).
	// Default is 0 (infinite)
	ConnectTimeout uint32 `toml:"connect_timeout"`
	// Whether or not to buffer records to disk before sending to ElasticSearch.
	UseBuffering bool `toml:"use_buffering"`
}

func (o *ElasticSearchOutput) ConfigStruct() interface{} {
	return &ElasticSearchOutputConfig{
		FlushInterval:         1000,
		FlushCount:            10,
		Server:                "http://localhost:9200",
		Username:              "",
		Password:              "",
		HTTPTimeout:           0,
		HTTPDisableKeepalives: false,
		ConnectTimeout:        0,
		UseBuffering:          true,
	}
}

func (o *ElasticSearchOutput) Init(config interface{}) (err error) {
	o.conf = config.(*ElasticSearchOutputConfig)

	o.batchChan = make(chan ESBatch)
	o.backChan = make(chan []byte, 2)
	o.recvChan = make(chan MsgPack, 1024)

	var serverUrl *url.URL
	if serverUrl, err = url.Parse(o.conf.Server); err == nil {
		var scheme string = strings.ToLower(serverUrl.Scheme)
		switch scheme {
		case "http", "https":
			var tlsConf *tls.Config = nil
			if scheme == "https" && &o.conf.Tls != nil {
				if tlsConf, err = tcp.CreateGoTlsConfig(&o.conf.Tls); err != nil {
					return fmt.Errorf("TLS init error: %s", err)
				}
			}

			o.bulkIndexer = NewHttpBulkIndexer(scheme, serverUrl.Host, serverUrl.Path,
				o.conf.FlushCount, o.conf.Username, o.conf.Password, o.conf.HTTPTimeout,
				o.conf.HTTPDisableKeepalives, o.conf.ConnectTimeout, tlsConf)
		case "udp":
			o.bulkIndexer = NewUDPBulkIndexer(serverUrl.Host, o.conf.FlushCount)
		default:
			err = errors.New("Server URL must specify one of `udp`, `http`, or `https`.")
		}
	} else {
		err = fmt.Errorf("Unable to parse ElasticSearch server URL [%s]: %s", o.conf.Server, err)
	}
	return
}

func (o *ElasticSearchOutput) Prepare(or OutputRunner, h PluginHelper) error {
	if or.Encoder() == nil {
		return errors.New("Encoder must be specified.")
	}

	o.or = or
	o.pConfig = h.PipelineConfig()
	o.stopChan = or.StopChan()

	var err error
	o.outputBlock, err = NewRetryHelper(RetryOptions{
		MaxDelay:   "5s",
		MaxRetries: -1,
	})
	if err != nil {
		return fmt.Errorf("can't create retry helper: %s", err.Error())
	}

	o.outBatch = make([]byte, 0, 10000)
	go o.committer()

	if o.conf.FlushInterval > 0 {
		d, err := time.ParseDuration(fmt.Sprintf("%dms", o.conf.FlushInterval))
		if err != nil {
			return fmt.Errorf("can't create flush ticker: %s", err.Error())
		}
		o.flushTicker = time.NewTicker(d)
	}
	go o.batchSender()
	return nil
}

func (o *ElasticSearchOutput) ProcessMessage(pack *PipelinePack) error {
	outBytes, err := o.or.Encode(pack)
	if err != nil {
		return fmt.Errorf("can't encode: %s", err)
	}

	if outBytes != nil {
		o.recvChan <- MsgPack{bytes: outBytes, queueCursor: pack.QueueCursor}
	}

	return nil
}

func (o *ElasticSearchOutput) batchSender() {
	ok := true
	for ok {
		select {
		case <-o.stopChan:
			ok = false
			continue
		case pack := <-o.recvChan:
			o.outBatch = append(o.outBatch, pack.bytes...)
			o.queueCursor = pack.queueCursor
			o.count++
			if len(o.outBatch) > 0 && o.bulkIndexer.CheckFlush(int(o.count), len(o.outBatch)) {
				o.sendBatch()
			}
		case <-o.flushTicker.C:
			if len(o.outBatch) > 0 {
				o.sendBatch()
			}
		}
	}
}

func (o *ElasticSearchOutput) sendBatch() {
	b := ESBatch{
		queueCursor: o.queueCursor,
		count:       o.count,
		batch:       o.outBatch,
	}
	o.count = 0
	select {
	case <-o.stopChan:
		return
	case o.batchChan <- b:
	}
	select {
	case <-o.stopChan:
	case o.outBatch = <-o.backChan:
	}
}

// Waits for batched data on the committer channel and sends it out to the
// elasticsearch cluster, putting now empty buffer on the return channel for
// reuse.
func (o *ElasticSearchOutput) committer() {
	o.backChan <- make([]byte, 0, 10000)

	var b ESBatch
	ok := true
	for ok {
		select {
		case <-o.stopChan:
			ok = false
			continue
		case b, ok = <-o.batchChan:
			if !ok {
				continue
			}
		}
		if err := o.sendRecord(b.batch); err != nil {
			atomic.AddInt64(&o.dropMessageCount, b.count)
			o.or.LogError(err)
		} else {
			atomic.AddInt64(&o.sentMessageCount, b.count)
		}
		o.or.UpdateCursor(b.queueCursor)
		b.batch = b.batch[:0]
		o.backChan <- b.batch
	}
}

// sendRecord invokes the indexer to send a batch of data to
// ElasticSearch. Blocks until the send goes through, only returns an error if
// the sending is abandoned.
func (o *ElasticSearchOutput) sendRecord(buffer []byte) error {
	err, retry := o.bulkIndexer.Index(buffer)
	if err == nil {
		return nil
	}
	if !retry {
		return err
	}

	defer o.outputBlock.Reset()
	for {
		select {
		case <-o.stopChan:
			return err
		default:
		}
		e := o.outputBlock.Wait()
		if e != nil {
			break
		}
		err, retry = o.bulkIndexer.Index(buffer)
		if err == nil {
			break
		}
		if !retry {
			break
		}
		o.or.LogError(fmt.Errorf("can't index: %s", err))
	}
	return err
}

func (o *ElasticSearchOutput) CleanUp() {
	if o.flushTicker != nil {
		o.flushTicker.Stop()
	}
}

// Satisfies the `pipeline.ReportingPlugin` interface to provide plugin state
// information to the Heka report and dashboard.
func (o *ElasticSearchOutput) ReportMsg(msg *message.Message) error {
	o.reportLock.Lock()
	defer o.reportLock.Unlock()

	message.NewInt64Field(msg, "SentMessageCount",
		atomic.LoadInt64(&o.sentMessageCount), "count")
	message.NewInt64Field(msg, "DropMessageCount",
		atomic.LoadInt64(&o.dropMessageCount), "count")
	return nil
}

// A BulkIndexer is used to index documents in ElasticSearch
type BulkIndexer interface {
	// Index documents
	Index(body []byte) (err error, retry bool)
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
	// Path (default to "")
	Path string
	// Maximum number of documents.
	MaxCount int
	// Internal HTTP Client.
	client *http.Client
	// Optional username for HTTP authentication
	username string
	// Optional password for HTTP authentication
	password string
}

func NewHttpBulkIndexer(protocol string, domain string, path string, maxCount int,
	username string, password string, httpTimeout uint32, httpDisableKeepalives bool,
	connectTimeout uint32, tlsConf *tls.Config) *HttpBulkIndexer {

	tr := &http.Transport{
		TLSClientConfig:   tlsConf,
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
		Path:     path,
		MaxCount: maxCount,
		client:   client,
		username: username,
		password: password,
	}
}

func (h *HttpBulkIndexer) CheckFlush(count int, length int) bool {
	if count >= h.MaxCount {
		return true
	}
	return false
}

func (h *HttpBulkIndexer) Index(body []byte) (err error, retry bool) {
	var response_body []byte
	var response_body_json map[string]interface{}

	if len(body) == 0 {
		return nil, false
	}

	url := fmt.Sprintf("%s://%s%s%s", h.Protocol, h.Domain, h.Path, "/_bulk")

	// Creating ElasticSearch Bulk HTTP request
	request, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("Can't create bulk request: %s", err.Error()), true
	}
	request.Header.Add("Accept", "application/json")
	if h.username != "" && h.password != "" {
		request.SetBasicAuth(h.username, h.password)
	}

	request_start_time := time.Now()
	response, err := h.client.Do(request)
	request_time := time.Since(request_start_time)
	if err != nil {
		if (h.client.Timeout > 0) && (request_time >= h.client.Timeout) &&
			(strings.Contains(err.Error(), "use of closed network connection")) {

			return fmt.Errorf("HTTP request was interrupted after timeout. It lasted %s",
				request_time.String()), true
		} else {
			return fmt.Errorf("HTTP request failed: %s", err.Error()), true
		}
	}
	if response != nil {
		defer response.Body.Close()
		if response_body, err = ioutil.ReadAll(response.Body); err != nil {
			return fmt.Errorf("Can't read HTTP response body. Status: %s. Error: %s",
				response.Status, err.Error()), true
		}
		err = json.Unmarshal(response_body, &response_body_json)
		if err != nil {
			return fmt.Errorf("HTTP response didn't contain valid JSON. Status: %s. Body: %s",
				response.Status, string(response_body)), true
		}
		json_errors, ok := response_body_json["errors"].(bool)
		if ok && json_errors && response.StatusCode != 200 {
			return fmt.Errorf(
				"ElasticSearch server reported error within JSON. Status: %s. Body: %s",
				response.Status, string(response_body)), false
		}
		if response.StatusCode > 304 {
			return fmt.Errorf("HTTP response error. Status: %s. Body: %s", response.Status,
				string(response_body)), false
		}
	}
	return nil, false
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

func (u *UDPBulkIndexer) Index(body []byte) (err error, retry bool) {
	if u.address == nil {
		if u.address, err = net.ResolveUDPAddr("udp", u.Domain); err != nil {
			return fmt.Errorf("Error resolving UDP address [%s]: %s", u.Domain, err), true
		}
	}
	if u.client == nil {
		if u.client, err = net.DialUDP("udp", nil, u.address); err != nil {
			return fmt.Errorf("Error creating UDP client: %s", err), true
		}
	}
	if u.address != nil {
		if _, err = u.client.Write(body[:]); err != nil {
			return fmt.Errorf("Error writing data to UDP server: %s", err), true
		}
	} else {
		return fmt.Errorf("Error writing data to UDP server, address not found"), true
	}
	return nil, false
}

func init() {
	RegisterPlugin("ElasticSearchOutput", func() interface{} {
		return new(ElasticSearchOutput)
	})
}
