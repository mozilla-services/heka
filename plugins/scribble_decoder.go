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
#   Rob Miller (rmiller@mozilla.com)
#
# ***** END LICENSE BLOCK *****/

package plugins

import (
	"code.google.com/p/go-uuid/uuid"
	"errors"
	"github.com/mozilla-services/heka/message"
	. "github.com/mozilla-services/heka/pipeline"
	"time"
)

type ScribbleDecoderConfig struct {
	// Map of message field names to message string values. Note that all
	// values *must* be strings. Any specified Pid and Severity field values
	// must be parseable as int32. Any specified Timestamp field value will be
	// parsed against the specified TimestampLayout. All specified user fields
	// will be created as strings.
	MessageFields MessageTemplate `toml:"message_fields"`

	// User specified timestamp layout string, used for parsing a timestamp
	// string into an actual time object. If not specified or it fails to
	// match, all of Go's default time layouts will be tried (see
	// http://golang.org/pkg/time/#pkg-constants).
	TimestampLayout string `toml:"timestamp_layout"`

	// Time zone in which the timestamps in the text are presumed to be in.
	// Should be a location name corresponding to a file in the IANA Time Zone
	// database (e.g. "America/Los_Angeles"), as parsed by Go's
	// `time.LoadLocation()` function (see
	// http://golang.org/pkg/time/#LoadLocation). Defaults to "UTC". Not
	// required if valid time zone info is embedded in every parsed timestamp,
	// since those can be parsed as specified in the `timestamp_layout`.
	TimestampLocation string `toml:"timestamp_location"`
}

type ScribbleDecoder struct {
	messageFields MessageTemplate
	unixnano      int64
	uuid          []byte
}

func (sd *ScribbleDecoder) ConfigStruct() interface{} {
	return new(ScribbleDecoderConfig)
}

func (sd *ScribbleDecoder) Init(config interface{}) (err error) {
	conf := config.(*ScribbleDecoderConfig)
	sd.messageFields = conf.MessageFields

	if timeStr, ok := sd.messageFields["Timestamp"]; ok {
		var loc *time.Location
		if loc, err = time.LoadLocation(conf.TimestampLocation); err != nil {
			return
		}
		var time_ time.Time
		time_, err = message.ForgivingTimeParse(conf.TimestampLayout, timeStr, loc)
		if err != nil {
			return
		}
		sd.unixnano = time_.UnixNano()
		delete(sd.messageFields, "Timestamp")
	}

	if uuidStr, ok := sd.messageFields["Uuid"]; ok {
		if len(uuidStr) == message.UUID_SIZE {
			sd.uuid = []byte(uuidStr)
		} else {
			if sd.uuid = uuid.Parse(uuidStr); sd.uuid == nil {
				return errors.New("Invalid UUID string.")
			}
		}
		delete(sd.messageFields, "Uuid")
	}
	return
}

func (sd *ScribbleDecoder) Decode(pack *PipelinePack) (packs []*PipelinePack, err error) {
	if err = sd.messageFields.PopulateMessage(pack.Message, nil); err != nil {
		return
	}
	if sd.unixnano != 0 {
		pack.Message.SetTimestamp(sd.unixnano)
	}
	if sd.uuid != nil {
		pack.Message.SetUuid(sd.uuid)
	}
	return []*PipelinePack{pack}, nil
}

func init() {
	RegisterPlugin("ScribbleDecoder", func() interface{} {
		return new(ScribbleDecoder)
	})
}
