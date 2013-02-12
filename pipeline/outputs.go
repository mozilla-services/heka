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
	"sort"
	"sync"
	"time"
)

type outputRunner struct {
	PluginRunnerBase
	Output      Output
	recycleChan chan<- *PipelinePack
}

func newOutputRunner(name string, output Output, recycleChan chan *PipelinePack) *outputRunner {
	outRunner := &outputRunner{}
	outRunner.Name = name
	outRunner.Output = output
	outRunner.InChan = make(chan *PipelinePack, PIPECHAN_BUFSIZE)
	outRunner.recycleChan = recycleChan
	return outRunner
}

func (self *outputRunner) Start(wg *sync.WaitGroup) {
	stopChan := make(chan interface{})
	notify.Start(STOP, stopChan)

	go func() {
		var pack *PipelinePack
	runnerLoop:
		for {
			runtime.Gosched()
			select {
			case pack = <-self.InChan:
				self.Output.Deliver(pack)
				pack.Zero()
				self.recycleChan <- pack
			case <-stopChan:
				break runnerLoop
			}
		}
		log.Println("Output stopped: ", self.Name)
		wg.Done()
	}()
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
		"\n\tLogger: %s\n\tPayload: %s\n\tFields: %+v\n>\n",
		time.Unix(0, msg.GetTimestamp()),
		msg.GetType(), msg.GetHostname(), msg.GetPid(), msg.GetUuid(),
		msg.GetLogger(), msg.GetPayload(), msg.Fields)
}

type CounterOutput struct {
	count uint
}

func (self *CounterOutput) Init(config interface{}) error {
	go self.counterLoop()
	return nil
}

func (self *CounterOutput) Deliver(pipelinePack *PipelinePack) {
	self.count++
}

func (self *CounterOutput) counterLoop() {
	tick := time.NewTicker(time.Duration(time.Second))
	aggregate := time.NewTicker(time.Duration(10 * time.Second))
	lastTime := time.Now()
	lastCount := uint(0)
	count := uint(0)
	zeroes := int8(0)
	var (
		msgsSent    uint
		elapsedTime time.Duration
		now         time.Time
		rate        float64
		rates       []float64
	)
	for {
		// Here for performance reasons
		runtime.Gosched()
		select {
		case <-aggregate.C:
			count = self.count
			amount := len(rates)
			if amount < 1 {
				continue
			}
			sort.Float64s(rates)
			min := rates[0]
			max := rates[amount-1]
			mean := min
			sum := float64(0)
			for _, val := range rates {
				sum += val
			}
			mean = sum / float64(amount)
			log.Printf("AGG Sum. Min: %0.2f   Max: %0.2f     Mean: %0.2f",
				min, max, mean)
			rates = rates[:0]
		case <-tick.C:
			count = self.count
			now = time.Now()
			msgsSent = count - lastCount
			lastCount = count
			elapsedTime = now.Sub(lastTime)
			lastTime = now
			rate = float64(msgsSent) / elapsedTime.Seconds()
			if msgsSent == 0 {
				if msgsSent == 0 || zeroes == 3 {
					continue
				}
				zeroes++
			} else {
				zeroes = 0
			}
			log.Printf("Got %d messages. %0.2f msg/sec\n", count, rate)
			rates = append(rates, rate)
		}
	}
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

type FileWriter struct {
	path      string
	format    string
	prefix_ts bool
	file      *os.File
	outBatch  []byte
}

type FileWriterConfig struct {
	Path      string
	Format    string
	Prefix_ts bool
	Perm      os.FileMode
}

func (self *FileWriter) ConfigStruct() interface{} {
	return &FileWriterConfig{Format: "text", Perm: 0666}
}

func (self *FileWriter) Init(config interface{}) (ticker <-chan time.Time,
	err error) {
	conf := config.(*FileWriterConfig)
	_, ok := FILEFORMATS[conf.Format]
	if !ok {
		return nil, fmt.Errorf("Unsupported FileOutput format: %s",
			conf.Format)
	}
	self.path = conf.Path
	self.format = conf.Format
	self.prefix_ts = conf.Prefix_ts
	self.outBatch = make([]byte, 0, 10000)
	if self.file, err = os.OpenFile(conf.Path,
		os.O_WRONLY|os.O_APPEND|os.O_CREATE, conf.Perm); err != nil {
		return nil, err
	}
	ticker = time.Tick(time.Second)
	return
}

func (self *FileWriter) MakeOutData() interface{} {
	b := make([]byte, 0, 2000)
	return &b
}

func (self *FileWriter) ZeroOutData(outData interface{}) {
	outBytes := outData.(*[]byte)
	*outBytes = (*outBytes)[:0]
}

func (self *FileWriter) PrepOutData(pack *PipelinePack, outData interface{},
	timeout *time.Duration) error {
	outBytes := outData.(*[]byte)
	if self.prefix_ts && self.format != "protobufstream" {
		ts := time.Now().Format(TSFORMAT)
		*outBytes = append(*outBytes, ts...)
	}

	switch self.format {
	case "json":
		jsonMessage, err := json.Marshal(pack.Message)
		if err != nil {
			log.Printf("Error converting message to JSON for %s", self.path)
			return err
		}
		*outBytes = append(*outBytes, jsonMessage...)
		*outBytes = append(*outBytes, NEWLINE)
	case "text":
		*outBytes = append(*outBytes, *pack.Message.Payload...)
		*outBytes = append(*outBytes, NEWLINE)
	case "protobufstream":
		return createProtobufStream(pack, outBytes)
	}
	return nil
}

func createProtobufStream(pack *PipelinePack, outBytes *[]byte) error {
	messageSize := proto.Size(pack.Message)
	err := client.EncodeStreamHeader(messageSize, message.Header_PROTOCOL_BUFFER, outBytes)
	if err != nil {
		return err
	}
	headerSize := len(*outBytes)
	pbuf := proto.NewBuffer((*outBytes)[headerSize:])
	err = pbuf.Marshal(pack.Message)
	if err != nil {
		return err
	}
	*outBytes = (*outBytes)[:headerSize+messageSize]
	return nil
}

func (self *FileWriter) Batch(outData interface{}) (err error) {
	outBytes := outData.(*[]byte)
	self.outBatch = append(self.outBatch, *outBytes...)
	return
}

func (self *FileWriter) Commit() (err error) {
	n, err := self.file.Write(self.outBatch)
	if err != nil {
		err = fmt.Errorf("FileWriter error writing to %s: %s", self.path,
			err)
		return err
	} else if n != len(self.outBatch) {
		err = fmt.Errorf("FileWriter truncated output for %s", self.path)
		return err
	}
	self.outBatch = self.outBatch[:0]
	self.file.Sync()
	return nil
}

func (self *FileWriter) Event(eventType string) {
	if eventType == STOP {
		self.file.Close()
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
	timeout *time.Duration) error {
	err := createProtobufStream(pack, outData.(*[]byte))
	return err

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
