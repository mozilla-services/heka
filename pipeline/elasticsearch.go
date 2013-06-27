/***** BEGIN LICENSE BLOCK *****
# This Source Code Form is subject to the terms of the Mozilla Public
# License, v. 2.0. If a copy of the MPL was not distributed with this file,
# You can obtain one at http://mozilla.org/MPL/2.0/.
#
# The Initial Developer of the Original Code is the Mozilla Foundation.
# Portions created by the Initial Developer are Copyright (C) 2013
# the Initial Developer. All Rights Reserved.
#
# Contributor(s):
#   Tanguy Leroux (tlrx.dev@gmail.com)
#
# ***** END LICENSE BLOCK *****/

package pipeline

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/mozilla-services/heka/message"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

// Output plugin that index messages to an elasticsearch cluster.
// Largely based on FileOutput plugin.
type ElasticSearchOutput struct {
	clusterName   string
	indexName     string
	typeName      string
	flushInterval uint32
	flushCount    int
	batchChan     chan []byte
	backChan      chan []byte
	format        string
	timestamp     string
	// The Message Formatter to use when converting
	// Heka messages to ElasticSearch documents
	messageFormatter MessageFormatter
	// The BulkIndexer used to index documents
	bulkIndexer BulkIndexer
}

// ConfigStruct for ElasticSearchOutput plugin.
type ElasticSearchOutputConfig struct {
	// ElasticSearch cluster name.
	Cluster string
	// Name of the index in which the messages will be indexed.
	Index string
	// Name of the document type of the messages.
	TypeName string `toml:"type_name"`
	// Interval at which accumulated messages should be bulk indexed to ElasticSearch, in
	// milliseconds (default 1000, i.e. 1 second).
	FlushInterval uint32 `toml:"flush_interval"`
	// Number of messages that triggers a bulk indexation to ElasticSearch
	// (default to 10)
	FlushCount int `toml:"flush_count"`
	// Format of the document.
	Format string
	// Field names to include in ElasticSearch document for "clean" format.
	Fields []string
	// Timestamp format.
	Timestamp string
	// ElasticSearch server address. This address also defines the Bulk
	// indexing mode. For example, "http://localhost:9200" defines a
	// server accessible on localhost and the indexation will be done
	// with the HTTP Bulk API. Whereas "udp://192.168.1.14:9700" defines
	// a server accessible on the local network and the indexation will
	// be done with the UDP Bulk API of ElasticSearch.
	// (default to "http://localhost:9200")
	Server string
}

func (o *ElasticSearchOutput) ConfigStruct() interface{} {
	return &ElasticSearchOutputConfig{
		Cluster:       "elasticsearch",
		Index:         "heka-%{2006.01.02}",
		TypeName:      "message",
		FlushInterval: 1000,
		FlushCount:    10,
		Format:        "clean",
		Timestamp:     "2006-01-02T15:04:05.000Z",
		Server:        "http://localhost:9200",
	}
}

func (o *ElasticSearchOutput) Init(config interface{}) (err error) {
	conf := config.(*ElasticSearchOutputConfig)
	o.clusterName = conf.Cluster
	o.indexName = conf.Index
	o.typeName = conf.TypeName
	o.flushInterval = conf.FlushInterval
	o.flushCount = conf.FlushCount
	o.batchChan = make(chan []byte)
	o.backChan = make(chan []byte, 2)
	o.format = conf.Format
	switch strings.ToLower(conf.Format) {
	case "raw":
		o.messageFormatter = NewRawMessageFormatter()
	case "clean":
		o.messageFormatter = NewCleanMessageFormatter(conf.Fields, conf.Timestamp)
	default:
		o.messageFormatter = NewRawMessageFormatter()
	}
	o.timestamp = conf.Timestamp
	if serverUrl, err := url.Parse(conf.Server); err == nil {
		switch strings.ToLower(serverUrl.Scheme) {
		case "http", "https":
			o.bulkIndexer = NewHttpBulkIndexer(strings.ToLower(serverUrl.Scheme), serverUrl.Host, o.flushCount)
		case "udp":
			o.bulkIndexer = NewUDPBulkIndexer(serverUrl.Host, o.flushCount)
		}
	} else {
		err = fmt.Errorf("Unable to parse ElasticSearch server URL [%s]: %s", conf.Server, err)
		return err
	}

	return
}

func (o *ElasticSearchOutput) Run(or OutputRunner, h PluginHelper) (err error) {
	var wg sync.WaitGroup
	wg.Add(2)
	go o.receiver(or, &wg)
	go o.committer(&wg)
	wg.Wait()
	return
}

// Runs in a separate goroutine, accepting incoming messages, buffering output
// data until the ticker triggers the buffered data should be put onto the
// committer channel.
func (o *ElasticSearchOutput) receiver(or OutputRunner, wg *sync.WaitGroup) {
	var pack *PipelinePack
	var e error
	var count int
	ok := true
	ticker := time.Tick(time.Duration(o.flushInterval) * time.Millisecond)
	outBatch := make([]byte, 0, 10000)
	outBytes := make([]byte, 0, 10000)
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
			// `handleMessage()` method recycles the pack.
			if e = o.handleMessage(pack, &outBytes); e != nil {
				or.LogError(e)
			} else {
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
			outBytes = outBytes[:0]
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

// ElasticSearchCoordinates stores the coordinates (_index, _type, _id)
// of an ElasticSearch document
type ElasticSearchCoordinates struct {
	Index           string
	Type            string
	Id              string
	Timestamp       *int64
	TimestampFormat string
}

func (e *ElasticSearchCoordinates) String() string {
	return string(e.Bytes())
}

// Renders the coordinates of the ElasticSearch document as JSON
func (e *ElasticSearchCoordinates) Bytes() []byte {
	buf := bytes.Buffer{}
	buf.WriteString(`{"index":{"_index":`)
	buf.WriteString(strconv.Quote(cleanIndexName(e.Index)))
	buf.WriteString(`,"_type":`)
	buf.WriteString(strconv.Quote(e.Type))
	if len(e.Id) > 0 {
		buf.WriteString(`,"_id":`)
		buf.WriteString(strconv.Quote(e.Id))
	}
	if e != nil && e.Timestamp != nil {
		t := time.Unix(0, *e.Timestamp)
		buf.WriteString(`,"_timestamp":"`)
		buf.WriteString(t.Format(e.TimestampFormat))
		buf.WriteString(`"`)
	}
	buf.WriteString(`}}`)
	return buf.Bytes()
}

// A Message Formatter formats a Heka message in JSON ([]byte)
// Replace it by client.Encoder ?
type MessageFormatter interface {
	// Formats a Heka message in JSON
	Format(*message.Message) (doc []byte, err error)
}

// Raw message formatter leaves the Heka message untouched
type RawMessageFormatter struct {
}

func NewRawMessageFormatter() *RawMessageFormatter {
	return &RawMessageFormatter{}
}

func (r *RawMessageFormatter) Format(m *message.Message) (doc []byte, err error) {
	return json.Marshal(m)
}

// Clean message formatter reformats the Heka message in a
// more friendly ElasticSearch/Kibana way
type CleanMessageFormatter struct {
	// Field names to include in ElasticSearch document for "clean" format
	fields          []string
	timestampFormat string
}

func NewCleanMessageFormatter(fields []string, timestampFormat string) *CleanMessageFormatter {
	if fields == nil || len(fields) == 0 {
		return &CleanMessageFormatter{
			fields: []string{
				"Uuid",
				"Timestamp",
				"Type",
				"Logger",
				"Severity",
				"Payload",
				"EnvVersion",
				"Pid",
				"Hostname",
				"Fields",
			},
			timestampFormat: timestampFormat,
		}
	} else {
		return &CleanMessageFormatter{fields: fields, timestampFormat: timestampFormat}
	}
}

// Append a field (with a name and a value) to a Buffer
func writeField(b *bytes.Buffer, name string, value string) {
	if b.Len() > 1 {
		b.WriteString(`,`)
	}
	b.WriteString(`"`)
	b.WriteString(name)
	b.WriteString(`":`)
	b.WriteString(value)
}

func (c *CleanMessageFormatter) Format(m *message.Message) (doc []byte, err error) {
	buf := bytes.Buffer{}
	buf.WriteString(`{`)
	// Iterates over fields configured for clean formating
	for _, f := range c.fields {
		switch strings.ToLower(f) {
		case "uuid":
			writeField(&buf, f, strconv.Quote(m.GetUuidString()))
		case "timestamp":
			t := time.Unix(0, m.GetTimestamp())
			writeField(&buf, f, strconv.Quote(t.Format(c.timestampFormat)))
		case "type":
			writeField(&buf, f, strconv.Quote(m.GetType()))
		case "logger":
			writeField(&buf, f, strconv.Quote(m.GetLogger()))
		case "severity":
			writeField(&buf, f, strconv.Itoa(int(m.GetSeverity())))
		case "payload":
			if utf8.ValidString(m.GetPayload()) {
				writeField(&buf, f, strconv.Quote(m.GetPayload()))
			}
		case "envversion":
			writeField(&buf, f, strconv.Quote(m.GetEnvVersion()))
		case "pid":
			writeField(&buf, f, strconv.Itoa(int(m.GetPid())))
		case "hostname":
			writeField(&buf, f, strconv.Quote(m.GetHostname()))
		case "fields":
			for _, field := range m.Fields {
				switch field.GetValueType() {
				case message.Field_STRING:
					writeField(&buf, *field.Name, strconv.Quote(field.GetValue().(string)))
				case message.Field_BYTES:
					data := field.GetValue().([]byte)[:]
					writeField(&buf, *field.Name, strconv.Quote(base64.StdEncoding.EncodeToString(data)))
				case message.Field_INTEGER:
					writeField(&buf, *field.Name, strconv.FormatInt(field.GetValue().(int64), 10))
				case message.Field_DOUBLE:
					writeField(&buf, *field.Name, strconv.FormatFloat(field.GetValue().(float64), 'g', -1, 64))
				case message.Field_BOOL:
					writeField(&buf, *field.Name, strconv.FormatBool(field.GetValue().(bool)))
				}
			}
		default:
			// Search fo a given fields in the message
			err = fmt.Errorf("Unable to find field: %s", f)
			return
		}
	}
	buf.WriteString(`}`)
	doc = buf.Bytes()
	return
}

// Performs the actual task of extracting data from the pack and writing it
// into the output buffer.
func (o *ElasticSearchOutput) handleMessage(pack *PipelinePack, outBytes *[]byte) (err error) {

	// Builds ElasticSearch document coordinates (1st line of bulk indexing)
	coordinates := &ElasticSearchCoordinates{
		Index:           o.indexName,
		Type:            o.typeName,
		Timestamp:       pack.Message.Timestamp,
		TimestampFormat: o.timestamp,
	}

	var document []byte
	document, err = o.messageFormatter.Format(pack.Message)
	pack.Recycle()
	if err != nil {
		err = fmt.Errorf("Error in message conversion to %s format: %s", o.format, err)
		return
	}

	// Write new bulk lines
	*outBytes = append(*outBytes, coordinates.Bytes()...)
	*outBytes = append(*outBytes, NEWLINE)
	*outBytes = append(*outBytes, document...)
	*outBytes = append(*outBytes, NEWLINE)

	document = document[:0]
	return
}

// Runs in a separate goroutine, waits for buffered data on the committer
// channel, bulk index it out to the elasticsearch cluster, and puts the now empty buffer on
// the return channel for reuse.
func (o *ElasticSearchOutput) committer(wg *sync.WaitGroup) {
	initBatch := make([]byte, 0, 10000)
	o.backChan <- initBatch
	var outBatch []byte

	for outBatch = range o.batchChan {
		o.bulkIndexer.Index(outBatch)
		outBatch = outBatch[:0]
		o.backChan <- outBatch
	}
	wg.Done()
}

// Replaces a date pattern (ex: %{2012.09.19} in the index name
func cleanIndexName(name string) (index string) {
	start := strings.Index(name, "%{")
	end := strings.Index(name, "}")

	if start > -1 && end > -1 {
		layout := name[start+len("%{") : end]
		index = name[:start] + time.Now().Format(layout)
	} else {
		index = name
	}
	return
}

// A BulkIndexer is used to index documents in ElasticSearch
type BulkIndexer interface {
	// Index documents
	Index(body []byte) (success bool, err error)
	// Check if a flush is needed
	CheckFlush(count int, length int) bool
}

// A HttpBulkIndexer uses the HTTP REST Bulk Api of ElasticSearch
// in order to index documents
type HttpBulkIndexer struct {
	// Protocol (http or https)
	Protocol string
	// Host name and port number (default to "localhost:9200")
	Domain string
	// Maximum number of documents
	MaxCount int
	// Internal HTTP Client
	client *http.Client
}

func NewHttpBulkIndexer(protocol string, domain string, maxCount int) *HttpBulkIndexer {
	return &HttpBulkIndexer{Protocol: protocol, Domain: domain, MaxCount: maxCount}
}

func (h *HttpBulkIndexer) CheckFlush(count int, length int) bool {
	if count >= h.MaxCount {
		return true
	}
	return false
}

func (h *HttpBulkIndexer) Index(body []byte) (success bool, err error) {
	if h.client == nil {
		h.client = &http.Client{}
	}
	url := fmt.Sprintf("%s://%s%s", h.Protocol, h.Domain, "/_bulk")

	// Creating ElasticSearch Bulk HTTP request
	if request, err := http.NewRequest("POST", url, bytes.NewReader(body)); err != nil {
		err = fmt.Errorf("Error creating bulk request: %s", err)
		return false, err
	} else {
		request.Header.Add("Accept", "application/json")
		response, err := h.client.Do(request)
		if err != nil {
			err = fmt.Errorf("Error executing bulk request: %s", err)
			return false, err
		}
		if response != nil {
			defer response.Body.Close()
			if response.StatusCode > 304 {
				err = fmt.Errorf("Bulk response in error: %s", response.Status)
				return false, err
			}
			if _, err = ioutil.ReadAll(response.Body); err != nil {
				err = fmt.Errorf("Bulk bulk response reading in error: %s", err)
				return false, err
			}
		}
	}
	return true, nil
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

func (u *UDPBulkIndexer) Index(body []byte) (success bool, err error) {
	if u.address == nil {
		if u.address, err = net.ResolveUDPAddr("udp", u.Domain); err != nil {
			err = fmt.Errorf("Error resolving UDP address [%s]: %s", u.Domain, err)
			return false, err
		}
	}
	if u.client == nil {
		if u.client, err = net.DialUDP("udp", nil, u.address); err != nil {
			err = fmt.Errorf("Error creating UDP client: %s", err)
			return false, err
		}
	}
	if u.address != nil {
		if _, err = u.client.Write(body[:]); err != nil {
			err = fmt.Errorf("Error writing data to UDP server: %s", err)
			return false, err
		}
	} else {
		err = fmt.Errorf("Error writing data to UDP server, address not found")
		return false, err
	}
	return true, nil
}
