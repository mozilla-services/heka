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
package pipeline

import (
	"bytes"
	"encoding/gob"
	"github.com/bitly/go-simplejson"
	"log"
	"time"
)

type Decoder interface {
	Plugin
	Decode(pipelinePack *PipelinePack) error
}

const (
	timeFormat           = "2006-01-02T15:04:05.000000-07:00"
	timeFormatFullSecond = "2006-01-02T15:04:05-07:00"
)

type JsonDecoder struct {
}

func (self *JsonDecoder) Init(config *PluginConfig) error {
	return nil
}

func (self *JsonDecoder) Decode(pipelinePack *PipelinePack) error {
	msgBytes := pipelinePack.MsgBytes
	msgJson, err := simplejson.NewJson(msgBytes)
	if err != nil {
		return err
	}

	msg := pipelinePack.Message
	msg.Type = msgJson.Get("type").MustString()
	timeStr := msgJson.Get("timestamp").MustString()
	msg.Timestamp, err = time.Parse(timeFormat, timeStr)
	if err != nil {
		msg.Timestamp, err = time.Parse(timeFormatFullSecond, timeStr)
		if err != nil {
			log.Printf("Timestamp parsing error: %s\n", err.Error())
		}
	}
	msg.Logger = msgJson.Get("logger").MustString()
	msg.Severity = msgJson.Get("severity").MustInt()
	msg.Payload, _ = msgJson.Get("payload").String()
	msg.Fields, _ = msgJson.Get("fields").Map()
	msg.Env_version = msgJson.Get("env_version").MustString()
	msg.Pid, _ = msgJson.Get("metlog_pid").Int()
	msg.Hostname, _ = msgJson.Get("metlog_hostname").String()

	pipelinePack.Decoded = true
	return nil
}

type GobDecoder struct {
}

func (self *GobDecoder) Init(config *PluginConfig) error {
	return nil
}

func (self *GobDecoder) Decode(pipelinePack *PipelinePack) error {
	msgBytes := pipelinePack.MsgBytes
	buffer := bytes.NewBuffer(msgBytes)

    // stuffs byte buffer into decoder
	decoder := gob.NewDecoder(buffer) 

    // piplinePack.Message is a pointer type, so writing into msg
	msg := pipelinePack.Message

    // .Decode write decoded message into msg.  Note that the Gob
    // system only uses Message types on Encode and Decode
	err := decoder.Decode(msg)
	if err != nil {
		return err
	}
	pipelinePack.Decoded = true
	return nil
}
