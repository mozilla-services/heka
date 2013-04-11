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
#
# ***** END LICENSE BLOCK *****/

/*

Flood client.

Flooding client used to test heka message through-put and tolerances.
Can be run with several configuration options to indicate how the messages
should be sent and encoded.

*/
package main

import (
	"code.google.com/p/go-uuid/uuid"
	"code.google.com/p/goprotobuf/proto"
	"flag"
	"fmt"
	"github.com/BurntSushi/toml"
	"github.com/mozilla-services/heka/client"
	"github.com/mozilla-services/heka/message"
	"log"
	"math"
	"math/rand"
	"net"
	"os"
	"os/signal"
	"runtime"
	"runtime/pprof"
	"strings"
	"syscall"
	"time"
)

type FloodTest struct {
	IpAddress            string                       `toml:"ip_address"`
	Sender               string                       `toml:"sender"`
	PprofFile            string                       `toml:"pprof_file"`
	Encoder              string                       `toml:"encoder"`
	NumMessages          uint64                       `toml:"num_messages"`
	Signer               message.MessageSigningConfig `toml:"signer"`
	CorruptPercentage    float64                      `toml:"corrupt_percentage"`
	SignedPercentage     float64                      `toml:"signed_percentage"`
	VariableSizeMessages bool                         `toml:"variable_size_messages"`
}

type FloodConfig map[string]FloodTest

type Sender interface {
	SendMessage(msgBytes []byte, encoding message.Header_MessageEncoding,
		msc *message.MessageSigningConfig, signed, corrupt bool) error
}

type UdpSender struct {
	connection  *net.UDPConn
	buf         []byte
	protoBuffer *proto.Buffer
}

func NewUdpSender(addrStr string) (*UdpSender, error) {
	var self *UdpSender
	udpAddr, err := net.ResolveUDPAddr("udp", addrStr)
	conn, err := net.DialUDP("udp", nil, udpAddr)
	if err == nil {
		self = &(UdpSender{connection: conn})
		self.buf = make([]byte, message.MAX_MESSAGE_SIZE+message.MAX_HEADER_SIZE+3)
		self.protoBuffer = proto.NewBuffer(self.buf)
	} else {
		self = nil
	}
	return self, err
}

func (self *UdpSender) SendMessage(msgBytes []byte,
	encoding message.Header_MessageEncoding,
	msc *message.MessageSigningConfig, signed, corrupt bool) (err error) {
	if signed {
		err = client.EncodeStreamHeader(msgBytes, encoding, &self.buf, msc)
	} else {
		err = client.EncodeStreamHeader(msgBytes, encoding, &self.buf, nil)
	}
	if err != nil {
		return
	}
	self.buf = append(self.buf, msgBytes...)
	if corrupt {
		index := rand.Int() % len(self.buf)
		replacement := rand.Int() % 256
		self.buf[index] = byte(replacement)
	}
	_, err = self.connection.Write(self.buf)
	return
}

type TcpSender struct {
	connection  net.Conn
	header      []byte
	protoBuffer *proto.Buffer
}

func NewTcpSender(addrStr string) (n *TcpSender, err error) {
	conn, err := net.Dial("tcp", addrStr)
	if err == nil {
		n = &(TcpSender{connection: conn})
		n.header = make([]byte, message.MAX_HEADER_SIZE+3)
		n.protoBuffer = proto.NewBuffer(n.header)
	}
	return
}

func (self *TcpSender) SendMessage(msgBytes []byte,
	encoding message.Header_MessageEncoding,
	msc *message.MessageSigningConfig, signed, corrupt bool) (err error) {
	if signed {
		err = client.EncodeStreamHeader(msgBytes, encoding, &self.header, msc)
	} else {
		err = client.EncodeStreamHeader(msgBytes, encoding, &self.header, nil)
	}
	if err != nil {
		return
	}
	var corruptHeader = (corrupt && rand.Int()%2 == 1)

	if corruptHeader {
		index := rand.Int() % len(self.header)
		replacement := rand.Int() % 256
		self.header[index] = byte(replacement)
	}
	_, err = self.connection.Write(self.header)
	if err == nil {
		var b byte
		var index int
		if corrupt && !corruptHeader {
			index = rand.Int() % len(msgBytes)
			replacement := rand.Int() % 256
			b = msgBytes[index]
			msgBytes[index] = byte(replacement)
			_, err = self.connection.Write(msgBytes)
			msgBytes[index] = b
		} else {
			_, err = self.connection.Write(msgBytes)
		}
	}
	return
}

func timerLoop(count *uint64, ticker *time.Ticker) {
	lastTime := time.Now().UTC()
	lastCount := *count
	zeroes := int8(0)
	var (
		msgsSent, newCount uint64
		elapsedTime        time.Duration
		now                time.Time
		rate               float64
	)
	for {
		_ = <-ticker.C
		newCount = *count
		now = time.Now()
		msgsSent = newCount - lastCount
		lastCount = newCount
		elapsedTime = now.Sub(lastTime)
		lastTime = now
		rate = float64(msgsSent) / elapsedTime.Seconds()
		if msgsSent == 0 {
			if newCount == 0 || zeroes == 3 {
				continue
			}
			zeroes++
		} else {
			zeroes = 0
		}
		log.Printf("Sent %d messages. %0.2f msg/sec\n", newCount, rate)
	}
}

func makeVariableMessage(encoder client.Encoder, items int) [][]byte {
	ma := make([][]byte, items)
	hostname, _ := os.Hostname()
	pid := int32(os.Getpid())
	var cnt int

	for x := 0; x < items; x++ {
		msg := &message.Message{}
		msg.SetUuid(uuid.NewRandom())
		msg.SetTimestamp(time.Now().UnixNano())
		msg.SetType("hekabench")
		msg.SetLogger("flood")
		msg.SetSeverity(int32(0))
		msg.SetEnvVersion("0.2")
		msg.SetPid(pid)
		msg.SetHostname(hostname)
		cnt = rand.Int() % 5
		for c := 0; c < cnt; c++ {
			field, _ := message.NewField(fmt.Sprintf("string%d", c), fmt.Sprintf("value%d", c), message.Field_RAW)
			msg.AddField(field)
		}
		cnt = rand.Int() % 5
		for c := 0; c < cnt; c++ {
			b := byte(c)
			field, _ := message.NewField(fmt.Sprintf("bytes%d", c), []byte{b, b, b, b, b, b, b, b}, message.Field_RAW)
			msg.AddField(field)
		}
		cnt = rand.Int() % 5
		for c := 0; c < cnt; c++ {
			field, _ := message.NewField(fmt.Sprintf("int%d", c), c, message.Field_RAW)
			msg.AddField(field)
		}
		cnt = rand.Int() % 5
		for c := 0; c < cnt; c++ {
			field, _ := message.NewField(fmt.Sprintf("double%d", c), float64(c), message.Field_RAW)
			msg.AddField(field)
		}
		cnt = rand.Int() % 5
		for c := 0; c < cnt; c++ {
			field, _ := message.NewField(fmt.Sprintf("bool%d", c), true, message.Field_RAW)
			msg.AddField(field)
		}
		cnt = (rand.Int() % 63) * 1024
		buf := make([]byte, cnt)
		field, _ := message.NewField("filler", buf, message.Field_RAW)
		msg.AddField(field)

		mbytes, err := encoder.EncodeMessage(msg)
		if err != nil {
			log.Println(err)
		}
		ma[x] = mbytes
	}
	return ma
}

func makeFixedMessage(encoder client.Encoder) [][]byte {
	ma := make([][]byte, 1)
	hostname, _ := os.Hostname()
	pid := int32(os.Getpid())

	msg := &message.Message{}
	msg.SetType("hekabench")
	msg.SetTimestamp(time.Now().UnixNano())
	msg.SetUuid(uuid.NewRandom())
	msg.SetSeverity(int32(6))
	msg.SetEnvVersion("0.8")
	msg.SetPid(pid)
	msg.SetHostname(hostname)
	msg.SetPayload(fmt.Sprintf("hekabench: %s", hostname))
	mbytes, err := encoder.EncodeMessage(msg)
	if err != nil {
		log.Println(err)
	}
	ma[0] = mbytes
	return ma
}

func main() {
	configFile := flag.String("config", "flood.toml", "Flood configuration file")
	configTest := flag.String("test", "default", "Test section to load")
	flag.Parse()

	var config FloodConfig
	if _, err := toml.DecodeFile(*configFile, &config); err != nil {
		log.Printf("Error decoding config file: %s", err)
		return
	}
	var test FloodTest
	var ok bool
	if test, ok = config[*configTest]; !ok {
		log.Printf("Configuration test: '%s' was not found", *configTest)
		return
	}

	if test.PprofFile != "" {
		profFile, err := os.Create(test.PprofFile)
		if err != nil {
			log.Fatalln(err)
		}
		pprof.StartCPUProfile(profFile)
		defer pprof.StopCPUProfile()
	}

	var err error
	var sender Sender
	switch test.Sender {
	case "udp":
		sender, err = NewUdpSender(test.IpAddress)
	case "tcp":
		sender, err = NewTcpSender(test.IpAddress)
	}
	if err != nil {
		log.Fatalf("Error creating sender: %s\n", err.Error())
	}

	var encoder client.Encoder
	switch test.Encoder {
	case "json":
		encoder = new(client.JsonEncoder)
	case "protobuf":
		encoder = new(client.ProtobufEncoder)
	}

	var messageArraySize = 1
	var messageArray [][]byte

	if test.VariableSizeMessages {
		messageArraySize = 100
		messageArray = makeVariableMessage(encoder, messageArraySize)
	} else {
		messageArray = makeFixedMessage(encoder)
	}
	// wait for sigint
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT)
	var msgsSent uint64
	var corruptPercentage, lastCorruptPercentage, signedPercentage, lastSignedPercentage float64
	var corrupt, signed bool

	// set up counter loop
	ticker := time.NewTicker(time.Duration(time.Second))
	go timerLoop(&msgsSent, ticker)

	test.CorruptPercentage /= 100.0
	test.SignedPercentage /= 100.0

	for gotsigint := false; !gotsigint; {
		runtime.Gosched()
		select {
		case <-sigChan:
			gotsigint = true
			continue
		default:
		}
		corruptPercentage = math.Floor(float64(msgsSent) * test.CorruptPercentage)
		if corruptPercentage != lastCorruptPercentage {
			lastCorruptPercentage = corruptPercentage
			corrupt = true
		} else {
			corrupt = false
		}
		signedPercentage = math.Floor(float64(msgsSent) * test.SignedPercentage)
		if signedPercentage != lastSignedPercentage {
			lastSignedPercentage = signedPercentage
			signed = true
		} else {
			signed = false
		}

		msgId := rand.Int() % messageArraySize
		err = sender.SendMessage(messageArray[msgId], encoder.Encoding(), &test.Signer, signed, corrupt)
		if err != nil {
			if !strings.Contains(err.Error(), "connection refused") {
				log.Printf("Error sending message: %s\n",
					err.Error())
			}
		} else {
			msgsSent++
			if test.NumMessages != 0 && msgsSent >= test.NumMessages {
				break
			}
		}
	}
	log.Println("Clean shutdown: ", msgsSent, " messages sent")
}
