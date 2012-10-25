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
package main

import (
	"flag"
	"heka/client"
	"log"
	"os"
	"os/signal"
	"runtime/pprof"
	"strings"
	"syscall"
	"time"
)

func timerLoop(count *uint64, ticker *time.Ticker) {
	lastTime := time.Now()
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
	addrStr := flag.String("udpaddr", "127.0.0.1:5565", "UDP address string")
	pprofName := flag.String("pprof", "", "pprof output file path")
	encoderName := flag.String("encoder", "json", "Message encoder (json|gob)")
	flag.Parse()

	if *pprofName != "" {
		profFile, err := os.Create(*pprofName)
		if err != nil {
			log.Fatalln(err)
		}
		pprof.StartCPUProfile(profFile)
		defer pprof.StopCPUProfile()
	}

	var err error
	sender, err := client.NewUdpSender(*addrStr)
	if err != nil {
		log.Fatalf("Error creating sender: %s\n", err.Error())
	}
	var encoder client.Encoder
	switch *encoderName {
	case "json":
		encoder = &client.JsonEncoder{}
	case "gob":
		encoder = client.NewGobEncoder()
	}
	timestamp := time.Now()
	hostname, _ := os.Hostname()
	message := client.Message{
		Type: "hekabench", Timestamp: timestamp,
		Logger: "hekabench", Severity: 6,
		Payload: "Test Payload", Env_version: "0.8",
		Pid: os.Getpid(), Hostname: hostname,
	}
	msgBytes, err := encoder.EncodeMessage(&message)

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
		err = sender.SendMessage(msgBytes)
		if err != nil {
			if !strings.Contains(err.Error(), "connection refused") {
				log.Printf("Error sending message: %s\n",
					err.Error())
			}
		} else {
			msgsSent++
		}
	}
	log.Println("Clean shutdown")
}
