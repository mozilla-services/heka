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
	"github.com/mozilla-services/heka/pipeline"
	"github.com/cactus/gostrftime"
	"strings"
	"time"
)

type PayloadEncoder struct {
	config *PayloadEncoderConfig
}

type PayloadEncoderConfig struct {
	AppendNewlines bool   `toml:"append_newlines"`
	PrefixTs       bool   `toml:"prefix_ts"`
	TsFromMessage  bool   `toml:"ts_from_message"`
	TsFormat       string `toml:"ts_format"`
}

func (pe *PayloadEncoder) ConfigStruct() interface{} {
	return &PayloadEncoderConfig{
		AppendNewlines: true,
		TsFormat:       "[%Y/%b/%d:%H:%M:%S %z]",
		TsFromMessage:  true,
	}
}

func (pe *PayloadEncoder) Init(config interface{}) (err error) {
	pe.config = config.(*PayloadEncoderConfig)
	if !strings.HasSuffix(pe.config.TsFormat, " ") {
		pe.config.TsFormat += " "
	}
	return
}

func (pe *PayloadEncoder) Encode(pack *pipeline.PipelinePack) (output []byte, err error) {
	payload := pack.Message.GetPayload()

	if !pe.config.AppendNewlines && !pe.config.PrefixTs {
		// Just the payload, ma'am.
		output = []byte(payload)
		return
	}

	if !pe.config.PrefixTs {
		// Payload + newline.
		output = make([]byte, 0, len(payload)+1)
		output = append(output, []byte(payload)...)
		output = append(output, '\n')
		return
	}

	// We're using a timestamp.
	var tm time.Time
	if pe.config.TsFromMessage {
		tm = time.Unix(0, pack.Message.GetTimestamp())
	} else {
		tm = time.Now()
	}
	ts := gostrftime.Strftime(pe.config.TsFormat, tm)

	// Timestamp + payload [+ optional newline].
	l := len(ts) + len(payload)
	output = make([]byte, 0, l+1)
	output = append(output, []byte(ts)...)
	output = append(output, []byte(payload)...)
	if pe.config.AppendNewlines {
		output = append(output, '\n')
	}
	return
}

func init() {
	pipeline.RegisterPlugin("PayloadEncoder", func() interface{} {
		return new(PayloadEncoder)
	})
}
