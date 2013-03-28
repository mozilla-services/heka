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
#   Mike Trinkala (trink@mozilla.com)
#
# ***** END LICENSE BLOCK *****/

package pipeline

import (
	"code.google.com/p/go-uuid/uuid"
	"code.google.com/p/goprotobuf/proto"
	"errors"
	"fmt"
	"github.com/bitly/go-simplejson"
	"github.com/mozilla-services/heka/message"
	"log"
	"time"
)

type DecoderRunner interface {
	PluginRunner
	Decoder() Decoder
	Start()
	InChan() chan *PipelinePack
}

type dRunner struct {
	pRunnerBase
	inChan chan *PipelinePack
}

func NewDecoderRunner(name string, decoder Decoder) DecoderRunner {
	inChan := make(chan *PipelinePack, PIPECHAN_BUFSIZE)
	return &dRunner{
		pRunnerBase{name: name, plugin: decoder.(Plugin)},
		inChan,
	}
}

func (dr *dRunner) Decoder() Decoder {
	return dr.plugin.(Decoder)
}

func (dr *dRunner) Start() {
	go func() {
		var pack *PipelinePack

		defer func() {
			if r := recover(); r != nil {
				dr.LogError(fmt.Errorf("Decoder '%s' panicked: %s", dr.name, r))
				if pack != nil {
					pack.Recycle()
				}
				dr.Start()
			}
		}()

		var err error
		for pack = range dr.inChan {
			if err = dr.Decoder().Decode(pack); err != nil {
				dr.LogError(err)
				pack.Recycle()
				continue
			}
			pack.Decoded = true
			pack.Config.Router().InChan <- pack
		}
	}()
}

func (dr *dRunner) InChan() chan *PipelinePack {
	return dr.inChan
}

func (dr *dRunner) LogError(err error) {
	log.Printf("Decoder '%s' error: %s", dr.name, err)
}

func (dr *dRunner) LogMessage(msg string) {
	log.Printf("Decoder '%s': %s", dr.name, msg)
}

type Decoder interface {
	Decode(pack *PipelinePack) error
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
	msg.SetUuid(u)
	msg.SetType(msgJson.Get("type").MustString())
	timeStr := msgJson.Get("timestamp").MustString()
	t, err := time.Parse(time.RFC3339Nano, timeStr)
	if err != nil {
		log.Printf("Timestamp parsing error: %s\n", err.Error())
		return errors.New("invalid Timestamp")
	}
	msg.SetTimestamp(t.UnixNano())
	msg.SetLogger(msgJson.Get("logger").MustString())
	msg.SetSeverity(int32(msgJson.Get("severity").MustInt()))
	msg.SetPayload(msgJson.Get("payload").MustString())
	msg.SetEnvVersion(msgJson.Get("env_version").MustString())
	i, _ := msgJson.Get("metlog_pid").Int()
	msg.SetPid(int32(i))
	msg.SetHostname(msgJson.Get("metlog_hostname").MustString())
	fields, _ := msgJson.Get("fields").Map()
	err = flattenMap(fields, msg, "")
	if err != nil {
		return err
	}
	return nil
}

type ProtobufDecoder struct{}

func (self *ProtobufDecoder) Init(config interface{}) error {
	return nil
}

func (self *ProtobufDecoder) Decode(pack *PipelinePack) error {
	err := proto.Unmarshal(pack.MsgBytes, pack.Message)
	if err != nil {
		return fmt.Errorf("unmarshaling error: ", err)
	}
	return nil
}
