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
#   Rob Miller (rmiller@mozilla.com)
#   Mike Trinkala (trink@mozilla.com)
#
# ***** END LICENSE BLOCK *****/

package pipeline

import (
	"encoding/json"
	"fmt"
	"github.com/mozilla-services/heka/client"
	"github.com/mozilla-services/heka/message"
	"github.com/rafrombrc/go-notify"
	"log"
	"net"
	"os"
	"sync"
	"time"
)

// Heka PluginRunner for Output plugins.
type OutputRunner interface {
	PluginRunner
	// Input channel where Output should be listening for incoming messages,
	// along w/ any capture groups that may have been defined and matched in
	// the Output's message_matcher.
	InChan() chan *PipelineCapture
	// Associated Output plugin instance.
	Output() Output
	// Starts the Output plugin listening on the input channel in a separate
	// goroutine and returns. Wait group should be released when the Output
	// plugin shuts down cleanly and the goroutine has completed.
	Start(h PluginHelper, wg *sync.WaitGroup) (err error)
	// Returns a ticker channel configured to send ticks at an interval
	// specified by the plugin's ticker_interval config value, if provided.
	Ticker() (ticker <-chan time.Time)
	// Wraps provided PipelinePack in a PipelineCapture (with nil Capture
	// value) and drops it on the Output's input channel.
	Deliver(pack *PipelinePack)
	// Retains a pack for future delivery to the plugin when a plugin needs
	// to shut down and wants to retain the pack for the next time its
	// running properly
	RetainPack(pack *PipelineCapture)
}

// Heka Output plugin type.
type Output interface {
	Run(or OutputRunner, h PluginHelper) (err error)
}

// Output plugin that writes message contents out using Go standard library's
// `log` package.
type LogOutput struct {
	payloadOnly bool
}

func (self *LogOutput) Init(config interface{}) (err error) {
	conf := config.(PluginConfig)
	if p, ok := conf["payload_only"]; ok {
		self.payloadOnly, ok = p.(bool)
	}
	return
}

func (self *LogOutput) Run(or OutputRunner, h PluginHelper) (err error) {
	inChan := or.InChan()

	var (
		pack *PipelinePack
		msg  *message.Message
	)
	for plc := range inChan {
		pack = plc.Pack
		msg = pack.Message
		if self.payloadOnly {
			log.Printf(msg.GetPayload())
		} else {
			log.Printf("<\n\tTimestamp: %s\n"+
				"\tType: %s\n"+
				"\tHostname: %s\n"+
				"\tPid: %d\n"+
				"\tUUID: %s\n"+
				"\tLogger: %s\n"+
				"\tPayload: %s\n"+
				"\tEnvVersion: %s\n"+
				"\tSeverity: %d\n"+
				"\tFields: %+v\n"+
				"\tCaptures: %v\n>\n",
				time.Unix(0, msg.GetTimestamp()), msg.GetType(),
				msg.GetHostname(), msg.GetPid(), msg.GetUuidString(),
				msg.GetLogger(), msg.GetPayload(), msg.GetEnvVersion(),
				msg.GetSeverity(), msg.Fields, plc.Captures)
		}
		pack.Recycle()
	}
	return
}

// Create a protocol buffers stream for the given message, put it in the
// provided byte slice.
func createProtobufStream(pack *PipelinePack, outBytes *[]byte) (err error) {
	enc := client.NewProtobufEncoder(nil)
	err = enc.EncodeMessageStream(pack.Message, outBytes)
	return
}

var (
	FILEFORMATS = map[string]bool{
		"json":           true,
		"text":           true,
		"protobufstream": true,
	}

	TSFORMAT = "[2006/Jan/02:15:04:05 -0700] "
)

const NEWLINE byte = 10

// Output plugin that writes message contents to a file on the file system.
type FileOutput struct {
	path          string
	format        string
	prefix_ts     bool
	perm          os.FileMode
	flushInterval uint32
	file          *os.File
	batchChan     chan []byte
	backChan      chan []byte
}

// ConfigStruct for FileOutput plugin.
type FileOutputConfig struct {
	// Full output file path.
	Path string
	// Format for message serialization, from text (payload only), json, or
	// protobufstream.
	Format string
	// Add timestamp prefix to each output line?
	Prefix_ts bool
	// Output file permissions (default 0644).
	Perm os.FileMode
	// Interval at which accumulated file data should be written to disk, in
	// milliseconds (default 1000, i.e. 1 second).
	FlushInterval uint32
}

func (o *FileOutput) ConfigStruct() interface{} {
	return &FileOutputConfig{Format: "text", Perm: 0644, FlushInterval: 1000}
}

func (o *FileOutput) Init(config interface{}) (err error) {
	conf := config.(*FileOutputConfig)
	if _, ok := FILEFORMATS[conf.Format]; !ok {
		err = fmt.Errorf("FileOutput '%s' unsupported format: %s", conf.Path,
			conf.Format)
		return
	}
	o.path = conf.Path
	o.format = conf.Format
	o.prefix_ts = conf.Prefix_ts
	o.perm = conf.Perm
	if err = o.openFile(); err != nil {
		err = fmt.Errorf("FileOutput '%s' error opening file: %s", o.path, err)
		return
	}
	o.flushInterval = conf.FlushInterval
	o.batchChan = make(chan []byte)
	o.backChan = make(chan []byte, 2) // Never block on the hand-back
	return
}

func (o *FileOutput) openFile() (err error) {
	o.file, err = os.OpenFile(o.path, os.O_WRONLY|os.O_APPEND|os.O_CREATE, o.perm)
	return
}

func (o *FileOutput) Run(or OutputRunner, h PluginHelper) (err error) {
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
func (o *FileOutput) receiver(or OutputRunner, wg *sync.WaitGroup) {
	var plc *PipelineCapture
	var e error
	ok := true
	ticker := time.Tick(time.Duration(o.flushInterval) * time.Millisecond)
	outBatch := make([]byte, 0, 10000)
	outBytes := make([]byte, 0, 1000)
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

// Performs the actual task of extracting data from the pack and writing it
// into the output buffer in the proper format.
func (o *FileOutput) handleMessage(pack *PipelinePack, outBytes *[]byte) (err error) {
	if o.prefix_ts && o.format != "protobufstream" {
		ts := time.Now().Format(TSFORMAT)
		*outBytes = append(*outBytes, ts...)
	}
	switch o.format {
	case "json":
		if jsonMessage, err := json.Marshal(pack.Message); err == nil {
			*outBytes = append(*outBytes, jsonMessage...)
			*outBytes = append(*outBytes, NEWLINE)
		} else {
			err = fmt.Errorf("FileOutput '%s' error encoding to JSON: %s", o.path, err)
		}
	case "text":
		*outBytes = append(*outBytes, *pack.Message.Payload...)
		*outBytes = append(*outBytes, NEWLINE)
	case "protobufstream":
		if err = createProtobufStream(pack, &*outBytes); err != nil {
			err = fmt.Errorf("FileOutput '%s' error encoding to ProtoBuf: %s", o.path, err)
		}
	default:
		err = fmt.Errorf("FileOutput '%s' error: Invalid format %s", o.path, o.format)
	}
	return
}

// Runs in a separate goroutine, waits for buffered data on the committer
// channel, writes it out to the filesystem, and puts the now empty buffer on
// the return channel for reuse.
func (o *FileOutput) committer(wg *sync.WaitGroup) {
	initBatch := make([]byte, 0, 10000)
	o.backChan <- initBatch
	var outBatch []byte
	var err error

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
			n, err := o.file.Write(outBatch)
			if err != nil {
				log.Printf("FileOutput error writing to %s: %s", o.path, err)
			} else if n != len(outBatch) {
				log.Printf("FileOutput truncated output for %s", o.path)
			} else {
				o.file.Sync()
			}
			outBatch = outBatch[:0]
			o.backChan <- outBatch
		case <-hupChan:
			o.file.Close()
			if err = o.openFile(); err != nil {
				// TODO: Need a way to handle this gracefully, see
				// https://github.com/mozilla-services/heka/issues/38
				panic(fmt.Sprintf("FileOutput unable to reopen file '%s': %s",
					o.path, err))
			}
		}
	}

	o.file.Close()
	wg.Done()
}

// Output plugin that sends messages via TCP using the Heka protocol.
type TcpOutput struct {
	address    string
	connection net.Conn
}

// ConfigStruct for TcpOutput plugin.
type TcpOutputConfig struct {
	// String representation of the TCP address to which this output should be
	// sending data.
	Address string
}

func (t *TcpOutput) ConfigStruct() interface{} {
	return &TcpOutputConfig{Address: "localhost:9125"}
}

func (t *TcpOutput) Init(config interface{}) (err error) {
	conf := config.(*TcpOutputConfig)
	t.address = conf.Address
	t.connection, err = net.Dial("tcp", t.address)
	return
}

func (t *TcpOutput) Run(or OutputRunner, h PluginHelper) (err error) {
	var e error
	var n int
	outBytes := make([]byte, 0, 2000)

	for plc := range or.InChan() {
		outBytes = outBytes[:0]

		if e = createProtobufStream(plc.Pack, &outBytes); e != nil {
			or.LogError(e)
			continue
		}

		if n, e = t.connection.Write(outBytes); e != nil {
			or.LogError(fmt.Errorf("writing to %s: %s", t.address, e))
		} else if n != len(outBytes) {
			or.LogError(fmt.Errorf("truncated output to: %s", t.address))
		}
	}

	t.connection.Close()

	return
}
