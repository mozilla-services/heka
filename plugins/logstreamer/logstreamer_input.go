/***** BEGIN LICENSE BLOCK *****
# This Source Code Form is subject to the terms of the Mozilla Public
# License, v. 2.0. If a copy of the MPL was not distributed with this file,
# You can obtain one at http://mozilla.org/MPL/2.0/.
#
# The Initial Developer of the Original Code is the Mozilla Foundation.
# Portions created by the Initial Developer are Copyright (C) 2014
# the Initial Developer. All Rights Reserved.
#
# Contributor(s):
#   Ben Bangert (bbangert@mozilla.com)
#   Rob Miller (rmiller@mozilla.com)
#
# ***** END LICENSE BLOCK *****/
package logstreamer

import (
	"code.google.com/p/go-uuid/uuid"
	"fmt"
	ls "github.com/mozilla-services/heka/logstreamer"
	"github.com/mozilla-services/heka/message"
	p "github.com/mozilla-services/heka/pipeline"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type LogstreamerInputConfig struct {
	// Hostname to use for the generated logfile message objects.
	Hostname string
	// Log base directory to run log regex under
	LogDirectory string `toml:"log_directory"`
	// Journal base directory for saving journal files
	JournalDirectory string `toml:"journal_directory"`
	// File match for regular expression
	FileMatch string `toml:"file_match"`
	// Priority to sort in
	Priority []string
	// Differentiator for splitting out logstreams if applicable
	Differentiator []string
	// Oldest logfiles to parse, as a duration parseable
	OldestDuration string `toml:"oldest_duration"`
	// Translation map for sorting substitutions
	Translation ls.SubmatchTranslationMap
	// Rescan interval declares how often the full directory scanner
	// runs to locate more logfiles/streams
	RescanInterval string `toml:"rescan_interval"`
	// Name of configured decoder instance.
	Decoder string
	// Type of parser used to break the log file up into messages
	ParserType string `toml:"parser_type"`
	// Delimiter used to split the log stream into log messages
	Delimiter string
	// String indicating if the delimiter is at the start or end of the line,
	// only used for regexp delimiters
	DelimiterLocation string `toml:"delimiter_location"`
}

type LogstreamerInput struct {
	logstreamSet       *ls.LogstreamSet
	logstreamSetLock   sync.RWMutex
	rescanInterval     time.Duration
	plugins            map[string]*LogstreamInput
	stopLogstreamChans []chan chan bool
	stopChan           chan bool
	decoderName        string
	parser             string
	delimiter          string
	delimiterLocation  string
	hostName           string
	pluginName         string
}

func (li *LogstreamerInput) ConfigStruct() interface{} {
	return &LogstreamerInputConfig{
		RescanInterval:   "1m",
		ParserType:       "token",
		OldestDuration:   "720h",
		LogDirectory:     "/var/log",
		JournalDirectory: filepath.Join(p.Globals().BaseDir, "logstreamer"),
	}
}

func (li *LogstreamerInput) SetName(name string) {
	li.pluginName = name
}

func (li *LogstreamerInput) Init(config interface{}) (err error) {
	var (
		errs    *ls.MultipleError
		oldest  time.Duration
		plugins []string
	)
	conf := config.(*LogstreamerInputConfig)

	// Setup the journal dir
	if err = os.MkdirAll(conf.JournalDirectory, 0744); err != nil {
		return err
	}

	if len(conf.FileMatch) > 0 && conf.FileMatch[len(conf.FileMatch)-1:] != "$" {
		conf.FileMatch += "$"
	}

	li.decoderName = conf.Decoder
	li.parser = conf.ParserType
	li.delimiter = conf.Delimiter
	li.delimiterLocation = conf.DelimiterLocation
	li.plugins = make(map[string]*LogstreamInput)

	// Setup the rescan interval
	if li.rescanInterval, err = time.ParseDuration(conf.RescanInterval); err != nil {
		return
	}

	// Parse the oldest duration
	if oldest, err = time.ParseDuration(conf.OldestDuration); err != nil {
		return
	}

	// If no differentiator is present than we use the plugin
	if len(conf.Differentiator) == 0 {
		conf.Differentiator = []string{li.pluginName}
	}

	for name, submap := range conf.Translation {
		if len(submap) == 1 {
			if _, ok := submap["missing"]; !ok {
				err = fmt.Errorf("A translation map with one entry ('%s') must be "+
					"specifying a 'missing' key.", name)
				return
			}
		}
	}

	// Create the main sort pattern
	sp := &ls.SortPattern{
		FileMatch:      conf.FileMatch,
		Translation:    conf.Translation,
		Priority:       conf.Priority,
		Differentiator: conf.Differentiator,
	}

	// Create the main logstream set
	li.logstreamSetLock.Lock()
	defer li.logstreamSetLock.Unlock()
	li.logstreamSet = ls.NewLogstreamSet(sp, oldest, conf.LogDirectory, conf.JournalDirectory)

	// Initial scan for logstreams
	plugins, errs = li.logstreamSet.ScanForLogstreams()
	if errs.IsError() {
		return errs
	}

	// Verify we can make a parser
	if _, _, err = CreateParser(li.parser, li.delimiter,
		li.delimiterLocation, li.decoderName); err != nil {
		return
	}

	// Declare our hostname
	if conf.Hostname == "" {
		li.hostName, err = os.Hostname()
		if err != nil {
			return
		}
	} else {
		li.hostName = conf.Hostname
	}

	// Create all our initial logstream plugins for the logstreams found
	for _, name := range plugins {
		stream, ok := li.logstreamSet.GetLogstream(name)
		if !ok {
			continue
		}
		stParser, parserFunc, _ := CreateParser(li.parser, li.delimiter,
			li.delimiterLocation, li.decoderName)
		li.plugins[name] = NewLogstreamInput(stream, stParser, parserFunc,
			name, li.hostName)
	}
	li.stopLogstreamChans = make([]chan chan bool, 0, len(plugins))
	li.stopChan = make(chan bool)
	return
}

// Main Logstreamer Input runner
// This runner kicks off all the other logstream inputs, and handles rescanning for
// updates to the filesystem that might affect file visibility for the logstream
// inputs
func (li *LogstreamerInput) Run(ir p.InputRunner, h p.PluginHelper) (err error) {
	var (
		ok         bool
		dRunner    p.DecoderRunner
		errs       *ls.MultipleError
		newstreams []string
	)

	// Setup the decoder runner that will be used
	if li.decoderName != "" {
		if dRunner, ok = h.DecoderRunner(li.decoderName, fmt.Sprintf("%s-%s", li.pluginName, li.decoderName)); !ok {
			return fmt.Errorf("Decoder not found: %s", li.decoderName)
		}
	}

	// Kick off all the current logstreams we know of
	for _, logstream := range li.plugins {
		stop := make(chan chan bool, 1)
		go logstream.Run(ir, h, stop, dRunner)
		li.stopLogstreamChans = append(li.stopLogstreamChans, stop)
	}

	ok = true
	rescan := time.Tick(li.rescanInterval)
	// Our main rescan loop that handles shutting down
	for ok {
		select {
		case <-li.stopChan:
			ok = false
			returnChans := make([]chan bool, len(li.stopLogstreamChans))
			// Send out all the stop signals
			for i, ch := range li.stopLogstreamChans {
				ret := make(chan bool)
				ch <- ret
				returnChans[i] = ret
			}

			// Wait for all the stops
			for _, ch := range returnChans {
				<-ch
			}

			// Close our own stopChan to indicate we shut down
			close(li.stopChan)
		case <-rescan:
			li.logstreamSetLock.Lock()
			newstreams, errs = li.logstreamSet.ScanForLogstreams()
			if errs.IsError() {
				ir.LogError(errs)
			}
			for _, name := range newstreams {
				stream, ok := li.logstreamSet.GetLogstream(name)
				if !ok {
					ir.LogError(fmt.Errorf("Found new logstream: %s, but couldn't fetch it.", name))
					continue
				}

				// Setup a new logstream input for this logstream and start it running
				stParser, parserFunc, _ := CreateParser(li.parser, li.delimiter,
					li.delimiterLocation, li.decoderName)

				lsi := NewLogstreamInput(stream, stParser, parserFunc, name,
					li.hostName)
				li.plugins[name] = lsi
				stop := make(chan chan bool, 1)
				go lsi.Run(ir, h, stop, dRunner)
				li.stopLogstreamChans = append(li.stopLogstreamChans, stop)
			}
			li.logstreamSetLock.Unlock()
		}
	}
	err = nil
	return
}

func (li *LogstreamerInput) Stop() {
	li.stopChan <- true
	<-li.stopChan
}

type Deliver func(pack *p.PipelinePack)

type LogstreamInput struct {
	stream        *ls.Logstream
	parser        p.StreamParser
	parseFunction string
	loggerIdent   string
	hostName      string
	recordCount   int
	stopped       chan bool
}

func NewLogstreamInput(stream *ls.Logstream, parser p.StreamParser, parserFunction,
	loggerIdent, hostName string) *LogstreamInput {
	return &LogstreamInput{
		stream:        stream,
		parser:        parser,
		parseFunction: parserFunction,
		loggerIdent:   loggerIdent,
		hostName:      hostName,
	}
}

func (lsi *LogstreamInput) Run(ir p.InputRunner, h p.PluginHelper, stopChan chan chan bool,
	dRunner p.DecoderRunner) {

	var (
		parser func(ir p.InputRunner, deliver Deliver, stop chan chan bool) error
		err    error
	)

	if lsi.parseFunction == "payload" {
		parser = lsi.payloadParser
	} else if lsi.parseFunction == "messageProto" {
		parser = lsi.messageProtoParser
	}

	// Setup our pack delivery function appropriately for the configuration
	deliver := func(pack *p.PipelinePack) {
		if dRunner == nil {
			ir.Inject(pack)
		} else {
			dRunner.InChan() <- pack
		}
	}

	// Check for more data interval
	interval, _ := time.ParseDuration("250ms")
	tick := time.Tick(interval)

	ok := true
	for ok {
		// Clear our error
		err = nil

		// Attempt to read as many as we can
		err = parser(ir, deliver, stopChan)

		// Save our location after reading as much as we can
		lsi.stream.SavePosition()
		lsi.recordCount = 0

		if err != nil && err != io.EOF {
			ir.LogError(err)
		}

		// Did our parser func get stopped?
		if lsi.stopped != nil {
			ok = false
			continue
		}

		// Wait for our next interval, stop if needed
		select {
		case lsi.stopped = <-stopChan:
			ok = false
		case <-tick:
			continue
		}
	}
	close(lsi.stopped)
}

// Standard text log file parser
func (lsi *LogstreamInput) payloadParser(ir p.InputRunner, deliver Deliver, stop chan chan bool) (err error) {
	var (
		pack   *p.PipelinePack
		record []byte
		n      int
	)
	for err == nil {
		select {
		case lsi.stopped = <-stop:
			return
		default:
		}
		n, record, err = lsi.parser.Parse(lsi.stream)
		if err == io.ErrShortBuffer {
			ir.LogError(fmt.Errorf("record exceeded MAX_RECORD_SIZE %d", message.MAX_RECORD_SIZE))
			err = nil // non-fatal, keep going
		}
		if n > 0 {
			lsi.stream.FlushBuffer(n)
		}
		if len(record) > 0 {
			payload := string(record)
			pack = <-ir.InChan()
			pack.Message.SetUuid(uuid.NewRandom())
			pack.Message.SetTimestamp(time.Now().UnixNano())
			pack.Message.SetType("logfile")
			pack.Message.SetHostname(lsi.hostName)
			pack.Message.SetLogger(lsi.loggerIdent)
			pack.Message.SetPayload(payload)
			deliver(pack)
			lsi.countRecord()
		}
	}
	return
}

// Framed protobuf message parser
func (lsi *LogstreamInput) messageProtoParser(ir p.InputRunner, deliver Deliver, stop chan chan bool) (err error) {
	var (
		pack   *p.PipelinePack
		record []byte
		n      int
	)
	for err == nil {
		select {
		case lsi.stopped = <-stop:
			return
		default:
		}
		n, record, err = lsi.parser.Parse(lsi.stream)
		if n > 0 {
			lsi.stream.FlushBuffer(n)
		}
		if len(record) > 0 {
			pack = <-ir.InChan()
			headerLen := int(record[1]) + 3 // recsep+len+header+unitsep
			messageLen := len(record) - headerLen
			// ignore authentication headers
			if messageLen > cap(pack.MsgBytes) {
				pack.MsgBytes = make([]byte, messageLen)
			}
			pack.MsgBytes = pack.MsgBytes[:messageLen]
			copy(pack.MsgBytes, record[headerLen:])
			deliver(pack)
			lsi.countRecord()
		}
	}
	return
}

func (lsi *LogstreamInput) countRecord() {
	lsi.recordCount += 1
	if lsi.recordCount > 500 {
		lsi.stream.SavePosition()
		lsi.recordCount = 0
	}
}

func CreateParser(parserType, delimiter, delimiterLocation, decoder string) (parser p.StreamParser,
	parseFunction string, err error) {
	parseFunction = "payload"
	if parserType == "" || parserType == "token" {
		tp := p.NewTokenParser()
		switch len(delimiter) {
		case 0: // use default
		case 1:
			tp.SetDelimiter(delimiter[0])
		default:
			err = fmt.Errorf("invalid delimiter: %s", delimiter)
		}
		parser = tp
	} else if parserType == "regexp" {
		rp := p.NewRegexpParser()
		if len(delimiter) > 0 {
			err = rp.SetDelimiter(delimiter)
		}
		err = rp.SetDelimiterLocation(delimiterLocation)
		parser = rp
	} else if parserType == "message.proto" {
		parser = p.NewMessageProtoParser()
		parseFunction = "messageProto"
		if decoder == "" {
			err = fmt.Errorf("The message.proto parser must have a decoder")
		}
	} else {
		err = fmt.Errorf("unknown parser type: %s", parserType)
	}
	return
}

func init() {
	p.RegisterPlugin("LogstreamerInput", func() interface{} {
		return new(LogstreamerInput)
	})
}

// ReportMsg provides plugin state to Heka report and dashboard.
func (li *LogstreamerInput) ReportMsg(msg *message.Message) error {
	li.logstreamSetLock.RLock()
	defer li.logstreamSetLock.RUnlock()

	for _, name := range li.logstreamSet.GetLogstreamNames() {
		logstream, ok := li.logstreamSet.GetLogstream(name)
		if ok {
			fname, bytes := logstream.ReportPosition()
			message.NewStringField(msg, fmt.Sprintf("%s-filename", name), fname)
			message.NewInt64Field(msg, fmt.Sprintf("%s-bytes", name), bytes, "count")
		}
	}
	return nil
}
