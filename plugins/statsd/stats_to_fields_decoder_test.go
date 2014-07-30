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
#   Victor Ng (vng@mozilla.com)
#
# ***** END LICENSE BLOCK *****/

package statsd

import (
	"fmt"
	. "github.com/mozilla-services/heka/pipeline"
	pipeline_ts "github.com/mozilla-services/heka/pipeline/testsupport"
	"github.com/mozilla-services/heka/pipelinemock"
	"github.com/rafrombrc/gomock/gomock"
	"github.com/rafrombrc/gospec/src/gospec"
	gs "github.com/rafrombrc/gospec/src/gospec"
	"strconv"
	"strings"
)

func StatsToFieldsDecoderSpec(c gospec.Context) {
	t := &pipeline_ts.SimpleT{}
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	globals := &GlobalConfigStruct{
		PluginChanSize: 5,
	}
	config := NewPipelineConfig(globals)

	c.Specify("A StatsToFieldsDecoder", func() {
		decoder := new(StatsToFieldsDecoder)
		dRunner := pipelinemock.NewMockDecoderRunner(ctrl)
		decoder.runner = dRunner

		pack := NewPipelinePack(config.InputRecycleChan())

		mergeStats := func(stats [][]string) string {
			lines := make([]string, len(stats))
			for i, line := range stats {
				lines[i] = strings.Join(line, " ")
			}
			return strings.Join(lines, "\n")
		}

		c.Specify("correctly converts stats to fields", func() {
			stats := [][]string{
				{"stat.one", "1", "1380047333"},
				{"stat.two", "2", "1380047333"},
				{"stat.three", "3", "1380047333"},
				{"stat.four", "4", "1380047333"},
				{"stat.five", "5", "1380047333"},
			}
			pack.Message.SetPayload(mergeStats(stats))
			_, err := decoder.Decode(pack)
			c.Expect(err, gs.IsNil)

			for i, stats := range stats {
				value, ok := pack.Message.GetFieldValue(stats[0])
				c.Expect(ok, gs.IsTrue)
				expected := float64(i + 1)
				c.Expect(value.(float64), gs.Equals, expected)
			}

			value, ok := pack.Message.GetFieldValue("timestamp")
			c.Expect(ok, gs.IsTrue)
			expected, err := strconv.ParseInt(stats[0][2], 0, 32)
			c.Assume(err, gs.IsNil)
			c.Expect(value.(int64), gs.Equals, expected)
		})

		c.Specify("generates multiple messages for multiple timestamps", func() {
			stats := [][]string{
				{"stat.one", "1", "1380047333"},
				{"stat.two", "2", "1380047333"},
				{"stat.three", "3", "1380047331"},
				{"stat.four", "4", "1380047333"},
				{"stat.five", "5", "1380047332"},
			}
			// Prime the pack supply w/ two new packs.
			dRunner.EXPECT().NewPack().Return(NewPipelinePack(nil))
			dRunner.EXPECT().NewPack().Return(NewPipelinePack(nil))

			// Decode and check the main pack.
			pack.Message.SetPayload(mergeStats(stats))
			packs, err := decoder.Decode(pack)
			c.Expect(err, gs.IsNil)
			value, ok := pack.Message.GetFieldValue("timestamp")
			c.Expect(ok, gs.IsTrue)
			expected, err := strconv.ParseInt(stats[0][2], 0, 32)
			c.Assume(err, gs.IsNil)
			c.Expect(value.(int64), gs.Equals, expected)

			// Check the first extra.
			pack = packs[1]
			value, ok = pack.Message.GetFieldValue("timestamp")
			c.Expect(ok, gs.IsTrue)
			expected, err = strconv.ParseInt(stats[2][2], 0, 32)
			c.Assume(err, gs.IsNil)
			c.Expect(value.(int64), gs.Equals, expected)

			// Check the second extra.
			pack = packs[2]
			value, ok = pack.Message.GetFieldValue("timestamp")
			c.Expect(ok, gs.IsTrue)
			expected, err = strconv.ParseInt(stats[4][2], 0, 32)
			c.Assume(err, gs.IsNil)
			c.Expect(value.(int64), gs.Equals, expected)
		})

		c.Specify("fails w/ invalid timestamp", func() {
			stats := [][]string{
				{"stat.one", "1", "1380047333"},
				{"stat.two", "2", "1380047333"},
				{"stat.three", "3", "1380047333c"},
				{"stat.four", "4", "1380047333"},
				{"stat.five", "5", "1380047332"},
			}
			pack.Message.SetPayload(mergeStats(stats))
			packs, err := decoder.Decode(pack)
			c.Expect(len(packs), gs.Equals, 0)
			c.Expect(err, gs.Not(gs.IsNil))
			expected := fmt.Sprintf("invalid timestamp: '%s'",
				strings.Join(stats[2], " "))
			c.Expect(err.Error(), gs.Equals, expected)
		})

		c.Specify("fails w/ invalid value", func() {
			stats := [][]string{
				{"stat.one", "1", "1380047333"},
				{"stat.two", "2", "1380047333"},
				{"stat.three", "3", "1380047333"},
				{"stat.four", "4d", "1380047333"},
				{"stat.five", "5", "1380047332"},
			}
			pack.Message.SetPayload(mergeStats(stats))
			packs, err := decoder.Decode(pack)
			c.Expect(len(packs), gs.Equals, 0)
			c.Expect(err, gs.Not(gs.IsNil))
			expected := fmt.Sprintf("invalid value: '%s'",
				strings.Join(stats[3], " "))
			c.Expect(err.Error(), gs.Equals, expected)
		})
	})
}
