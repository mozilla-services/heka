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
	"fmt"
	"github.com/mozilla-services/heka/pipeline"
	"time"
	//pipeline_ts "github.com/mozilla-services/heka/pipeline/testsupport"
	gs "github.com/rafrombrc/gospec/src/gospec"
)

func PayloadEncoderSpec(c gs.Context) {

	c.Specify("A PayloadEncoder", func() {
		encoder := new(PayloadEncoder)
		config := encoder.ConfigStruct().(*PayloadEncoderConfig)
		tsFormat := "[2006/Jan/02:15:04:05 -0700]"
		supply := make(chan *pipeline.PipelinePack, 1)
		pack := pipeline.NewPipelinePack(supply)
		payload := "This is the payload!"
		pack.Message.SetPayload(payload)
		ts := time.Now()
		pack.Message.SetTimestamp(ts.UnixNano())

		var (
			output []byte
			err    error
		)

		c.Specify("works with default config options", func() {
			err = encoder.Init(config)
			c.Expect(err, gs.IsNil)

			output, err = encoder.Encode(pack)
			c.Expect(err, gs.IsNil)
			c.Expect(string(output), gs.Equals, fmt.Sprintln(payload))
		})

		c.Specify("honors append_newlines = false", func() {
			config.AppendNewlines = false
			err = encoder.Init(config)
			c.Expect(err, gs.IsNil)

			output, err = encoder.Encode(pack)
			c.Expect(err, gs.IsNil)
			c.Expect(string(output), gs.Equals, payload)
		})

		c.Specify("prefixes timestamp", func() {
			config.PrefixTs = true
			err = encoder.Init(config)
			c.Expect(err, gs.IsNil)

			output, err = encoder.Encode(pack)
			c.Expect(err, gs.IsNil)
			formattedTime := ts.Format(tsFormat)
			expected := fmt.Sprintf("%s %s\n", formattedTime, payload)
			c.Expect(string(output), gs.Equals, expected)
		})

		c.Specify("supports alternate time format", func() {
			config.PrefixTs = true
			config.TsFormat = "%a, %d %b %Y %H:%M:%S %Z"
			err = encoder.Init(config)
			c.Expect(err, gs.IsNil)

			output, err = encoder.Encode(pack)
			c.Expect(err, gs.IsNil)
			formattedTime := ts.Format(time.RFC1123)
			expected := fmt.Sprintf("%s %s\n", formattedTime, payload)
			c.Expect(string(output), gs.Equals, expected)
		})
	})
}
