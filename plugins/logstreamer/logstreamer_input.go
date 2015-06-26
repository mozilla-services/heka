/***** BEGIN LICENSE BLOCK *****
# This Source Code Form is subject to the terms of the Mozilla Public
# License, v. 2.0. If a copy of the MPL was not distributed with this file,
# You can obtain one at http://mozilla.org/MPL/2.0/.
#
# The Initial Developer of the Original Code is the Mozilla Foundation.
# Portions created by the Initial Developer are Copyright (C) 2014-2015
# the Initial Developer. All Rights Reserved.
#
# Contributor(s):
#   Ben Bangert (bbangert@mozilla.com)
#   Rob Miller (rmiller@mozilla.com)
#
# ***** END LICENSE BLOCK *****/
package logstreamer

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	ls "github.com/mozilla-services/heka/logstreamer"
	"github.com/mozilla-services/heka/message"
	p "github.com/mozilla-services/heka/pipeline"
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
	// So we can default to TokenSplitter.
	Splitter string
}

type LogstreamerInput struct {
	pConfig            *p.PipelineConfig
	logstreamSet       *ls.LogstreamSet
	logstreamSetLock   sync.RWMutex
	rescanInterval     time.Duration
	plugins            map[string]*LogstreamInput
	stopLogstreamChans []chan chan bool
	stopChan           chan bool
	parser             string
	delimiter          string
	delimiterLocation  string
	hostName           string
	pluginName         string
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
		OldestDuration:   "720h",
		LogDirectory:     "/var/log",
		JournalDirectory: filepath.Join(baseDir, "logstreamer"),
		Splitter:         "TokenSplitter",
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
	// Declare our hostname
	if conf.Hostname == "" {
		li.hostName = li.pConfig.Hostname()
	} else {
		li.hostName = conf.Hostname
	}

	// Create all our initial logstream plugins for the logstreams found
	for _, name := range plugins {
		stream, ok := li.logstreamSet.GetLogstream(name)
		if !ok {
			continue
		}
		li.plugins[name] = NewLogstreamInput(stream, name, li.hostName)
	}
	li.stopLogstreamChans = make([]chan chan bool, 0, len(plugins))
	li.stopChan = make(chan bool)
	return
}

// Creates deliverer and stop channel and starts the provided LogstreamInput.
func (li *LogstreamerInput) startLogstreamInput(logstream *LogstreamInput, i int,
	ir p.InputRunner, h p.PluginHelper) {

	stop := make(chan chan bool, 1)
	token := strconv.Itoa(i)
	deliverer := ir.NewDeliverer(token)
	sRunner := ir.NewSplitterRunner(token)
	li.stopLogstreamChans = append(li.stopLogstreamChans, stop)
	go logstream.Run(ir, h, stop, deliverer, sRunner)
}

// Main Logstreamer Input runner. This runner kicks off all the other
// logstream inputs, and handles rescanning for updates to the filesystem that
// might affect file visibility for the logstream inputs.
func (li *LogstreamerInput) Run(ir p.InputRunner, h p.PluginHelper) (err error) {
	var (
		ok         bool
		errs       *ls.MultipleError
		newstreams []string
	)

	// Kick off all the current logstreams we know of
	i := 0
	for _, logstream := range li.plugins {
		i++
		li.startLogstreamInput(logstream, i, ir, h)
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
					ir.LogError(fmt.Errorf("Found new logstream: %s, but couldn't fetch it.",
						name))
					continue
				}

				lsi := NewLogstreamInput(stream, name, li.hostName)
				li.plugins[name] = lsi
				i++
				li.startLogstreamInput(lsi, i, ir, h)
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
	stream              *ls.Logstream
	loggerIdent         string
	hostName            string
	recordCount         int
	stopped             chan bool
	prevMsgWasTruncated bool
	ir                  p.InputRunner
	stopChan            chan chan bool
	deliverer           p.Deliverer
	sRunner             p.SplitterRunner
}

func NewLogstreamInput(stream *ls.Logstream, loggerIdent,
	hostName string) *LogstreamInput {

	return &LogstreamInput{
		stream:              stream,
		loggerIdent:         loggerIdent,
		hostName:            hostName,
		prevMsgWasTruncated: false,
	}
}

func (lsi *LogstreamInput) Run(ir p.InputRunner, h p.PluginHelper, stopChan chan chan bool,
	deliverer p.Deliverer, sRunner p.SplitterRunner) {

	if !sRunner.UseMsgBytes() {
		sRunner.SetPackDecorator(lsi.packDecorator)
	}

	lsi.ir = ir
	lsi.stopChan = stopChan
	lsi.deliverer = deliverer
	lsi.sRunner = sRunner
	var err error

	// Check for more data interval
	interval, _ := time.ParseDuration("250ms")
	tick := time.Tick(interval)

	ok := true
	for ok {
		// Clear our error
		err = nil

		// Attempt to read and deliver as many as we can.
		err = lsi.deliverRecords()
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
	sRunner.Done()
}

func (lsi *LogstreamInput) deliverRecords() (err error) {
	var (
		record []byte
		n      int
	)
	for err == nil {
		select {
		case lsi.stopped = <-lsi.stopChan:
			return
		default:
		}
		isMessageTruncated := false
		n, record, err = lsi.sRunner.GetRecordFromStream(lsi.stream)
		if err == io.ErrShortBuffer {
			if lsi.sRunner.KeepTruncated() {
				err = fmt.Errorf("record exceeded MAX_RECORD_SIZE %d and was truncated",
					message.MAX_RECORD_SIZE)
			} else {
				err = fmt.Errorf("record exceeded MAX_RECORD_SIZE %d and was dropped",
					message.MAX_RECORD_SIZE)
			}
			lsi.ir.LogError(err)
			err = nil // non-fatal, keep going
			isMessageTruncated = true
		}
		if n > 0 {
			lsi.stream.FlushBuffer(n)
		}
		if len(record) > 0 {
			if lsi.prevMsgWasTruncated == false {
				// Send the message only if previous record had normal size.
				if !isMessageTruncated || lsi.sRunner.KeepTruncated() {
					lsi.sRunner.DeliverRecord(record, lsi.deliverer)
					lsi.countRecord()
				}
			}
		} else if err == io.EOF && lsi.sRunner.IncompleteFinal() {
			record = lsi.sRunner.GetRemainingData()
			if len(record) > 0 {
				lsi.sRunner.DeliverRecord(record, lsi.deliverer)
				lsi.countRecord()
			}
		}
		lsi.prevMsgWasTruncated = isMessageTruncated
	}
	return err
}

func (lsi *LogstreamInput) packDecorator(pack *p.PipelinePack) {
	pack.Message.SetType("logfile")
	pack.Message.SetHostname(lsi.hostName)
	pack.Message.SetLogger(lsi.loggerIdent)
}

func (lsi *LogstreamInput) countRecord() {
	lsi.recordCount += 1
	if lsi.recordCount > 500 {
		lsi.stream.SavePosition()
		lsi.recordCount = 0
	}
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

func init() {
	p.RegisterPlugin("LogstreamerInput", func() interface{} {
		return new(LogstreamerInput)
	})
}
