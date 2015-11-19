/***** BEGIN LICENSE BLOCK *****
# This Source Code Form is subject to the terms of the Mozilla Public
# License, v. 2.0. If a copy of the MPL was not distributed with this file,
# You can obtain one at http://mozilla.org/MPL/2.0/.
#
# The Initial Developer of the Original Code is the Mozilla Foundation.
# Portions created by the Initial Developer are Copyright (C) 2014
# the Initial Developer. All Rights Reserved.
#
# Contributor(s):
#   Anton Lindström (carlantonlindstrom@gmail.com)
#
# ***** END LICENSE BLOCK *****/

package fluent

import (
	"fmt"
	"strconv"

	"github.com/mozilla-services/heka/message"
	"github.com/mozilla-services/heka/pipeline"
	"github.com/pborman/uuid"

	"github.com/fluent/fluent-logger-golang/fluent"
)

const EMPTY_STRING = ""

type FluentDecoderConfig struct {
	DefaultType   string               `toml:"default_type"`
	FieldMappings *FluentFieldMappings `toml:"field_mappings"`
}

type FluentFieldMappings struct {
	Payload    string
	Logger     string
	Type       string
	EnvVersion string
	Hostname   string
	Pid        string
	Uuid       string
	Severity   string
}

type FluentDecoder struct {
	fieldMappings *FluentFieldMappings
	defaultType   string
}

func (f *FluentDecoder) ConfigStruct() interface{} {
	return &FluentDecoderConfig{
		DefaultType:   "fluent",
		FieldMappings: &FluentFieldMappings{},
	}
}

func (f *FluentDecoder) Init(config interface{}) (err error) {
	conf := config.(*FluentDecoderConfig)
	f.fieldMappings = conf.FieldMappings
	f.defaultType = conf.DefaultType
	return
}

func (f *FluentDecoder) template(msg *message.Message,
	record map[string]interface{}) (err error) {

	for key, rawValue := range record {
		value, ok := rawValue.(string)
		if !ok {
			return fmt.Errorf("Record must be of type map[string]string")
		}

		switch key {
		case EMPTY_STRING:
			continue
		case f.fieldMappings.Logger:
			msg.SetLogger(value)
		case f.fieldMappings.Payload:
			msg.SetPayload(value)
		case f.fieldMappings.Type:
			msg.SetType(value)
		case f.fieldMappings.EnvVersion:
			msg.SetEnvVersion(value)
		case f.fieldMappings.Hostname:
			msg.SetHostname(value)
		case f.fieldMappings.Pid:
			pid, err := strconv.ParseInt(value, 10, 32)
			if err != nil {
				return err
			}
			msg.SetPid(int32(pid))
		case f.fieldMappings.Severity:
			severity, err := strconv.ParseInt(value, 10, 32)
			if err != nil {
				return err
			}
			msg.SetSeverity(int32(severity))
		case f.fieldMappings.Uuid:
			if len(value) == message.UUID_SIZE {
				msg.SetUuid([]byte(value))
			} else if uuidBytes := uuid.Parse(value); uuidBytes != nil {
				msg.SetUuid(uuidBytes)
			} else {
				return fmt.Errorf("Invalid UUID string")
			}
		default:
			message.NewStringField(msg, key, value)
		}
	}

	return nil
}

func (f *FluentDecoder) Decode(pack *pipeline.PipelinePack) (packs []*pipeline.PipelinePack, err error) {
	fluentMessage := &fluent.Message{}
	if _, err := fluentMessage.UnmarshalMsg(pack.MsgBytes); err != nil {
		return nil, err
	}

	pack.Message.SetTimestamp(fluentMessage.Time * 1e9)
	//pack.Message.SetType(f.defaultType)

	record, ok := fluentMessage.Record.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("Record must be of type map[string]string")
	}

	record["tag"] = fluentMessage.Tag

	if err := f.template(pack.Message, record); err != nil {
		return nil, err
	}

	return []*pipeline.PipelinePack{pack}, nil
}

func init() {
	pipeline.RegisterPlugin("FluentDecoder", func() interface{} {
		return new(FluentDecoder)
	})
}
