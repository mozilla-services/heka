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

/*

Flood client.

Flooding client used to test heka message through-put and tolerances.
Can be run with several configuration options to indicate how the messages
should be sent and encoded.

*/
package main

import (
	"bytes"
	"code.google.com/p/go-uuid/uuid"
	"flag"
	"fmt"
	"github.com/bbangert/toml"
	"github.com/mozilla-services/heka/client"
	"github.com/mozilla-services/heka/message"
	"io"
	"log"
	"math"
	"math/rand"
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
	StaticMessageSize    uint64                       `toml:"static_message_size"`
	AsciiOnly            bool                         `toml:"ascii_only"`
}

type FloodConfig map[string]FloodTest

func timerLoop(count, bytes *uint64, ticker *time.Ticker) {
	lastTime := time.Now().UTC()
	lastCount := *count
	lastBytes := *bytes
	zeroes := int8(0)
	var (
		msgsSent, newCount, bytesSent, newBytes uint64
		elapsedTime                             time.Duration
		now                                     time.Time
		msgRate, bitRate                        float64
	)
	for {
		_ = <-ticker.C
		newCount = *count
		newBytes = *bytes
		now = time.Now()
		msgsSent = newCount - lastCount
		lastCount = newCount
		bytesSent = newBytes - lastBytes
		lastBytes = newBytes
		elapsedTime = now.Sub(lastTime)
		lastTime = now
		msgRate = float64(msgsSent) / elapsedTime.Seconds()
		bitRate = float64(bytesSent*8.0) / 1e6 / elapsedTime.Seconds()
		if msgsSent == 0 {
			if newCount == 0 || zeroes == 3 {
				continue
			}
			zeroes++
		} else {
			zeroes = 0
		}
		log.Printf("Sent %d messages. %0.2f msg/sec %0.2f Mbit/sec\n", newCount, msgRate, bitRate)
	}
}

func makeVariableMessage(encoder client.Encoder, items int, rdm *randomDataMaker) [][]byte {
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
		cnt = (rand.Int() % 3) * 1024
		msg.SetPayload(makePayload(uint64(cnt), rdm))
		cnt = rand.Int() % 5
		for c := 0; c < cnt; c++ {
			field, _ := message.NewField(fmt.Sprintf("string%d", c), fmt.Sprintf("value%d", c), "")
			msg.AddField(field)
		}
		cnt = rand.Int() % 5
		for c := 0; c < cnt; c++ {
			b := byte(c)
			field, _ := message.NewField(fmt.Sprintf("bytes%d", c), []byte{b, b, b, b, b, b, b, b}, "")
			msg.AddField(field)
		}
		cnt = rand.Int() % 5
		for c := 0; c < cnt; c++ {
			field, _ := message.NewField(fmt.Sprintf("int%d", c), c, "")
			msg.AddField(field)
		}
		cnt = rand.Int() % 5
		for c := 0; c < cnt; c++ {
			field, _ := message.NewField(fmt.Sprintf("double%d", c), float64(c), "")
			msg.AddField(field)
		}
		cnt = rand.Int() % 5
		for c := 0; c < cnt; c++ {
			field, _ := message.NewField(fmt.Sprintf("bool%d", c), true, "")
			msg.AddField(field)
		}
		cnt = (rand.Int() % 60) * 1024
		buf := make([]byte, cnt)
		field, _ := message.NewField("filler", buf, "")
		msg.AddField(field)

		var stream []byte
		if err := encoder.EncodeMessageStream(msg, &stream); err != nil {
			log.Println(err)
		}
		ma[x] = stream
	}
	return ma
}

type randomDataMaker struct {
	src       rand.Source
	asciiOnly bool
}

func (r *randomDataMaker) Read(p []byte) (n int, err error) {
	todo := len(p)
	offset := 0
	for {
		valStash := int64(r.src.Int63())
		var val int64
		for i := 0; i < 8; i++ {
			val = valStash & 0xff
			if r.asciiOnly {
				val = val % 94
				val += 32
			}
			p[offset] = byte(val)
			todo--
			if todo == 0 {
				return len(p), nil
			}
			offset++
			valStash >>= 8
		}
	}
	panic("unreachable")
}

func makePayload(size uint64, rdm *randomDataMaker) (payload string) {
	hostname, _ := os.Hostname()
	payload = fmt.Sprintf("hekabench: %s", hostname)
	buf := make([]byte, size)
	payloadSuffix := bytes.NewBuffer(buf)
	if _, err := io.CopyN(payloadSuffix, rdm, int64(size)); err == nil {
		payload = fmt.Sprintf("%s - %s", payload, payloadSuffix.String())
	} else {
		log.Println("Error getting random string: ", err)
	}
	return
}

func makeFixedMessage(encoder client.Encoder, size uint64, rdm *randomDataMaker) [][]byte {
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
	msg.SetPayload(makePayload(size, rdm))
	var stream []byte
	if err := encoder.EncodeMessageStream(msg, &stream); err != nil {
		log.Println(err)
	}
	ma[0] = stream
	return ma
}

func sendMessage(sender client.Sender, buf []byte, corrupt bool) (err error) {
	var b byte
	var index int
	if corrupt {
		index = rand.Int() % len(buf)
		replacement := rand.Int() % 256
		b = buf[index]
		buf[index] = byte(replacement)
		err = sender.SendMessage(buf)
		buf[index] = b
	} else {
		err = sender.SendMessage(buf)
	}
	return
}

func main() {
	configFile := flag.String("config", "flood.toml", "Flood configuration file")
	configTest := flag.String("test", "default", "Test section to load")

	flag.Parse()

	if flag.NFlag() == 0 {
		flag.PrintDefaults()
		os.Exit(0)
	}

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

	sender, err := client.NewNetworkSender(test.Sender, test.IpAddress)
	if err != nil {
		log.Fatalf("Error creating sender: %s\n", err.Error())
	}

	var unsignedEncoder client.Encoder
	var signedEncoder client.Encoder
	switch test.Encoder {
	case "json":
		unsignedEncoder = client.NewJsonEncoder(nil)
		signedEncoder = client.NewJsonEncoder(&test.Signer)
	case "protobuf":
		unsignedEncoder = client.NewProtobufEncoder(nil)
		signedEncoder = client.NewProtobufEncoder(&test.Signer)
	}

	var numTestMessages = 1
	var unsignedMessages [][]byte
	var signedMessages [][]byte

	rdm := &randomDataMaker{
		src:       rand.NewSource(time.Now().UnixNano()),
		asciiOnly: test.AsciiOnly,
	}

	if test.VariableSizeMessages {
		numTestMessages = 64
		unsignedMessages = makeVariableMessage(unsignedEncoder, numTestMessages, rdm)
		signedMessages = makeVariableMessage(signedEncoder, numTestMessages, rdm)
	} else {
		if test.StaticMessageSize == 0 {
			test.StaticMessageSize = 1000
		}
		unsignedMessages = makeFixedMessage(unsignedEncoder, test.StaticMessageSize,
			rdm)
		signedMessages = makeFixedMessage(signedEncoder, test.StaticMessageSize,
			rdm)
	}
	// wait for sigint
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT)
	var msgsSent, bytesSent uint64
	var corruptPercentage, lastCorruptPercentage, signedPercentage, lastSignedPercentage float64
	var corrupt bool

	// set up counter loop
	ticker := time.NewTicker(time.Duration(time.Second))
	go timerLoop(&msgsSent, &bytesSent, ticker)

	test.CorruptPercentage /= 100.0
	test.SignedPercentage /= 100.0

	var buf []byte
	for gotsigint := false; !gotsigint; {
		runtime.Gosched()
		select {
		case <-sigChan:
			gotsigint = true
			continue
		default:
		}
		msgId := rand.Int() % numTestMessages
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
			buf = signedMessages[msgId]
		} else {
			buf = unsignedMessages[msgId]
		}
		bytesSent += uint64(len(buf))
		err = sendMessage(sender, buf, corrupt)
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
	sender.Close()
	log.Println("Clean shutdown: ", msgsSent, " messages sent")
}
