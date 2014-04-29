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

Heka Sandbox Manager

*/
package main

import (
	"code.google.com/p/go-uuid/uuid"
	"crypto/tls"
	"flag"
	"github.com/bbangert/toml"
	"github.com/mozilla-services/heka/client"
	"github.com/mozilla-services/heka/message"
	"github.com/mozilla-services/heka/plugins/tcp"
	"io/ioutil"
	"log"
	"os"
	"time"
)

type SbmgrConfig struct {
	IpAddress string                       `toml:"ip_address"`
	Signer    message.MessageSigningConfig `toml:"signer"`
	UseTls    bool                         `toml:"use_tls"`
	Tls       tcp.TlsConfig
}

func main() {
	configFile := flag.String("config", "sbmgr.toml", "Sandbox manager configuration file")
	scriptFile := flag.String("script", "xyz.lua", "Sandbox script file")
	scriptConfig := flag.String("scriptconfig", "xyz.toml", "Sandbox script configuration file")
	filterName := flag.String("filtername", "filter", "Sandbox filter name (used on unload)")
	action := flag.String("action", "load", "Sandbox manager action")
	flag.Parse()

	var config SbmgrConfig
	if _, err := toml.DecodeFile(*configFile, &config); err != nil {
		log.Printf("Error decoding config file: %s", err)
		return
	}
	var sender *client.NetworkSender
	var err error
	if config.UseTls {
		var goTlsConfig *tls.Config
		goTlsConfig, err = tcp.CreateGoTlsConfig(&config.Tls)
		if err != nil {
			log.Fatalf("Error creating TLS config: %s\n", err)
		}
		sender, err = client.NewTlsSender("tcp", config.IpAddress, goTlsConfig)
	} else {
		sender, err = client.NewNetworkSender("tcp", config.IpAddress)
	}
	if err != nil {
		log.Fatalf("Error creating sender: %s\n", err.Error())
	}
	encoder := client.NewProtobufEncoder(&config.Signer)
	manager := client.NewClient(sender, encoder)

	hostname, _ := os.Hostname()
	msg := &message.Message{}
	msg.SetLogger("heka-sbmgr")
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
		conf, err := ioutil.ReadFile(*scriptConfig)
		if err != nil {
			log.Printf("Error reading scriptConfig: %s\n", err.Error())
			return
		}
		f, _ := message.NewField("config", string(conf), "toml")
		msg.AddField(f)
	case "unload":
		f, _ := message.NewField("name", *filterName, "")
		msg.AddField(f)
	default:
		log.Printf("Invalid action: %s", *action)
	}

	f1, _ := message.NewField("action", *action, "")
	msg.AddField(f1)
	err = manager.SendMessage(msg)
	if err != nil {
		log.Printf("Error sending message: %s\n", err.Error())
	}
}
