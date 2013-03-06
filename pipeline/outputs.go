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
	"code.google.com/p/goprotobuf/proto"
	"encoding/json"
	"fmt"
	"github.com/mozilla-services/heka/client"
	"github.com/mozilla-services/heka/message"
	"github.com/rafrombrc/go-notify"
	"log"
	"net"
	"os"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
)

type OutputRunner interface {
	PluginRunnerBase
	Start(wg *sync.WaitGroup)
	Stop()
	Output() Output
}

type outputRunner struct {
	pluginRunnerBase
	output Output
}

func NewOutputRunner(name string, output Output) OutputRunner {
	inChan := make(chan *PipelinePack, PIPECHAN_BUFSIZE)
	return &outputRunner{
		pluginRunnerBase{
			name:   name,
			inChan: inChan,
		},
		output,
	}
}

func (self *outputRunner) Start(wg *sync.WaitGroup) {
	go func() {
		var pack *PipelinePack
		var ok bool = true
		log.Printf("Output started: %s\n", self.Name())
		for ok {
			runtime.Gosched()
			select {
			case pack, ok = <-self.InChan():
				if !ok {
					break
				}
				self.output.Deliver(pack)
				pack.Recycle()
			}
		}
		log.Printf("Output stopped: %s\n", self.Name())
		wg.Done()
	}()
}

func (self *outputRunner) Stop() {
	close(self.InChan())
}

func (self *outputRunner) Output() Output {
	return self.output
}

type Output interface {
	Deliver(pipelinePack *PipelinePack)
}

type LogOutput struct {
}

func (self *LogOutput) Init(config interface{}) error {
	return nil
}

func (self *LogOutput) Deliver(pipelinePack *PipelinePack) {
	msg := *(pipelinePack.Message)
	log.Printf("<\n\tTimestamp: %s\n\tType: %s\n\tHostname: %s\n\tPid: %d\n\tUUID: %s"+
		"\n\tLogger: %s\n\tPayload: %s\n\tEnvVersion: %s\n\tSeverity: %d\n"+
		"\tFields: %+v\n>\n",
		time.Unix(0, msg.GetTimestamp()),
		msg.GetType(), msg.GetHostname(), msg.GetPid(), msg.GetUuidString(),
		msg.GetLogger(), msg.GetPayload(), msg.GetEnvVersion(),
		msg.GetSeverity(), msg.Fields)
}

// Create a protocol buffers stream for the given message, put it in the given
// byte slice.
func createProtobufStream(pack *PipelinePack, outBytes *[]byte) (err error) {
	messageSize := proto.Size(pack.Message)
	if err = client.EncodeStreamHeader(messageSize, message.Header_PROTOCOL_BUFFER,
		outBytes); err != nil {
		return
	}
	headerSize := len(*outBytes)
	pbuf := proto.NewBuffer((*outBytes)[headerSize:])
	if err = pbuf.Marshal(pack.Message); err != nil {
		return
	}
	*outBytes = (*outBytes)[:headerSize+messageSize]
	return
}

// FileWriter implementation
var (
	FILEFORMATS = map[string]bool{
		"json":           true,
		"text":           true,
		"protobufstream": true,
	}

	TSFORMAT = "[2006/Jan/02:15:04:05 -0700] "
)

const NEWLINE byte = 10

type FileOutput struct {
	path          string
	format        string
	prefix_ts     bool
	perm          os.FileMode
	flushInterval uint32
	file          *os.File
	inChan        chan *PipelinePack
	batchChan     chan []byte
	backChan      chan []byte
	wg            *sync.WaitGroup
}

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
	o.inChan = make(chan *PipelinePack, PIPECHAN_BUFSIZE)
	o.batchChan = make(chan []byte)
	o.backChan = make(chan []byte, 1) // Don't block on the hand-back
	return
}

func (o *FileOutput) openFile() (err error) {
	o.file, err = os.OpenFile(o.path, os.O_WRONLY|os.O_APPEND|os.O_CREATE, o.perm)
	return
}

func (o *FileOutput) Deliver(pack *PipelinePack) {
	atomic.AddInt32(&pack.RefCount, 1)
	o.inChan <- pack
}

func (o *FileOutput) Start(wg *sync.WaitGroup) {
	o.wg = wg
	wg.Add(1)
	go o.receiver()
	go o.committer()
}

func (o *FileOutput) receiver() {
	var pack *PipelinePack
	var err error
	var ok bool
	ticker := time.Tick(time.Duration(o.flushInterval) * time.Millisecond)
	outBatch := make([]byte, 0, 10000)
	outBytes := make([]byte, 0, 1000)

	stopChan := make(chan interface{})
	notify.Start(STOP, stopChan)

	for {
		select {
		case pack, ok = <-o.inChan:
			if !ok {
				// Closed inChan => we're shutting down, flush data
				if len(outBatch) > 0 {
					o.batchChan <- outBatch
				}
				close(o.batchChan)
				return
			}
			if err = o.handleMessage(pack, &outBytes); err != nil {
				log.Println(err)
			} else {
				outBatch = append(outBatch, outBytes...)
			}
			outBytes = outBytes[:0]
			pack.Recycle()
		case <-ticker:
			if len(outBatch) > 0 {
				// This will block until the other side is ready to accept
				// this batch, freeing us to start on the next one.
				o.batchChan <- outBatch
				outBatch = <-o.backChan
			}
		case <-stopChan:
			close(o.inChan)
		}
	}
}

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

func (o *FileOutput) committer() {
	initBatch := make([]byte, 0, 10000)
	o.backChan <- initBatch
	var outBatch []byte
	var err error
	var ok bool

	hupChan := make(chan interface{})
	notify.Start(RELOAD, hupChan)

	for {
		select {
		case outBatch, ok = <-o.batchChan:
			if !ok {
				// Channel is closed => we're shutting down, exit cleanly.
				o.file.Close()
				o.wg.Done()
				return
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
}

// TcpWriter implementation
type TcpWriter struct {
	address    string
	connection net.Conn
}

type TcpWriterConfig struct {
	Address string
}

func (t *TcpWriter) ConfigStruct() interface{} {
	return &TcpWriterConfig{Address: "localhost:9125"}
}

func (t *TcpWriter) Init(config interface{}) (err error) {
	conf := config.(*TcpWriterConfig)
	t.address = conf.Address
	t.connection, err = net.Dial("tcp", t.address)
	return
}

func (t *TcpWriter) MakeOutData() interface{} {
	b := make([]byte, 0, 2000)
	return &b
}

func (t *TcpWriter) ZeroOutData(outData interface{}) {
	outBytes := outData.(*[]byte)
	*outBytes = (*outBytes)[:0]
}

func (t *TcpWriter) PrepOutData(pack *PipelinePack, outData interface{},
	timeout *time.Duration) (err error) {
	err = createProtobufStream(pack, outData.(*[]byte))
	return
}

func (t *TcpWriter) Write(outData interface{}) (err error) {
	outBytes := outData.(*[]byte)
	n, err := t.connection.Write(*outBytes)
	if err != nil {
		err = fmt.Errorf("TcpWriter error writing to %s: %s", t.address, err)
		return err
	} else if n != len(*outBytes) {
		err = fmt.Errorf("TcpWriter truncated output for %s", t.address)
		return err
	}
	return nil
}

func (t *TcpWriter) Event(eventType string) {
	if eventType == STOP {
		t.connection.Close()
	}
}
