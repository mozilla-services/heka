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
	"github.com/mozilla-services/heka/message"
	. "github.com/mozilla-services/heka/pipeline"
	"github.com/mozilla-services/heka/plugins/tcp"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type ESBatch struct {
	count int64
	batch []byte
}

// Output plugin that index messages to an elasticsearch cluster.
// Largely based on FileOutput plugin.
type ElasticSearchOutput struct {
	backChan            chan []byte
	batchChan           chan ESBatch    // Chan to pass completed batches
	bufferedOut         *BufferedOutput // The output buffering object
	bulkIndexer         BulkIndexer     // The BulkIndexer used to index documents
	conf                *ElasticSearchOutputConfig
	processMessageCount int64
	dropMessageCount    int64
	or                  OutputRunner
	outputBlock         *RetryHelper
	pConfig             *PipelineConfig
	reportLock          sync.Mutex
	outputExit          chan error
	outputError         chan error
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
	// Specifies size of queue buffer for output. 0 means that buffer is
	// unlimited.
	QueueMaxBufferSize uint64 `toml:"queue_max_buffer_size"`
	// Specifies action which should be executed if queue is full. Possible
	// values are "shutdown", "drop", or "block".
	QueueFullAction string `toml:"queue_full_action"`
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
		QueueMaxBufferSize:    0,
		QueueFullAction:       "shutdown",
	}
}

func (o *ElasticSearchOutput) SetName(name string) {
}

func (o *ElasticSearchOutput) Init(config interface{}) (err error) {
	o.conf = config.(*ElasticSearchOutputConfig)

	if !o.conf.UseBuffering {
		o.batchChan = make(chan ESBatch)
		o.backChan = make(chan []byte, 2)
	}

	o.outputExit = make(chan error)

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
	switch o.conf.QueueFullAction {
	case "shutdown", "drop", "block":
	default:
		return fmt.Errorf("`queue_full_action` must be 'shutdown', 'drop', or 'block', got %s",
			o.conf.QueueFullAction)
	}
	return
}

func (o *ElasticSearchOutput) sendBatch(outBatch []byte, count int64) (nextBatch []byte) {
	if o.conf.UseBuffering {
		err := o.bufferedOut.QueueBytes(outBatch)
		if err == nil {
			atomic.AddInt64(&o.processMessageCount, count)
		} else if err == QueueIsFull {
			o.or.LogError(err)
			if !o.queueFull(outBatch, count) {
				return nil // Signals we're shutting down.
			}
		} else if err != nil {
			o.or.LogError(err)
			atomic.AddInt64(&o.dropMessageCount, count)
		}
		nextBatch = outBatch[:0]
	} else {
		// This will block until the other side is ready to accept
		// this batch, so we can't get too far ahead.
		b := ESBatch{
			count: count,
			batch: outBatch,
		}
		o.batchChan <- b
		nextBatch = <-o.backChan
	}
	return nextBatch
}

func (o *ElasticSearchOutput) Run(or OutputRunner, h PluginHelper) (err error) {
	var (
		ok       = true
		pack     *PipelinePack
		inChan   = or.InChan()
		count    int64
		e        error
		outBytes []byte
		stopChan = make(chan bool, 1)
		outBatch = make([]byte, 0, 10000)
		ticker   = time.Tick(time.Duration(o.conf.FlushInterval) * time.Millisecond)
	)

	o.outputError = make(chan error, 5)

	if or.Encoder() == nil {
		return errors.New("Encoder must be specified.")
	}

	re := regexp.MustCompile("\\W")
	name := re.ReplaceAllString(or.Name(), "_")

	o.outputBlock, err = NewRetryHelper(RetryOptions{
		MaxDelay:   "5s",
		MaxRetries: -1,
	})
	if err != nil {
		return fmt.Errorf("can't create retry helper: %s", err.Error())
	}

	o.pConfig = h.PipelineConfig()
	o.or = or

	if o.conf.UseBuffering {
		o.bufferedOut, err = NewBufferedOutput("output_queue", name, or, h,
			o.conf.QueueMaxBufferSize)
		if err != nil {
			if err == QueueIsFull {
				or.LogMessage("Queue capacity is already reached.")
			} else {
				return
			}
		}
		o.bufferedOut.Start(o, o.outputError, o.outputExit, stopChan)
	} else {
		go o.committer()
	}

	for ok {
		select {
		case e := <-o.outputError:
			or.LogError(e)
		case pack, ok = <-inChan:
			// Closed inChan => we're shutting down, flush data.
			if !ok {
				if len(outBatch) > 0 {
					o.sendBatch(outBatch, count)
				}
				if !o.conf.UseBuffering {
					close(o.batchChan)
				}
				stopChan <- true
				// Make sure buffer isn't blocked on sending to outputError
				select {
				case e := <-o.outputError:
					or.LogError(e)
				default:
				}
				<-o.outputExit
				break
			}

			outBytes, e = or.Encode(pack)
			pack.Recycle()
			if e != nil {
				or.LogError(e)
				atomic.AddInt64(&o.dropMessageCount, 1)
				continue
			}

			if outBytes != nil {
				outBatch = append(outBatch, outBytes...)
				if count++; o.bulkIndexer.CheckFlush(int(count), len(outBatch)) {
					if len(outBatch) > 0 {
						outBatch = o.sendBatch(outBatch, count)
						if outBatch == nil {
							ok = false
							break
						}
					}
					count = 0
				}
			}
		case <-ticker:
			if len(outBatch) > 0 {
				if o.conf.UseBuffering {
					if err = o.bufferedOut.RollQueue(); err != nil {
						or.LogError(err)
						return
					}
				}
				outBatch = o.sendBatch(outBatch, count)
				if outBatch == nil {
					ok = false
					break
				}
			}
			count = 0
		case err = <-o.outputExit:
			ok = false
		}
	}
	return
}

// Waits for batched data on the committer channel and sends it out to the
// elasticsearch cluster, putting now empty buffer on the return channel for
// reuse. Only used if we're *not* using disk buffering, in which case the
// BufferedOutput handles this for us.
func (o *ElasticSearchOutput) committer() {
	o.backChan <- make([]byte, 0, 10000)

	for b := range o.batchChan {
		if err := o.SendRecord(b.batch); err != nil {
			atomic.AddInt64(&o.dropMessageCount, b.count)
			o.or.LogError(err)
		} else {
			atomic.AddInt64(&o.processMessageCount, b.count)
		}
		b.batch = b.batch[:0]
		o.backChan <- b.batch
	}
	o.outputExit <- nil
}

func (o *ElasticSearchOutput) queueFull(buffer []byte, count int64) bool {
	switch o.conf.QueueFullAction {
	// Tries to queue message until its possible to send it to output.
	case "block":
		for o.outputBlock.Wait() == nil {
			if o.pConfig.Globals.IsShuttingDown() {
				return false
			}
			blockErr := o.bufferedOut.QueueBytes(buffer)
			if blockErr == nil {
				atomic.AddInt64(&o.processMessageCount, count)
				o.outputBlock.Reset()
				break
			}
			select {
			case e := <-o.outputError:
				o.or.LogError(e)
			default:
			}
			runtime.Gosched()
		}
	// Terminate Heka activity.
	case "shutdown":
		o.pConfig.Globals.ShutDown()
		return false

	// Drop packets
	case "drop":
		atomic.AddInt64(&o.dropMessageCount, count)
	}
	return true
}

func (o *ElasticSearchOutput) SendRecord(buffer []byte) (err error) {
	var retry bool
	err, retry = o.bulkIndexer.Index(buffer)
	if err != nil {
		if retry {
			return err
		}
		o.or.LogError(fmt.Errorf("can't index: %s", err))
	}
	return nil
}

// Satisfies the `pipeline.ReportingPlugin` interface to provide plugin state
// information to the Heka report and dashboard.
func (o *ElasticSearchOutput) ReportMsg(msg *message.Message) error {
	o.reportLock.Lock()
	defer o.reportLock.Unlock()

	message.NewInt64Field(msg, "ProcessMessageCount",
		atomic.LoadInt64(&o.processMessageCount), "count")
	message.NewInt64Field(msg, "DropMessageCount",
		atomic.LoadInt64(&o.dropMessageCount), "count")

	if o.conf.UseBuffering {
		o.bufferedOut.ReportMsg(msg)
	}
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
		if response.StatusCode > 304 {
			return fmt.Errorf("HTTP response error status: %s", response.Status), false
		}
		if response_body, err = ioutil.ReadAll(response.Body); err != nil {
			return fmt.Errorf("Can't read HTTP response body: %s", err.Error()), true
		}
		err = json.Unmarshal(response_body, &response_body_json)
		if err != nil {
			return fmt.Errorf("HTTP response didn't contain valid JSON. Body: %s",
				string(response_body)), true
		}
		json_errors, ok := response_body_json["errors"].(bool)
		if ok && json_errors {
			return fmt.Errorf("ElasticSearch server reported error within JSON: %s",
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
