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
	DOCUMENTFORMATS = map[string]bool{
		"raw":   true,
		"clean": true,
	}

	CLUSTERNAME = "elasticsearch"
	INDEXNAME   = "heka-%{2006.01.02}"
	TYPENAME    = "message"

	DELIMITERSTART = "%{"
	DELIMITEREND   = "}"

	TIMESTAMPFORMAT = "2006-01-02T15:04:05.000Z"

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
	// Field names to include in ElasticSearch document for "clean" format
	fields []string
	// ElasticSearch server URL
	// Defaults to "http://localhost:9200"
	server string
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
	// milliseconds (default 1000, i.e. 1 second).
	FlushInterval uint32
	// Format of the document
	Format string
	// Field names to include in ElasticSearch document for "clean" format
	Fields []string
	// ElasticSearch server URL
	// Defaults to "http://localhost:9200"
	Server string
}

func (c ElasticSearchOutputConfig) String() string {
	return fmt.Sprintf(`ElasticSearchOutputConfig{Cluster: "%s", Index: "%s", TypeName: "%s", Format: "%s", FlushInterval: "%d"}`, c.Cluster, c.Index, c.TypeName, c.Format, c.FlushInterval)
}

func (o *ElasticSearchOutput) ConfigStruct() interface{} {
	return &ElasticSearchOutputConfig{Cluster: CLUSTERNAME, Index: INDEXNAME, TypeName: TYPENAME, FlushInterval: 5000, Format: "raw", Server: "http://localhost:9200"}
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
	o.fields = conf.Fields
	o.server = conf.Server
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
	Index     string
	Type      string
	Id        string
	Timestamp *int64
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
		buf.WriteString(t.Format(TIMESTAMPFORMAT))
		buf.WriteString(`"`)
	}
	buf.WriteString(`}}`)
	return buf.Bytes()
}

// Reformats a Message into a clean JSON object
func (o *ElasticSearchOutput) CleanFormatMarshal(m *message.Message) (doc []byte, err error) {
	buf := bytes.Buffer{}
	buf.WriteString(`{`)

	if o.fields == nil || len(o.fields) == 0 {
		err = fmt.Errorf("ElasticSearchOutput fields is empty")
		return
	}

	// Iterates over fields configured for clean formating
	for i, f := range o.fields {
		buf.WriteString(`"`)
		buf.WriteString(f)
		buf.WriteString(`":`)

		switch f {
		case "uuid":
			buf.WriteString(strconv.Quote(m.GetUuidString()))
		case "timestamp":
			t := time.Unix(0, m.GetTimestamp())
			buf.WriteString(strconv.Quote(t.Format(TIMESTAMPFORMAT)))
		case "type":
			buf.WriteString(strconv.Quote(m.GetType()))
		case "logger":
			buf.WriteString(strconv.Quote(m.GetLogger()))
		case "severity":
			buf.WriteString(strconv.Itoa(int(m.GetSeverity())))
		case "payload":
			buf.WriteString(strconv.Quote(m.GetPayload()))
		case "env_version":
			buf.WriteString(strconv.Quote(m.GetEnvVersion()))
		case "pid":
			buf.WriteString(strconv.Itoa(int(m.GetPid())))
		case "hostname":
			buf.WriteString(strconv.Quote(m.GetHostname()))
		default:
			err = fmt.Errorf("ElasticSearchOutput clean field not found: %s", f)
			return
		}
		if i < len(o.fields)-1 {
			buf.WriteString(`,`)
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
	coordinates := &ElasticSearchCoordinates{Index: o.indexName, Type: o.typeName, Timestamp: pack.Message.Timestamp}

	var document []byte
	switch o.format {
	case "raw":
		if document, err = json.Marshal(pack.Message); err != nil {
			err = fmt.Errorf("ElasticSearchOutput error in message conversion from %s format: %s", o.format, err)
		}
	case "clean":
		if document, err = o.CleanFormatMarshal(pack.Message); err != nil {
			err = fmt.Errorf("ElasticSearchOutput error in message conversion from %s format: %s", o.format, err)
		}
	default:
		err = fmt.Errorf("ElasticSearchOutput message conversion error: Invalid format %s", o.format)
	}

	if err == nil {
		// Write new bulk lines
		*outBytes = append(*outBytes, coordinates.Bytes()...)
		*outBytes = append(*outBytes, NEWLINE)
		*outBytes = append(*outBytes, document...)
		*outBytes = append(*outBytes, NEWLINE)
	} else {
		err = fmt.Errorf("ElasticSearchOutput error encoding document coordinates to JSON: %s", err)
	}
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
			go doBulkRequest(o.server+"/_bulk", bytes.NewReader(outBatch))
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
	start := strings.Index(name, DELIMITERSTART)
	end := strings.Index(name, DELIMITEREND)

	if start > -1 && end > -1 {
		layout := name[start+len(DELIMITERSTART) : end]
		index = name[:start] + time.Now().Format(layout)
	} else {
		index = name
	}
	return
}
