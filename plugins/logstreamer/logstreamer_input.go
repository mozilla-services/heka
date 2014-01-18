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
#   Ben Bangert (bbangert@mozilla.com)
#
# ***** END LICENSE BLOCK *****/

package logstreamer

import (
	"fmt"
	ls "github.com/mozilla-services/heka/logstreamer"
	p "github.com/mozilla-services/heka/pipeline"
	"time"
)

type LogstreamerInputConfig struct {
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
	Translation logstreamer.SubmatchTranslationMap
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
	rescanInterval     time.Duration
	plugins            map[string]*LogstreamInput
	stopLogstreamChans []chan bool
	stopChan           chan bool
	decoderName        string
	parser             string
	delimiter          string
	delimiterLocation  string
}

func (li *LogstreamerInput) ConfigStruct() interface{} {
	return &LogstreamerInputConfig{
		RescanInterval: "1m",
		parser:         "token",
	}
}

func (li *LogstreamerInput) Init(config interface{}) (err error) {
	conf := config.(*LogstreamerInputConfig)
	sp := &ls.SortPattern{
		FileMatch:      conf.FileMatch,
		Translation:    conf.Translation,
		Priority:       conf.Priority,
		Differentiator: conf.Differentiator,
	}
	var (
		oldest  time.Duration
		rescan  time.Duration
		plugins []string
	)
	li.decoderName = conf.Decoder
	li.parser = conf.ParserType
	li.delimiter = conf.Delimiter
	li.delimiterLocation = conf.DelimiterLocation

	oldest, err = time.ParseDuration(conf.OldestDuration)
	if err != nil {
		return
	}
	rescan, err = time.ParseDuration(conf.RescanInterval)
	if err != nil {
		return
	}
	li.rescanInterval = rescan
	li.logstreamSet = ls.NewLogstreamSet(sp, oldest, conf.LogDirectory, conf.JournalDirectory)
	plugins, err = li.logstreamSet.ScanForLogstreams()
	if err != nil {
		return
	}
	// Create all our initial logstream plugins for the logstreams found
	for _, name := range plugins {
		stream, ok := li.logstreamSet.GetLogstream(name)
		if !ok {
			continue
		}
		li.plugins[name] = NewLogstreamInput(stream)
	}
	li.stopLogstreamChans = make([]chan bool, 0, len(plugins))
	li.stopChan = make(chan bool)
}

func (li *LogstreamerInput) Run(ir p.InputRunner, h p.PluginHelper) (err error) {
	var (
		dRunner p.DecoderRunner
	)
	if li.decoderName != "" {
		if dRunner, ok = h.DecoderRunner(li.decoderName); !ok {
			return fmt.Errorf("Decoder not found: %s", li.decoderName)
		}
	}

	for name, logstream := range li.plugins {
		stop := make(chan bool, 1)
		go logstream.Run(ir, h, stop, dRunner)
		li.stopLogstreamChans = append(li.stopLogstreamChans, stop)
	}
	ok = true
	rescan = time.Tick(li.rescanInterval)
	for ok {
		select {
		case <-li.stopChan:
			ok = false
			// Send out all the stop signals
			for _, ch := range li.stopLogstreamChans {
				ch <- true
			}
			// Wait for all the stops
			for _, ch := range li.stopLogstreamChans {
				<-ch
			}

			// Close our own stopChan to indicate we shut down
			li.stopChan.Close()
		case <-rescan:
			newstreams, errs := li.logstreamSet.ScanForLogstreams()
			if errs != nil {
				ir.LogError(errs)
			}
			for _, name := range newstreams {
				stream, ok := li.logstreamSet.GetLogstream(name)
				if !ok {
					ir.LogError(fmt.Errorf("Found new logstream: %s, but couldn't fetch it.", name))
					continue
				}

				// Setup a new logstream and start it running
				lsi := NewLogstreamInput(stream)
				li.plugins[name] = lsi
				stop := make(chan bool, 1)
				go lsi.Run(ir, h, stop, dRunner)
				li.stopLogstreamChans = append(li.stopLogstreamChans, stop)
			}
		}
	}
	err = nil
	return
}

func (li *LogstreamerInput) Stop() {
	li.stopChan <- true
	<-li.stopChan
}

type ParseFunction func(ls *LogstreamInput) (err error)

type LogstreamInput struct {
	stream        *ls.Logstream
	shutdownChan  chan<- bool
	parser        p.StreamParser
	parseFunction ParseFunction
}

func NewLogstreamInput(stream *ls.Logstream) *LogstreamInput {
	return &LogstreamInput{
		stream:       stream,
		shutdownChan: make(chan bool),
	}
}

func (ls *LogstreamInput) Run(ir p.InputRunner, h p.PluginHelper, stopChan chan bool,
	dRunner p.DecoderRunner) {
	deliver := func(pack *p.PipelinePack) {
		if dRunner == nil {
			ir.Inject(pack)
		} else {
			dRunner.InChan() <- pack
		}
	}

	tick := time.Tick(time.ParseDuration("500ms"))
	ok := true
	for ok {
		// Attempt to read as many as we can

		// Wait for our next interval
		select {
		case <-stopChan:
			ok = false
		case <-tick:
			continue
		}
	}
	stopChan.Close()
}

func CreateParser(parserType, delimiter, delimiterLocation string) (parser p.StreamParser,
	parseFunction ParseFunction, err error) {
	parseFunction = payloadParser
	if parserType == "" || parserType == "token" {
		parser = p.NewTokenParser()
		switch len(delimiter) {
		case 0: // use default
		case 1:
			parser.SetDelimiter(delimiter[0])
		default:
			err = fmt.Errorf("invalid delimiter: %s", delimiter)
		}
	} else if parserType == "regexp" {
		parser = p.NewRegexpParser()
		if len(delimiter) > 0 {
			err = parser.SetDelimiter(delimiter)
		}
		err = parser.SetDelimiterLocation(delimiterLocation)
	} else if parserType == "message.proto" {
		parser = p.NewMessageProtoParser()
		parseFunction = messageProtoParser
		if conf.Decoder == "" {
			err = fmt.Errorf("The message.proto parser must have a decoder")
		}
	} else {
		err = fmt.Errorf("unknown parser type: %s", parserType)
	}
	return
}

// Standard text log file parser
func payloadParser(fm *FileMonitor, isRotated bool) (bytesRead int64, err error) {
	var (
		n      int
		pack   *PipelinePack
		record []byte
	)
	for err == nil {
		n, record, err = fm.parser.Parse(fm.fd)
		if err != nil {
			if err == io.EOF && isRotated {
				record = fm.parser.GetRemainingData()
			} else if err == io.ErrShortBuffer {
				fm.ir.LogError(fmt.Errorf("record exceeded MAX_RECORD_SIZE %d", message.MAX_RECORD_SIZE))
				err = nil // non-fatal, keep going
			}
		}
		if len(record) > 0 {
			payload := string(record)
			pack = <-fm.ir.InChan()
			pack.Message.SetUuid(uuid.NewRandom())
			pack.Message.SetTimestamp(time.Now().UnixNano())
			pack.Message.SetType("logfile")
			pack.Message.SetSeverity(int32(0))
			pack.Message.SetEnvVersion("0.8")
			pack.Message.SetPid(0)
			pack.Message.SetHostname(fm.hostname)
			pack.Message.SetLogger(fm.logger_ident)
			pack.Message.SetPayload(payload)
			fm.outChan <- pack
			fm.last_logline_start = fm.seek + bytesRead
			fm.last_logline = payload
		}
		bytesRead += int64(n)
	}
	return
}

// Framed protobuf message parser
func messageProtoParser(fm *FileMonitor, isRotated bool) (bytesRead int64, err error) {
	var (
		n      int
		pack   *PipelinePack
		record []byte
	)
	for err == nil {
		n, record, err = fm.parser.Parse(fm.fd)
		if len(record) > 0 {
			pack = <-fm.ir.InChan()
			headerLen := int(record[1]) + 3 // recsep+len+header+unitsep
			messageLen := len(record) - headerLen
			// ignore authentication headers
			if messageLen > cap(pack.MsgBytes) {
				pack.MsgBytes = make([]byte, messageLen)
			}
			pack.MsgBytes = pack.MsgBytes[:messageLen]
			copy(pack.MsgBytes, record[headerLen:])
			fm.outChan <- pack
			fm.last_logline_start = fm.seek + bytesRead
			fm.last_logline = string(record)
		}
		bytesRead += int64(n)
	}
	return
}

func init() {
	RegisterPlugin("LogstreamerInput", func() interface{} {
		return new(LogstreamerInput)
	})
}
