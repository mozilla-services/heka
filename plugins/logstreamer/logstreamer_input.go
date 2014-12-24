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
	"errors"
	"fmt"
	ls "github.com/mozilla-services/heka/logstreamer"
	"github.com/mozilla-services/heka/message"
	p "github.com/mozilla-services/heka/pipeline"
	"io"
	"os"
	"path/filepath"
	"strconv"
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
	// Type of parser used to break the log file up into messages
	ParserType string `toml:"parser_type"`
	// Delimiter used to split the log stream into log messages
	Delimiter string
	// String indicating if the delimiter is at the start or end of the line,
	// only used for regexp delimiters
	DelimiterLocation string `toml:"delimiter_location"`
	// Whether truncate message exceeding buffer size instead of dropping it
	KeepTruncatedMessages bool `toml:"keep_truncated_messages"`
}

type LogstreamerInput struct {
	pConfig               *p.PipelineConfig
	logstreamSet          *ls.LogstreamSet
	logstreamSetLock      sync.RWMutex
	rescanInterval        time.Duration
	plugins               map[string]*LogstreamInput
	stopLogstreamChans    []chan chan bool
	stopChan              chan bool
	parser                string
	delimiter             string
	delimiterLocation     string
	hostName              string
	pluginName            string
	keepTruncatedMessages bool
}

// Heka will call this before calling any other methods to give us access to
// the pipeline configuration.
func (li *LogstreamerInput) SetPipelineConfig(pConfig *p.PipelineConfig) {
	li.pConfig = pConfig
}

func (li *LogstreamerInput) ConfigStruct() interface{} {
	baseDir := li.pConfig.Globals.BaseDir
	return &LogstreamerInputConfig{
		RescanInterval:   "1m",
		ParserType:       "token",
		OldestDuration:   "720h",
		LogDirectory:     "/var/log",
		JournalDirectory: filepath.Join(baseDir, "logstreamer"),
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

	// Setup the journal dir.
	if err = os.MkdirAll(conf.JournalDirectory, 0744); err != nil {
		return err
	}

	if conf.FileMatch == "" {
		return errors.New("`file_match` setting is required.")
	}
	if len(conf.FileMatch) > 0 && conf.FileMatch[len(conf.FileMatch)-1:] != "$" {
		conf.FileMatch += "$"
	}

	li.parser = conf.ParserType
	li.delimiter = conf.Delimiter
	li.delimiterLocation = conf.DelimiterLocation
	li.plugins = make(map[string]*LogstreamInput)

	// Setup the rescan interval.
	if li.rescanInterval, err = time.ParseDuration(conf.RescanInterval); err != nil {
		return
	}
	// Parse the oldest duration.
	if oldest, err = time.ParseDuration(conf.OldestDuration); err != nil {
		return
	}
	// If no differentiator is present then we use the plugin name.
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
	li.logstreamSet, err = ls.NewLogstreamSet(sp, oldest, conf.LogDirectory,
		conf.JournalDirectory)
	if err != nil {
		return
	}
	// Initial scan for logstreams
	plugins, errs = li.logstreamSet.ScanForLogstreams()
	if errs.IsError() {
		return errs
	}
	// Verify we can make a parser
	_, _, err = CreateParser(li.parser, li.delimiter, li.delimiterLocation)
	if err != nil {
		return
	}
	// Declare our hostname
	if conf.Hostname == "" {
		li.hostName = li.pConfig.Hostname()
	} else {
		li.hostName = conf.Hostname
	}

	li.keepTruncatedMessages = conf.KeepTruncatedMessages

	// Create all our initial logstream plugins for the logstreams found
	for _, name := range plugins {
		stream, ok := li.logstreamSet.GetLogstream(name)
		if !ok {
			continue
		}
		stParser, parserFunc, _ := CreateParser(li.parser, li.delimiter,
			li.delimiterLocation)
		li.plugins[name] = NewLogstreamInput(stream, stParser, parserFunc,
			name, li.hostName, li.keepTruncatedMessages)
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
		errs       *ls.MultipleError
		newstreams []string
	)

	// message.proto parser needs ProtobufDecoder.
	if li.parser == "message.proto" && !ir.UseMsgBytes() {
		// stopChan dance needed to prevent hang on shutdown.
		go func() {
			<-li.stopChan
			close(li.stopChan)
		}()
		return errors.New("`message.proto` parser_type requires ProtobufDecoder")
	}

	// Kick off all the current logstreams we know of
	i := 0
	for _, logstream := range li.plugins {
		i++
		stop := make(chan chan bool, 1)
		deliverer := ir.NewDeliverer(strconv.Itoa(i))
		go logstream.Run(ir, h, stop, deliverer)
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
					li.delimiterLocation)

				lsi := NewLogstreamInput(stream, stParser, parserFunc, name,
					li.hostName, li.keepTruncatedMessages)
				li.plugins[name] = lsi
				stop := make(chan chan bool, 1)
				i++
				deliverer := ir.NewDeliverer(strconv.Itoa(i))
				go lsi.Run(ir, h, stop, deliverer)
				li.stopLogstreamChans = append(li.stopLogstreamChans, stop)
			}
			li.logstreamSetLock.Unlock()
		}
	}
	return nil
}

func (li *LogstreamerInput) Stop() {
	li.stopChan <- true
	<-li.stopChan
}

type LogstreamInput struct {
	stream                *ls.Logstream
	parser                p.StreamParser
	parseFunction         string
	loggerIdent           string
	hostName              string
	recordCount           int
	stopped               chan bool
	keepTruncatedMessages bool
	prevMsgWasTruncated   bool
}

func NewLogstreamInput(stream *ls.Logstream, parser p.StreamParser, parserFunction,
	loggerIdent, hostName string, keepTruncatedMessages bool) *LogstreamInput {
	return &LogstreamInput{
		stream:                stream,
		parser:                parser,
		parseFunction:         parserFunction,
		loggerIdent:           loggerIdent,
		hostName:              hostName,
		prevMsgWasTruncated:   false,
		keepTruncatedMessages: keepTruncatedMessages,
	}
}

func (lsi *LogstreamInput) Run(ir p.InputRunner, h p.PluginHelper, stopChan chan chan bool,
	deliverer p.Deliverer) {

	var (
		parser func(ir p.InputRunner, deliver p.DeliverFunc, stop chan chan bool) error
		err    error
	)

	if lsi.parseFunction == "payload" {
		parser = lsi.payloadParser
	} else if lsi.parseFunction == "messageProto" {
		parser = lsi.messageProtoParser
	}

	// Check for more data interval
	interval, _ := time.ParseDuration("250ms")
	tick := time.Tick(interval)

	ok := true
	for ok {
		// Clear our error
		err = nil

		// Attempt to read as many as we can
		err = parser(ir, deliverer.DeliverFunc(), stopChan)

		// Save our position if the stream hasn't done so for us.
		if err != io.EOF {
			lsi.stream.SavePosition()
		}
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
	deliverer.Done()
}

// Standard text log file parser
func (lsi *LogstreamInput) payloadParser(ir p.InputRunner, deliver p.DeliverFunc,
	stop chan chan bool) (err error) {

	var (
		pack   *p.PipelinePack
		record []byte
		n      int
	)
	inChan := ir.InChan()
	for err == nil {
		select {
		case lsi.stopped = <-stop:
			return
		default:
		}
		is_message_truncated := false
		n, record, err = lsi.parser.Parse(lsi.stream)
		if err == io.ErrShortBuffer {
			err = nil // non-fatal, keep going
			if lsi.keepTruncatedMessages == true {
				ir.LogError(fmt.Errorf("record exceeded MAX_RECORD_SIZE %d and was truncated", message.MAX_RECORD_SIZE))
			} else {
				ir.LogError(fmt.Errorf("record exceeded MAX_RECORD_SIZE %d and was dropped", message.MAX_RECORD_SIZE))
			}
			is_message_truncated = true
		}
		if n > 0 {
			lsi.stream.FlushBuffer(n)
		}
		if len(record) > 0 {
			if lsi.prevMsgWasTruncated == false {
				// pack message only if previous record had normal size
				if is_message_truncated == false || lsi.keepTruncatedMessages == true {
					// pack message if it's not truncated or it is truncated and force keeping is set
					payload := string(record)
					pack = <-inChan
					pack.Message.SetUuid(uuid.NewRandom())
					pack.Message.SetTimestamp(time.Now().UnixNano())
					pack.Message.SetType("logfile")
					pack.Message.SetHostname(lsi.hostName)
					pack.Message.SetLogger(lsi.loggerIdent)
					pack.Message.SetPayload(payload)
					deliver(pack)
					lsi.countRecord()
				}
			} // any part of big message (fist, possible next big one or tail) are ignored
		}
		lsi.prevMsgWasTruncated = is_message_truncated
	}
	return
}

// Framed protobuf message parser
func (lsi *LogstreamInput) messageProtoParser(ir p.InputRunner, deliver p.DeliverFunc,
	stop chan chan bool) (err error) {

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

func CreateParser(parserType, delimiter, delimiterLocation string) (parser p.StreamParser,
	parseFunction string, err error) {

	parseFunction = "payload"

	switch parserType {
	case "", "token":
		tp := p.NewTokenParser()
		switch len(delimiter) {
		case 0: // use default
		case 1:
			tp.SetDelimiter(delimiter[0])
		default:
			err = fmt.Errorf("invalid delimiter: %s", delimiter)
		}
		parser = tp
	case "regexp":
		rp := p.NewRegexpParser()
		if len(delimiter) > 0 {
			err = rp.SetDelimiter(delimiter)
		}
		err = rp.SetDelimiterLocation(delimiterLocation)
		parser = rp
	case "message.proto":
		parser = p.NewMessageProtoParser()
		parseFunction = "messageProto"
	default:
		err = fmt.Errorf("unknown parser type: %s", parserType)
	}
	return parser, parseFunction, err
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
