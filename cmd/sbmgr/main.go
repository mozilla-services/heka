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

Sandbox Manager

*/
package main

import (
	"code.google.com/p/go-uuid/uuid"
	"flag"
	"github.com/BurntSushi/toml"
	"github.com/mozilla-services/heka/client"
	"github.com/mozilla-services/heka/message"
	"io/ioutil"
	"log"
	"os"
	"time"
)

type SbmgrConfig struct {
	IpAddress string                       `toml:"ip_address"`
	Signer    message.MessageSigningConfig `toml:"signer"`
}

func main() {
	configFile := flag.String("config", "sbmgr.toml", "Sandbox manager configuration file")
	scriptFile := flag.String("script", "xyz.lua", "Sandbox script file")
	scriptConf := flag.String("scriptconf", "xyz.toml", "Sandbox script configuration file")
	filterName := flag.String("filtername", "filter", "Sandbox filter name (used on unload)")
	action := flag.String("action", "load", "Sandbox manager action")
	flag.Parse()

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
	msg := &message.Message{}
	msg.SetType("heka.control.sandbox")
	msg.SetTimestamp(time.Now().UnixNano())
	msg.SetUuid(uuid.NewRandom())
	msg.SetHostname(hostname)

	switch *action {
	case "load":
		code, err := ioutil.ReadFile(*scriptFile)
		if err != nil {
			log.Printf("Error reading scriptFile: %s\n", err.Error())
			return
		}
		msg.SetPayload(string(code))
		conf, err := ioutil.ReadFile(*scriptConf)
		if err != nil {
			log.Printf("Error reading scriptConf: %s\n", err.Error())
			return
		}
		f, _ := message.NewField("config", string(conf), message.Field_RAW)
		msg.AddField(f)
	case "unload":
		f, _ := message.NewField("name", *filterName, message.Field_RAW)
		msg.AddField(f)
	default:
		log.Printf("Invalid action: %s", *action)
	}

	f1, _ := message.NewField("action", *action, message.Field_RAW)
	msg.AddField(f1)
	err = manager.SendMessage(msg)
	if err != nil {
		log.Printf("Error sending message: %s\n", err.Error())
	}
}
