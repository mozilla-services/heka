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
#   Christian Vozar (christian@bellycard.com)
#
# ***** END LICENSE BLOCK *****/

/*

Heka Inject client.

Inject client used to test heka message flow and plugin operations.
Allows for injecting messages with specified message variable into Heka
pipeline.

*/
package main

import (
	"flag"
	"os"
	"time"

	"github.com/mozilla-services/heka/client"
	"github.com/mozilla-services/heka/message"
	"github.com/pborman/uuid"
)

type HekaClient struct {
	client  client.Client
	encoder client.StreamEncoder // e.g. protobufs
	sender  client.Sender        // e.g. tcp
}

// NewHekaClient returns a new HekaClient with pre-defined encoder and sender.
func NewHekaClient(hi string) (hc *HekaClient, err error) {
	hc = &HekaClient{}
	hc.encoder = client.NewProtobufEncoder(nil)
	hc.sender, err = client.NewNetworkSender("tcp", hi)
	if err == nil {
		return hc, nil
	}
	return
}

type InjectData struct {
	mtype    string
	logger   string
	severity int
	payload  string
	pid      int
	hostname string
}

func (hc *HekaClient) injectMessage(m *InjectData) (err error) {
	var stream []byte

	msg := &message.Message{}
	msg.SetTimestamp(time.Now().UnixNano())
	msg.SetUuid(uuid.NewRandom())
	msg.SetType(m.mtype)
	msg.SetLogger(m.logger)
	msg.SetPid(int32(m.pid))
	msg.SetSeverity(int32(m.severity))
	msg.SetHostname(m.hostname)
	msg.SetPayload(string(m.payload))

	err = hc.encoder.EncodeMessageStream(msg, &stream)
	if err != nil {
		client.LogError.Printf("Inject: [error] encode message: %s\n", err)
	}
	err = hc.sender.SendMessage(stream)
	if err != nil {
		client.LogError.Printf("Inject: [error] send message: %s\n", err)
	}
	return nil
}

func main() {
	flagHekaInstance := flag.String("heka", "127.0.0.1:5565", "Heka instance to inject message")
	flagType := flag.String("type", "inject.message", "Type of message")
	flagLogger := flag.String("logger", "Inject Client", "Data source")
	flagSeverity := flag.Int("severity", 7, "Syslog severity level")
	flagPayload := flag.String("payload", "", "Textual data")
	flagPid := flag.Int("pid", 0, "Process ID generating message")
	flagHostname := flag.String("hostname", "", "Hostname generating message")

	flag.Parse()

	if flag.NFlag() == 0 {
		flag.PrintDefaults()
		os.Exit(0)
	}

	data := &InjectData{
		mtype:    *flagType,
		logger:   *flagLogger,
		severity: *flagSeverity,
		payload:  *flagPayload,
	}

	if *flagPid == 0 {
		data.pid = os.Getpid()
	} else {
		data.pid = *flagPid
	}

	if *flagHostname == "" {
		data.hostname, _ = os.Hostname()
	} else {
		data.hostname = *flagHostname
	}

	hc, err := NewHekaClient(*flagHekaInstance)
	if err == nil {
		err := hc.injectMessage(data)
		if err != nil {
			client.LogError.Printf("Inject: [error] %s\n", err)
		}
	}
}
