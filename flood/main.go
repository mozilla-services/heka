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
Can be run with several options on the command line to indicate how
the messages should be sent and encoded.

*/
package main

import (
	"code.google.com/p/go-uuid/uuid"
	"flag"
	"fmt"
	"github.com/BurntSushi/toml"
	"github.com/mozilla-services/heka/client"
	"github.com/mozilla-services/heka/message"
	"log"
	"os"
	"os/signal"
	"runtime/pprof"
	"strings"
	"syscall"
	"time"
)

type FloodSection struct {
	IpAddress   string                       `toml:"ip_address"`
	Sender	    string                       `toml:"sender"`
	PprofFile   string                       `toml:"pprof_file"`
	Encoder     string                       `toml:"encoder"`
	NumMessages uint64                       `toml:"num_messages"`
	Signer      message.MessageSigningConfig `toml:"signer"`
}

type FloodConfig map[string]FloodSection

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

func main() {
	configFile := flag.String("config", "flood.toml", "Flood configuration file")
	configSection := flag.String("section", "default", "Configuration section to load")
	flag.Parse()

	var config FloodConfig
	if _, err := toml.DecodeFile(*configFile, &config); err != nil {
		log.Printf("Error decoding config file: %s", err)
		return
	}
	var section FloodSection
	var ok bool
	if section, ok = config[*configSection]; !ok {
		log.Printf("Configuration section: '%s' was not found", *configSection)
		return
	}

	if section.PprofFile != "" {
		profFile, err := os.Create(section.PprofFile)
		if err != nil {
			log.Fatalln(err)
		}
		pprof.StartCPUProfile(profFile)
		defer pprof.StopCPUProfile()
	}

	var err error
	var sender client.Sender
	switch section.Sender {
	case "udp":
		sender, err = client.NewUdpSender(section.IpAddress)
	case "tcp":
		sender, err = client.NewTcpSender(section.IpAddress)
	}
	if err != nil {
		log.Fatalf("Error creating sender: %s\n", err.Error())
	}

	var encoder client.Encoder
	switch section.Encoder {
	case "json":
		encoder = new(client.JsonEncoder)
	case "protobuf":
		encoder = new(client.ProtobufEncoder)
	}

	hostname, _ := os.Hostname()
	message := &message.Message{}
	message.SetType("hekabench")
	message.SetTimestamp(time.Now().UnixNano())
	message.SetUuid(uuid.NewRandom())
	message.SetSeverity(int32(6))
	message.SetEnvVersion("0.8")
	message.SetPid(int32(os.Getpid()))
	message.SetHostname(hostname)
	message.SetPayload(fmt.Sprintf("hekabench: %s", hostname))
	msgBytes, err := encoder.EncodeMessage(message)

	// wait for sigint
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT)
	var msgsSent uint64

	// set up counter loop
	ticker := time.NewTicker(time.Duration(time.Second))
	go timerLoop(&msgsSent, ticker)

	for gotsigint := false; !gotsigint; {
		select {
		case <-sigChan:
			gotsigint = true
			continue
		default:
		}
		if len(section.Signer.Name) != 0 {
			err = sender.SendSignedMessage(msgBytes, &section.Signer)
		} else {
			err = sender.SendMessage(msgBytes)
		}
		if err != nil {
			if !strings.Contains(err.Error(), "connection refused") {
				log.Printf("Error sending message: %s\n",
					err.Error())
			}
		} else {
			msgsSent++
			if section.NumMessages != 0 && msgsSent >= section.NumMessages {
				break
			}
		}
	}
	log.Println("Clean shutdown: ", msgsSent, " messages sent")
}
