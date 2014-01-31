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

package tcp

import (
	"bytes"
	"crypto/tls"
	"fmt"
	"github.com/mozilla-services/heka/message"
	. "github.com/mozilla-services/heka/pipeline"
	"io"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

// Output plugin that sends messages via TCP using the Heka protocol.
type TcpOutput struct {
	conf                *TcpOutputConfig
	parser              *MessageProtoParser
	address             string
	connection          net.Conn
	writeFile           *os.File
	writeId             uint
	readFile            *os.File
	readId              uint
	readOffset          int64
	checkpointFilename  string
	checkpointFile      *os.File
	queue               string
	name                string
	reportLock          sync.Mutex
	processMessageCount int64
	sentMessageCount    int64
}

// ConfigStruct for TcpOutput plugin.
type TcpOutputConfig struct {
	// String representation of the TCP address to which this output should be
	// sending data.
	Address string
	UseTls  bool `toml:"use_tls"`
	Tls     TlsConfig
	// Interval at which the output queue logs will roll, in
	// seconds. Defaults to 300.
	TickerInterval uint `toml:"ticker_interval"`
}

func (t *TcpOutput) ConfigStruct() interface{} {
	return &TcpOutputConfig{Address: "localhost:9125",
		TickerInterval: uint(300)}
}

func (t *TcpOutput) SetName(name string) {
	re := regexp.MustCompile("\\W")
	t.name = re.ReplaceAllString(name, "_")
}

func (t *TcpOutput) Init(config interface{}) (err error) {
	t.conf = config.(*TcpOutputConfig)
	t.parser = NewMessageProtoParser()
	t.address = t.conf.Address
	t.queue = GetHekaConfigDir(filepath.Join("output_queue", t.name))
	t.checkpointFilename = GetHekaConfigDir(filepath.Join(t.queue, "checkpoint.txt"))
	if !fileExists(t.queue) {
		if err = os.MkdirAll(t.queue, 0766); err != nil {
			return
		}
	}
	t.writeId = findBufferId(t.queue, true)
	return
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	if err == nil {
		return true
	}
	return false
}

func getQueueFilename(queue string, id uint) string {
	return filepath.Join(queue, fmt.Sprintf("%d.log", id))
}

func extractBufferId(filename string) (id uint, err error) {
	name := filepath.Base(filename)
	if len(name) < 5 {
		err = fmt.Errorf("invalid filename (too short)")
		return
	}
	i, err := strconv.Atoi(name[:len(name)-4])
	id = uint(i)
	return
}

func findBufferId(dir string, newest bool) uint {
	var current uint
	var first = true
	if matches, err := filepath.Glob(filepath.Join(dir, "*.log")); err == nil {
		for _, fn := range matches {
			id, err := extractBufferId(fn)
			if err != nil {
				continue
			}
			if first {
				current = id
				first = false
			} else {
				if newest {
					if id > current {
						current = id
					}
				} else {
					if id < current {
						current = id
					}
				}
			}
		}
	}
	return current
}

func (t *TcpOutput) writeCheckpoint(id uint, offset int64) (err error) {
	if t.checkpointFile == nil {
		if t.checkpointFile, err = os.OpenFile(t.checkpointFilename, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644); err != nil {
			return
		}
	}
	t.checkpointFile.Seek(0, 0)
	n, err := t.checkpointFile.WriteString(fmt.Sprintf("%d %d", id, offset))
	if err != nil {
		return
	}
	err = t.checkpointFile.Truncate(int64(n))
	return
}

func readCheckpoint(filename string) (id uint, offset int64, err error) {
	file, err := os.Open(filename)
	if err != nil {
		return
	}
	defer file.Close()

	b := make([]byte, 64)
	n, err := file.Read(b)
	if err != nil {
		return
	}
	idx := bytes.IndexByte(b, ' ')
	if idx == -1 {
		err = fmt.Errorf("invalid checkpoint format")
		return
	}

	var un uint64
	if un, err = strconv.ParseUint(string(b[:idx]), 10, 32); err != nil {
		err = fmt.Errorf("invalid checkpoint id")
		return
	}
	id = uint(un)

	if offset, err = strconv.ParseInt(string(b[idx+1:n]), 10, 64); err != nil {
		err = fmt.Errorf("invalid checkpoint offset")
		return
	}

	return
}

func (t *TcpOutput) writeToNextFile() (err error) {
	if t.writeFile != nil {
		t.writeFile.Close()
		t.writeFile = nil
	}
	t.writeId++
	t.writeFile, err = os.OpenFile(getQueueFilename(t.queue, t.writeId), os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0644)
	return err
}

func (t *TcpOutput) readFromNextFile() (err error) {
	if t.readFile != nil {
		t.readFile.Close()
		t.readFile = nil
	}

	t.readOffset = 0
	if fileExists(t.checkpointFilename) {
		if t.readId, t.readOffset, err = readCheckpoint(t.checkpointFilename); err != nil {
			return fmt.Errorf("readCheckpoint %s", err)
		}
	} else {
		t.readId = findBufferId(GetHekaConfigDir(t.queue), false)
	}
	if t.readFile, err = os.Open(getQueueFilename(t.queue, t.readId)); err != nil {
		return
	}
	_, err = t.readFile.Seek(t.readOffset, 0)
	return
}

func (t *TcpOutput) connect() (err error) {
	if t.conf.UseTls {
		var goTlsConf *tls.Config
		if goTlsConf, err = CreateGoTlsConfig(&t.conf.Tls); err != nil {
			return fmt.Errorf("TLS init error: %s", err)
		}
		t.connection, err = tls.Dial("tcp", t.address, goTlsConf)
	} else {
		t.connection, err = net.Dial("tcp", t.address)
	}
	return
}

func (t *TcpOutput) sendRecord(record []byte) (err error) {
	var n int
	if t.connection == nil {
		if err = t.connect(); err != nil {
			return
		}
	}

	if n, err = t.connection.Write(record); err != nil {
		err = fmt.Errorf("writing to %s: %s", t.address, err)
	} else if n != len(record) {
		err = fmt.Errorf("truncated output to: %s", t.address)
	}
	return
}

func (t *TcpOutput) StreamOutput(outputError, outputExit chan error, stopChan chan bool) {
	var (
		err    error
		n      int
		record []byte
	)

	defer func() {
		if t.checkpointFile != nil {
			t.checkpointFile.Close()
			t.checkpointFile = nil
		}
		if t.readFile != nil {
			t.readFile.Close()
			t.readFile = nil
		}
		if t.connection != nil {
			t.connection.Close()
			t.connection = nil
		}
	}()

	if err = t.readFromNextFile(); err != nil {
		outputExit <- err
		return
	}

	rh, _ := NewRetryHelper(RetryOptions{
		MaxDelay:   "10s",
		Delay:      "1s",
		MaxRetries: -1,
	})

	for true {
		select {
		case <-stopChan:
			outputExit <- nil
			return
		default: // carry on
		}
		n, record, err = t.parser.Parse(t.readFile)
		if err != nil {
			if err == io.EOF {
				nextReadId := t.readId + 1
				filename := getQueueFilename(t.queue, nextReadId)
				if fileExists(filename) {
					t.readFile.Close()
					t.readFile = nil
					if err = os.Remove(getQueueFilename(t.queue, t.readId)); err != nil {
						break
					}
					if err = t.writeCheckpoint(nextReadId, 0); err != nil {
						break
					}
					if err = t.readFromNextFile(); err != nil {
						break
					}
				} else {
					time.Sleep(time.Duration(500) * time.Millisecond)
				}
			} else {
				break
			}
		} else {
			if len(record) > 0 {
				rh.Reset()
				for true {
					err = t.sendRecord(record)
					if err == nil {
						atomic.AddInt64(&t.sentMessageCount, 1)
						break
					}
					select {
					case <-stopChan:
						outputExit <- nil
						return
					default:
						outputError <- err
						if t.connection != nil {
							t.connection.Close()
							t.connection = nil
						}
						rh.Wait() // this will delay Heka shutdown up to MaxDelay
					}
				}
			} else {
				runtime.Gosched()
			}
		}
		if n > 0 {
			t.readOffset += int64(n) // offset can advance without finding a valid record
			if err = t.writeCheckpoint(t.readId, t.readOffset); err != nil {
				break
			}
		}
	}

	outputExit <- err
	return
}

func (t *TcpOutput) Run(or OutputRunner, h PluginHelper) (err error) {
	var (
		ok          = true
		pack        *PipelinePack
		outBytes    = make([]byte, 0, 1000) // encoding will reallocate the buffer as necessary
		inChan      = or.InChan()
		ticker      = or.Ticker()
		outputExit  = make(chan error)
		outputError = make(chan error, 5)
		stopChan    = make(chan bool, 1)
	)

	if err = t.writeToNextFile(); err != nil {
		return
	}

	go t.StreamOutput(outputError, outputExit, stopChan)

	for ok {
		select {
		case pack, ok = <-inChan:
			if !ok {
				stopChan <- true
				<-outputExit
				break
			}
			atomic.AddInt64(&t.processMessageCount, 1)
			if err = ProtobufEncodeMessage(pack, &outBytes); err != nil {
				or.LogError(err)
				pack.Recycle()
				continue
			}
			if _, err = t.writeFile.Write(outBytes); err != nil {
				or.LogError(fmt.Errorf("writing to %s: %s", getQueueFilename(t.queue, t.writeId), err))
			}
			pack.Recycle()

		case <-ticker:
			if err = t.writeToNextFile(); err != nil {
				return
			}

		case e := <-outputError:
			or.LogError(e)

		case err = <-outputExit:
			ok = false
		}
	}

	return
}

func init() {
	RegisterPlugin("TcpOutput", func() interface{} {
		return new(TcpOutput)
	})
}

// Satisfies the `pipeline.ReportingPlugin` interface to provide plugin state
// information to the Heka report and dashboard.
func (t *TcpOutput) ReportMsg(msg *message.Message) error {
	t.reportLock.Lock()
	defer t.reportLock.Unlock()

	message.NewInt64Field(msg, "ProcessMessageCount", atomic.LoadInt64(&t.processMessageCount), "count")
	message.NewInt64Field(msg, "SentMessageCount", atomic.LoadInt64(&t.sentMessageCount), "count")
	return nil
}
