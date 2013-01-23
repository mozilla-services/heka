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
	"code.google.com/p/go-uuid/uuid"
	"errors"
	"fmt"
	"github.com/bitly/go-simplejson"
	"github.com/mozilla-services/heka/message"
	"log"
	"strings"
	"time"
)

type Decoder interface {
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
	uuidString, _ := msgJson.Get("uuid").String()
	u := uuid.Parse(uuidString)
	copy(msg.Uuid, u)
	*msg.Type = msgJson.Get("type").MustString()
	timeStr := msgJson.Get("timestamp").MustString()
	t, err := time.Parse(time.RFC3339Nano, timeStr)
	if err != nil {
		log.Printf("Timestamp parsing error: %s\n", err.Error())
		return errors.New("invalid Timestamp")
	}
	*msg.Timestamp = t.UnixNano()
	*msg.Logger = msgJson.Get("logger").MustString()
	*msg.Severity = int32(msgJson.Get("severity").MustInt())
	*msg.Payload, _ = msgJson.Get("payload").String()
	*msg.EnvVersion = msgJson.Get("env_version").MustString()
	i, _ := msgJson.Get("metlog_pid").Int()
	*msg.Pid = int32(i)
	*msg.Hostname, _ = msgJson.Get("metlog_hostname").String()

	fields := msgJson.Get("fields")
	for i := 0; ; i++ {
		fi := fields.GetIndex(i) // no way to check if this is valid
		// so break on the first field index that has no name
		name, err := fi.Get("name").String()
		if err != nil {
			break
		}
		tmp, _ := fi.Get("value_type").String()
		value_type, ok := message.Field_ValueType_value[tmp]
		if ok == false {
			return errors.New("invalid value type")
		}
		valueArrayName := fmt.Sprintf("value_%s", strings.ToLower(tmp))
		tmp, _ = fi.Get("value_format").String()
		value_format, ok := message.Field_ValueFormat_value[tmp]
		if ok == false {
			return errors.New("invalid value format")
		}
		a, err := fi.Get(valueArrayName).Array()
		if err != nil {
			return errors.New("invalid value array")
		}
		f := message.NewFieldInit(name,
			message.Field_ValueType(value_type),
			message.Field_ValueFormat(value_format))
		for _, v := range a {
			err = f.AddValue(v)
			if err != nil {
				return err
			}
		}
		msg.AddField(f)
	}
	return nil
}
