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
	"encoding/json"
	"fmt"
	"github.com/mozilla-services/heka/message"
	"github.com/rafrombrc/go-notify"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

var (
	httpClient = &http.Client{}
)

// Output plugin that index messages to an elasticsearch cluster.
// Largely based on FileOutput plugin.
type ElasticSearchOutput struct {
	clusterName   string
	indexName     string
	typeName      string
	flushInterval uint32
	batchChan     chan []byte
	backChan      chan []byte
	client        *http.Client
	format        string
	// The Message Formatter to use when converting
	// Heka messages to ElasticSearch documents
	messageFormatter MessageFormatter
	// ElasticSearch server URL
	// Defaults to "http://localhost:9200"
	server    string
	timestamp string
}

// ConfigStruct for ElasticSearchOutput plugin.
type ElasticSearchOutputConfig struct {
	// ElasticSearch cluster name.
	Cluster string
	// Name of the index in which the messages will be indexed.
	Index string
	// Name of the document type of the messages.
	TypeName string
	// Interval at which accumulated messages should be bulk indexed to ElasticSearch, in
	// milliseconds (default 5000, i.e. 5 seconds).
	FlushInterval uint32
	// Format of the document
	Format string
	// Field names to include in ElasticSearch document for "clean" format
	Fields []string
	// ElasticSearch server URL
	// Defaults to "http://localhost:9200"
	Server string
	// Timestamp format
	Timestamp string
}

func (o *ElasticSearchOutput) ConfigStruct() interface{} {
	return &ElasticSearchOutputConfig{
		Cluster:       "elasticsearch",
		Index:         "heka-%{2006.01.02}",
		TypeName:      "message",
		FlushInterval: 5000,
		Format:        "clean",
		Server:        "http://localhost:9200",
		Timestamp:     "2006-01-02T15:04:05.000Z",
	}
}

func (o *ElasticSearchOutput) Init(config interface{}) (err error) {
	conf := config.(*ElasticSearchOutputConfig)
	o.clusterName = conf.Cluster
	o.indexName = conf.Index
	o.typeName = conf.TypeName
	o.flushInterval = conf.FlushInterval
	o.batchChan = make(chan []byte)
	o.backChan = make(chan []byte, 2)
	o.client = new(http.Client)
	o.format = conf.Format
	switch strings.ToLower(conf.Format) {
	case "raw":
		o.messageFormatter = NewRawMessageFormatter()
	case "clean":
		o.messageFormatter = NewCleanMessageFormatter(conf.Fields, conf.Timestamp)
	default:
		o.messageFormatter = NewRawMessageFormatter()
	}
	o.server = conf.Server
	o.timestamp = conf.Timestamp
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
	var plc *PipelineCapture
	var e error
	ok := true
	ticker := time.Tick(time.Duration(o.flushInterval) * time.Millisecond)
	outBatch := make([]byte, 0, 10000)
	outBytes := make([]byte, 0, 10000)
	inChan := or.InChan()

	for ok {
		select {
		case plc, ok = <-inChan:
			if !ok {
				// Closed inChan => we're shutting down, flush data
				if len(outBatch) > 0 {
					o.batchChan <- outBatch
				}
				close(o.batchChan)
				break
			}
			if e = o.handleMessage(plc.Pack, &outBytes); e != nil {
				or.LogError(e)
			} else {
				outBatch = append(outBatch, outBytes...)
			}
			outBytes = outBytes[:0]
			plc.Pack.Recycle()
		case <-ticker:
			if len(outBatch) > 0 {
				// This will block until the other side is ready to accept
				// this batch, freeing us to start on the next one.
				o.batchChan <- outBatch
				outBatch = <-o.backChan
			}
		}
	}
	wg.Done()
}

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
			writeField(&buf, f, strconv.Quote(m.GetPayload()))
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
					//writeField(&buf, *field.Name, strconv.Quote(string(field.GetValue().([]byte)[:])))
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
	if document, err = o.messageFormatter.Format(pack.Message); err != nil {
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

	ok := true
	hupChan := make(chan interface{})
	notify.Start(RELOAD, hupChan)

	for ok {
		select {
		case outBatch, ok = <-o.batchChan:
			if !ok {
				// Channel is closed => we're shutting down, exit cleanly.
				break
			}
			doBulkRequest(o.server+"/_bulk", bytes.NewReader(outBatch))
			outBatch = outBatch[:0]
			o.backChan <- outBatch
		case <-hupChan:
			// Closing
			log.Printf("ElasticSearchOutput closed")
		}
	}
	wg.Done()
}

func doBulkRequest(url string, body io.Reader) {
	// Creating ElasticSearch Bulk request
	if request, err := http.NewRequest("POST", url, body); err != nil {
		log.Printf("ElasticSearchOutput error creating bulk request: %s", err)
	} else {
		request.Header.Add("Accept", "application/json")
		response, err := httpClient.Do(request)
		if err != nil {
			log.Printf("ElasticSearchOutput error executing bulk request: %s", err)
		}
		if response != nil {
			defer response.Body.Close()
		}
		if response.StatusCode > 304 {
			log.Printf("ElasticSearchOutput bulk response in error: %s", response.Status)
		}
		if _, err = ioutil.ReadAll(response.Body); err != nil {
			log.Printf("ElasticSearchOutput bulk response reading in error: %s", err)
		}
	}
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
