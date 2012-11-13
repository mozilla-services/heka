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
	"github.com/bitly/go-simplejson"
	"github.com/ugorji/go-msgpack"
	"log"
	"time"
)

// The timezone information has been stripped as 
// everything should be encoded to UTC time
const (
	TimeFormat           = "2006-01-02T15:04:05.000000Z"
	TimeFormatFullSecond = "2006-01-02T15:04:05Z"
)

type Decoder interface {
	Plugin
	Decode(pipelinePack *PipelinePack) error
}

type JsonDecoder struct{}

func (self *JsonDecoder) Init(config interface{}) error {
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
	tmp_time, err := time.Parse(TimeFormat, timeStr)
	msg.Timestamp = tmp_time

	if err != nil {
		tmp_time, err = time.Parse(TimeFormatFullSecond, timeStr)
		msg.Timestamp = tmp_time
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

type MsgPackDecoder struct {
	Buffer  *bytes.Buffer
	Decoder *msgpack.Decoder
}

func (self *MsgPackDecoder) Init(config interface{}) error {
	self.Buffer = new(bytes.Buffer)
	self.Decoder = msgpack.NewDecoder(self.Buffer, nil)
	return nil
}

func (self *MsgPackDecoder) Decode(pipelinePack *PipelinePack) error {
	self.Buffer.Write(pipelinePack.MsgBytes)
	defer self.Buffer.Reset() // Needed? Shouldn't be, unless there's an error.
	if err := self.Decoder.Decode(pipelinePack.Message); err != nil {
		return err
	}
	pipelinePack.Decoded = true
	return nil
}
