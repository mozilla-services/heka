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
#   Victor Ng (vng@mozilla.com)
#
# ***** END LICENSE BLOCK *****/

package payload

import (
	"fmt"
	"github.com/crankycoder/xmlpath"
	. "github.com/mozilla-services/heka/pipeline"
	"strings"
	"time"
)

type PayloadXmlDecoderConfig struct {
	// Regular expression that describes log line format and capture group
	// values.
	XPathMapConfig map[string]string `toml:"xpath_map"`

	// Maps severity strings to their int version
	SeverityMap map[string]int32 `toml:"severity_map"`

	// Keyed to the message field that should be filled in, the value will be
	// interpolated so it can use capture parts from the message match.
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

type PayloadXmlDecoder struct {
	XPathMap        map[string]*xmlpath.Path
	SeverityMap     map[string]int32
	MessageFields   MessageTemplate
	TimestampLayout string
	tzLocation      *time.Location
	dRunner         DecoderRunner
}

func (pxd *PayloadXmlDecoder) ConfigStruct() interface{} {
	return &PayloadXmlDecoderConfig{
		TimestampLayout: "2012-04-23T18:25:43.511Z",
	}
}

func (pxd *PayloadXmlDecoder) Init(config interface{}) (err error) {
	conf := config.(*PayloadXmlDecoderConfig)

	pxd.XPathMap = make(map[string]*xmlpath.Path)
	for capture_name, xpath_expr := range conf.XPathMapConfig {
		pxd.XPathMap[capture_name] = xmlpath.MustCompile(xpath_expr)
	}

	pxd.SeverityMap = make(map[string]int32)
	pxd.MessageFields = make(MessageTemplate)
	if conf.SeverityMap != nil {
		for codeString, codeInt := range conf.SeverityMap {
			pxd.SeverityMap[codeString] = codeInt
		}
	}
	if conf.MessageFields != nil {
		for field, action := range conf.MessageFields {
			pxd.MessageFields[field] = action
		}
	}
	pxd.TimestampLayout = conf.TimestampLayout
	if pxd.tzLocation, err = time.LoadLocation(conf.TimestampLocation); err != nil {
		err = fmt.Errorf("PayloadXmlDecoder unknown timestamp_location '%s': %s",
			conf.TimestampLocation, err)
	}
	return
}

// Heka will call this to give us access to the runner.
func (pxd *PayloadXmlDecoder) SetDecoderRunner(dr DecoderRunner) {
	pxd.dRunner = dr
}

// Matches the given string against the XPath and returns the match result
// and captures
func (pxd *PayloadXmlDecoder) match(s string) (captures map[string]string) {
	captures = make(map[string]string)

	reader := strings.NewReader(s)

	for capture_group, path := range pxd.XPathMap {
		root, err := xmlpath.Parse(reader)
		if err != nil {
			continue
		}

		if value, ok := path.String(root); ok {
			captures[capture_group] = value
		}
		// Reset the reader
		reader.Seek(0, 0)
	}

	return
}

// Runs the message payload against decoder's map of JSONPaths. If
// there's a match, the message will be populated based on the
// decoder's message template, with capture values interpolated into
// the message template values.
func (pxd *PayloadXmlDecoder) Decode(pack *PipelinePack) (packs []*PipelinePack, err error) {
	captures := pxd.match(pack.Message.GetPayload())

	pdh := &PayloadDecoderHelper{
		Captures:        captures,
		dRunner:         pxd.dRunner,
		TimestampLayout: pxd.TimestampLayout,
		TzLocation:      pxd.tzLocation,
		SeverityMap:     pxd.SeverityMap,
	}

	pdh.DecodeTimestamp(pack)
	pdh.DecodeSeverity(pack)

	// Update the new message fields based on the fields we should
	// change and the capture parts
	if err = pxd.MessageFields.PopulateMessage(pack.Message, captures); err == nil {
		packs = []*PipelinePack{pack}
	}
	return
}

func init() {
	RegisterPlugin("PayloadXmlDecoder", func() interface{} {
		return new(PayloadXmlDecoder)
	})
}
