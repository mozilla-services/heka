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
#   Mike Trinkala (trink@mozilla.com)
#
# ***** END LICENSE BLOCK *****/

/*

Sandbox Manager Load Test

*/
package main

import (
	"code.google.com/p/go-uuid/uuid"
	"flag"
	"fmt"
	"github.com/bbangert/toml"
	"github.com/mozilla-services/heka/client"
	"github.com/mozilla-services/heka/message"
	"log"
	"os"
	"time"
)

type SbmgrConfig struct {
	IpAddress string                       `toml:"ip_address"`
	Signer    message.MessageSigningConfig `toml:"signer"`
}

func main() {
	configFile := flag.String("config", "sbmgrload.toml", "Sandbox manager load configuration file")
	action := flag.String("action", "load", "load/unload")
	numItems := flag.Int("num", 1, "Number of sandboxes to load/unload")
	flag.Parse()

	code := `
lastTime = os.time() * 1e9
lastCount = 0
count = 0
rate = 0.0
rates = {}

function process_message ()
    count = count + 1
    return 0
end

function timer_event(ns)
    local msgsSent = count - lastCount
    if msgsSent == 0 then return end

    local elapsedTime = ns - lastTime
    if elapsedTime == 0 then return end

    lastCount = count
    lastTime = ns
    rate = msgsSent / (elapsedTime / 1e9)
    rates[#rates+1] = rate
    output(string.format("Got %d messages. %0.2f msg/sec", count, rate))
    inject_message()

    local samples = #rates
    if samples == 10 then -- generate a summary every 10 samples
        table.sort(rates)
	     local min = rates[1]
	     local max = rates[samples]
	     local sum = 0
        for i, val in ipairs(rates) do
            sum = sum + val
	     end
        output(string.format("AGG Sum. Min: %0.2f Max: %0.2f Mean: %0.2f", min, max, sum/samples))
        inject_message()
	     rates = {}
    end
end
`
	confFmt := `
[CounterSandbox%d]
type = "SandboxFilter"
message_matcher = "Type == 'hekabench'"
ticker_interval = 1
script_type = "lua"
filename = ""
preserve_data = true
memory_limit = 32767
instruction_limit = 1000
output_limit = 1024
`

	var config SbmgrConfig
	if _, err := toml.DecodeFile(*configFile, &config); err != nil {
		log.Printf("Error decoding config file: %s", err)
		return
	}
	sender, err := client.NewNetworkSender("tcp", config.IpAddress)
	if err != nil {
		log.Fatalf("Error creating sender: %s\n", err.Error())
	}
	encoder := client.NewProtobufEncoder(&config.Signer)
	manager := client.NewClient(sender, encoder)
	hostname, _ := os.Hostname()

	switch *action {
	case "load":
		for i := 0; i < *numItems; i++ {
			conf := fmt.Sprintf(confFmt, i)
			msg := &message.Message{}
			msg.SetType("heka.control.sandbox")
			msg.SetTimestamp(time.Now().UnixNano())
			msg.SetUuid(uuid.NewRandom())
			msg.SetHostname(hostname)
			msg.SetPayload(code)
			f, _ := message.NewField("config", conf, "toml")
			msg.AddField(f)
			f1, _ := message.NewField("action", *action, "")
			msg.AddField(f1)
			err = manager.SendMessage(msg)
			if err != nil {
				log.Printf("Error sending message: %s\n", err.Error())
			}
		}
	case "unload":
		for i := 0; i < *numItems; i++ {
			msg := &message.Message{}
			msg.SetType("heka.control.sandbox")
			msg.SetTimestamp(time.Now().UnixNano())
			msg.SetUuid(uuid.NewRandom())
			msg.SetHostname(hostname)
			f, _ := message.NewField("name", fmt.Sprintf("CounterSandbox%d", i), "")
			msg.AddField(f)
			f1, _ := message.NewField("action", *action, "")
			msg.AddField(f1)
			err = manager.SendMessage(msg)
			if err != nil {
				log.Printf("Error sending message: %s\n", err.Error())
			}
		}

	default:
		log.Printf("Invalid action: %s\n", *action)
	}

}
