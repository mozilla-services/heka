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
	"time"
)

type Decoder interface {
	Decode(pipelinePack *PipelinePack) error
}

type JsonDecoder struct{}

func (self *JsonDecoder) Init(config interface{}) error {
	return nil
}

func flattenValue(v interface{}, msg *message.Message, path string) error {
	switch v.(type) {
	case string, float64, bool:
		f, _ := message.NewField(path, v, message.Field_RAW)
		msg.AddField(f)
	case []interface{}:
		err := flattenArray(v.([]interface{}), msg, path)
		if err != nil {
			return err
		}
	case map[string]interface{}:
		err := flattenMap(v.(map[string]interface{}), msg, path)
		if err != nil {
			return err
		}
	default:
		return fmt.Errorf("Path %s, unsupported value type: %T", path, v)
	}
	return nil
}

func flattenArray(a []interface{}, msg *message.Message, path string) error {
	if len(a) > 0 {
		switch a[0].(type) {
		case string, float64, bool:
			f, _ := message.NewField(path, a[0], message.Field_RAW)
			for _, v := range a[1:] {
				err := f.AddValue(v)
				if err != nil {
					return err
				}
			}
			msg.AddField(f)

		default:
			var childPath string
			for i, v := range a {
				childPath = fmt.Sprintf("%s.%d", path, i)
				err := flattenValue(v, msg, childPath)
				if err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func flattenMap(m map[string]interface{}, msg *message.Message, path string) error {
	var childPath string
	for k, v := range m {
		if len(path) == 0 {
			childPath = k
		} else {
			childPath = fmt.Sprintf("%s.%s", path, k)
		}
		err := flattenValue(v, msg, childPath)
		if err != nil {
			return err
		}
	}
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
	fields, _ := msgJson.Get("fields").Map()
	err = flattenMap(fields, msg, "")
	if err != nil {
		return err
	}
	return nil
}
