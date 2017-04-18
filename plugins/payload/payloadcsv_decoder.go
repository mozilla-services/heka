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
#   Nacer Adamou (adamou.nacer@gmail.com)
#
# ***** END LICENSE BLOCK *****/

package payload

import (
	"fmt"
	"encoding/csv"
	. "github.com/mozilla-services/heka/pipeline"
	"strings"
	"time"
)

type PayloadCSVDecoderConfig struct {
	// Mapping between a field and its position in the csv file
	FieldsMapConfig map[string]uint32 `toml:"fields_map"`

	// Field delimiter (set to ',' by default)
	Comma rune `toml:"comma"`

	// Comment character for start of line
	Comment rune `toml:"comment"`

	// Number of expected fields per record
	FieldsPerRecord  int `toml:"fields_per_record"`

	// Allow lazy quotes
	LazyQuotes bool `toml:"lazy_quotes"`

	// Trim leading space
	TrimLeadingSpace bool `toml:"trim_leading_space"`

	// Maps severity strings to their int version
	SeverityMap map[string]int32 `toml:"severity_map"`

	// Keyed to the message field that should be filled in.
	MessageFields MessageTemplate `toml:"message_fields"`

	// User specified timestamp layout string, used for parsing a timestamp
	// string into an actual time object. If not specified or it fails to
	// match, all the default time layout's will be tried.
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

type PayloadCSVDecoder struct {
	FieldsMap map[string]uint32
	Comma rune
	Comment rune
	FieldsPerRecord int
	LazyQuotes bool
	TrimLeadingSpace bool
	MessageFields MessageTemplate
	TimestampLayout string
	tzLocation      *time.Location
	dRunner         DecoderRunner
}

func (pcd *PayloadCSVDecoder) ConfigStruct() interface{} {
	return &PayloadCSVDecoderConfig{
		TimestampLayout: "2012-04-23T18:25:43.511Z",
	}
}

func (pcd *PayloadCSVDecoder) Init(config interface{}) (err error) {
	conf := config.(*PayloadCSVDecoderConfig)

	pcd.FieldsMap = make(map[string]uint32)
	for capture_name, field_pos := range conf.FieldsMapConfig {
		pcd.FieldsMap[capture_name] = field_pos
	}

	pcd.Comma = conf.Comma
	pcd.Comment = conf.Comment
	pcd.FieldsPerRecord = conf.FieldsPerRecord
	pcd.LazyQuotes = conf.LazyQuotes
	pcd.TrimLeadingSpace = conf.TrimLeadingSpace

	pcd.MessageFields = make(MessageTemplate)
	if conf.MessageFields != nil {
		for field, action := range conf.MessageFields {
			pcd.MessageFields[field] = action
		}
	}
	pcd.TimestampLayout = conf.TimestampLayout
	if pcd.tzLocation, err = time.LoadLocation(conf.TimestampLocation); err != nil {
		err = fmt.Errorf("PayloadCSVDecoder unknown timestamp_location '%s': %s",
			conf.TimestampLocation, err)
	}
	return
}

// Heka will call this to give us access to the runner.
func (pcd *PayloadCSVDecoder) SetDecoderRunner(dr DecoderRunner) {
	pcd.dRunner = dr
}

// Matches the given field name against its position in each record and returns
// the match result
func (pcd *PayloadCSVDecoder) match(s string) (captures map[string]string, err error) {
	captures = make(map[string]string)
	err = nil

	reader := csv.NewReader(strings.NewReader(s))
	reader.Comma = pcd.Comma
	reader.Comment = pcd.Comment
	reader.FieldsPerRecord = pcd.FieldsPerRecord
	reader.LazyQuotes = pcd.LazyQuotes
	reader.TrimLeadingSpace = pcd.TrimLeadingSpace

	if record, err := reader.Read(); err == nil {
		for field_name, field_pos := range pcd.FieldsMap {
			if field_pos >= uint32(len(record)) {
				// Out of range field are assigned an empty string.
				captures[field_name] = ""
			} else {
				captures[field_name] = record[field_pos]
			}
		}
	}
	return
}

// Runs the message payload against decoder's map of field's position. If
// there's a match, the message will be populated based on the
// decoder's message template, with capture values interpolated into
// the message template values.
func (pcd *PayloadCSVDecoder) Decode(pack *PipelinePack) (packs []*PipelinePack, err error) {
	var captures map[string]string
	if captures, err = pcd.match(pack.Message.GetPayload()); err != nil {
		return nil, err
	}

	pdh := &PayloadDecoderHelper{
		Captures:        captures,
		dRunner:         pcd.dRunner,
		TimestampLayout: pcd.TimestampLayout,
		TzLocation:      pcd.tzLocation,
	}

	pdh.DecodeTimestamp(pack)
	pdh.DecodeSeverity(pack)


	// Update the new message fields based on the fields we should
	// change and the capture parts
	if err = pcd.MessageFields.PopulateMessage(pack.Message, captures); err == nil {
		packs = []*PipelinePack{pack}
	}
	return
}

func init() {
	RegisterPlugin("PayloadCSVDecoder", func() interface{} {
		return new(PayloadCSVDecoder)
	})
}
