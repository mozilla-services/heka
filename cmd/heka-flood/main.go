/***** BEGIN LICENSE BLOCK *****
# This Source Code Form is subject to the terms of the Mozilla Public
# License, v. 2.0. If a copy of the MPL was not distributed with this file,
# You can obtain one at http://mozilla.org/MPL/2.0/.
#
# The Initial Developer of the Original Code is the Mozilla Foundation.
# Portions created by the Initial Developer are Copyright (C) 2012-2014
# the Initial Developer. All Rights Reserved.
#
# Contributor(s):
#   Rob Miller (rmiller@mozilla.com)
#   Mike Trinkala (trink@mozilla.com)
#   Christian Vozar (christian@bellycard.com)
#
# ***** END LICENSE BLOCK *****/

/*

Heka Flood client.

Flooding client used to test heka message through-put and tolerances.
Can be run with several configuration options to indicate how the messages
should be sent and encoded.

*/
package main

import (
	"bytes"
	"crypto/tls"
	"flag"
	"fmt"
	"io"
	"math"
	"math/rand"
	"os"
	"os/signal"
	"runtime"
	"runtime/pprof"
	"syscall"
	"time"

	"github.com/bbangert/toml"
	"github.com/gogo/protobuf/proto"
	"github.com/mozilla-services/heka/client"
	"github.com/mozilla-services/heka/message"
	"github.com/mozilla-services/heka/plugins/tcp"
	"github.com/pborman/uuid"
)

type FloodTest struct {
	NumMessages          uint64                       `toml:"num_messages"`
	StaticMessageSize    uint64                       `toml:"static_message_size"`
	IpAddress            string                       `toml:"ip_address"`
	Sender               string                       `toml:"sender"`
	PprofFile            string                       `toml:"pprof_file"`
	Encoder              string                       `toml:"encoder"`
	MsgInterval          string                       `toml:"message_interval"`
	Signer               message.MessageSigningConfig `toml:"signer"`
	CorruptPercentage    float64                      `toml:"corrupt_percentage"`
	SignedPercentage     float64                      `toml:"signed_percentage"`
	OversizedPercentage  float64                      `toml:"oversized_percentage"`
	VariableSizeMessages bool                         `toml:"variable_size_messages"`
	AsciiOnly            bool                         `toml:"ascii_only"`
	UseTls               bool                         `toml:"use_tls"`
	Tls                  tcp.TlsConfig                `toml:"tls"`
	MaxMessageSize       uint32                       `toml:"max_message_size"`
	ReconnectOnError     bool                         `toml:"reconnect_on_error"`
	ReconnectInterval    int32                        `toml:"reconnect_interval"`
	msgInterval          time.Duration
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
		client.LogInfo.Printf("Sent %d messages. %0.2f msg/sec %0.2f Mbit/sec\n", newCount, msgRate, bitRate)
	}
}

func makeVariableMessage(encoder client.StreamEncoder, items int,
	rdm *randomDataMaker, oversized bool) [][]byte {

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
		msg.SetEnvVersion("0.2")
		msg.SetPid(pid)
		msg.SetHostname(hostname)
		cnt = (rand.Int() % 3) * 1024
		if oversized {
			msg.SetPayload(makePayload(uint64(cnt)+uint64(message.MAX_MESSAGE_SIZE), rdm))
		} else {
			msg.SetPayload(makePayload(uint64(cnt), rdm))
		}
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
			client.LogError.Println(err)
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
}

func makePayload(size uint64, rdm *randomDataMaker) (payload string) {
	hostname, _ := os.Hostname()
	payload = fmt.Sprintf("hekabench: %s", hostname)
	buf := make([]byte, 0, size)
	payloadSuffix := bytes.NewBuffer(buf)
	if _, err := io.CopyN(payloadSuffix, rdm, int64(size)); err == nil {
		payload = fmt.Sprintf("%s - %s", payload, payloadSuffix.String())
	} else {
		client.LogError.Println("Error getting random string: ", err)
	}
	return
}

type OversizedEncoder struct{}

func (o *OversizedEncoder) EncodeMessage(msg *message.Message) ([]byte, error) {
	return proto.Marshal(msg)
}

func (o *OversizedEncoder) EncodeMessageStream(msg *message.Message,
	outBytes *[]byte) (err error) {

	msgBytes, err := o.EncodeMessage(msg)
	if err == nil {
		err = CreateHekaStream(msgBytes, outBytes)
	}
	return
}

func CreateHekaStream(msgBytes []byte, outBytes *[]byte) error {
	h := &message.Header{}
	h.SetMessageLength(uint32(len(msgBytes)))
	headerSize := proto.Size(h)
	requiredSize := message.HEADER_FRAMING_SIZE + headerSize + len(msgBytes)
	if cap(*outBytes) < requiredSize {
		*outBytes = make([]byte, requiredSize)
	} else {
		*outBytes = (*outBytes)[:requiredSize]
	}
	(*outBytes)[0] = message.RECORD_SEPARATOR
	(*outBytes)[1] = uint8(headerSize)
	pbuf := proto.NewBuffer((*outBytes)[message.HEADER_DELIMITER_SIZE:message.HEADER_DELIMITER_SIZE])
	if err := pbuf.Marshal(h); err != nil {
		return err
	}
	(*outBytes)[headerSize+message.HEADER_DELIMITER_SIZE] = message.UNIT_SEPARATOR
	copy((*outBytes)[message.HEADER_FRAMING_SIZE+headerSize:], msgBytes)
	return nil
}

func makeFixedMessage(encoder client.StreamEncoder, size uint64,
	rdm *randomDataMaker) [][]byte {

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
		client.LogError.Println(err)
	}
	ma[0] = stream
	return ma
}

func createSender(test FloodTest) (sender *client.NetworkSender, err error) {
	if test.UseTls {
		var goTlsConfig *tls.Config
		goTlsConfig, err = tcp.CreateGoTlsConfig(&test.Tls)
		if err != nil {
			return
		}
		sender, err = client.NewTlsSender(test.Sender, test.IpAddress, goTlsConfig)
	} else {
		sender, err = client.NewNetworkSender(test.Sender, test.IpAddress)
	}
	return
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
	configFile := flag.String("config", "flood.toml", "Heka Flood configuration file")
	configTest := flag.String("test", "default", "Test section to load")

	flag.Parse()

	if flag.NFlag() == 0 {
		flag.PrintDefaults()
		os.Exit(0)
	}

	var config FloodConfig
	if _, err := toml.DecodeFile(*configFile, &config); err != nil {
		client.LogError.Printf("Error decoding config file: %s", err)
		return
	}
	var test FloodTest
	var ok bool
	if test, ok = config[*configTest]; !ok {
		client.LogError.Printf("Configuration test: '%s' was not found", *configTest)
		return
	}

	if test.MsgInterval != "" {
		var err error
		if test.msgInterval, err = time.ParseDuration(test.MsgInterval); err != nil {
			client.LogError.Printf("Invalid message_interval duration %s: %s", test.MsgInterval,
				err.Error())
			return
		}
	}

	if test.PprofFile != "" {
		profFile, err := os.Create(test.PprofFile)
		if err != nil {
			client.LogError.Fatalln(err)
		}
		pprof.StartCPUProfile(profFile)
		defer pprof.StopCPUProfile()
	}

	if test.MaxMessageSize > 0 {
		message.SetMaxMessageSize(test.MaxMessageSize)
	}
	if test.ReconnectInterval < 1 {
		test.ReconnectInterval = 5
	}

	retryIntervalDuration := time.Duration(test.ReconnectInterval) * time.Second
	var sender *client.NetworkSender
	var err error

	for {
		sender, err = createSender(test)
		if err != nil {
			client.LogError.Printf("Error creating sender: %s\n", err)
			if test.ReconnectOnError {
				client.LogInfo.Println("Attempting to reconnect...")
				time.Sleep(retryIntervalDuration)
				continue
			} else {
				return
			}
		} else {
			break
		}
	}

	unsignedEncoder := client.NewProtobufEncoder(nil)
	signedEncoder := client.NewProtobufEncoder(&test.Signer)
	oversizedEncoder := &OversizedEncoder{}

	var numTestMessages = 1
	var unsignedMessages [][]byte
	var signedMessages [][]byte
	var oversizedMessages [][]byte

	rdm := &randomDataMaker{
		src:       rand.NewSource(time.Now().UnixNano()),
		asciiOnly: test.AsciiOnly,
	}

	if test.VariableSizeMessages {
		numTestMessages = 64
		unsignedMessages = makeVariableMessage(unsignedEncoder, numTestMessages, rdm, false)
		signedMessages = makeVariableMessage(signedEncoder, numTestMessages, rdm, false)
		oversizedMessages = makeVariableMessage(oversizedEncoder, 1, rdm, true)
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
	var msgsSent, bytesSent, msgsDelivered uint64
	var corruptPercentage, lastCorruptPercentage, signedPercentage, lastSignedPercentage, oversizedPercentage, lastOversizedPercentage float64
	var corrupt bool

	// set up counter loop
	ticker := time.NewTicker(time.Duration(time.Second))
	go timerLoop(&msgsSent, &bytesSent, ticker)

	test.CorruptPercentage /= 100.0
	test.SignedPercentage /= 100.0
	test.OversizedPercentage /= 100.0

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
			oversizedPercentage = math.Floor(float64(msgsSent) * test.OversizedPercentage)
			if oversizedPercentage != lastOversizedPercentage {
				lastOversizedPercentage = oversizedPercentage
				buf = oversizedMessages[0]
			} else {
				buf = unsignedMessages[msgId]
			}
		}
		bytesSent += uint64(len(buf))
		if err = sendMessage(sender, buf, corrupt); err != nil {
			client.LogError.Printf("Error sending message: %s\n", err.Error())
			if test.ReconnectOnError {
				for {
					client.LogInfo.Println("Attempting to reconnect...")
					time.Sleep(retryIntervalDuration)
					sender, err = createSender(test)
					if err != nil {
						client.LogError.Printf("Error creating sender: %s\n", err)
					} else {
						break
					}
				}
			} else {
				break
			}
		} else {
			msgsDelivered++
		}
		msgsSent++
		if test.NumMessages != 0 && msgsSent >= test.NumMessages {
			break
		}
		if test.msgInterval != 0 {
			time.Sleep(test.msgInterval)
		}
	}
	sender.Close()
	client.LogInfo.Println("Clean shutdown: ", msgsSent, " messages sent; ",
		msgsDelivered, " messages delivered.")
}
