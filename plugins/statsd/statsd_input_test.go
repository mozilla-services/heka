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

package statsd

import (
	"fmt"
	. "github.com/mozilla-services/heka/pipeline"
	pipeline_ts "github.com/mozilla-services/heka/pipeline/testsupport"
	. "github.com/mozilla-services/heka/pipelinemock"
	plugins_ts "github.com/mozilla-services/heka/plugins/testsupport"
	"github.com/rafrombrc/gomock/gomock"
	gs "github.com/rafrombrc/gospec/src/gospec"
	"strconv"
	"sync"
	"testing"
)

func StatsdInputSpec(c gs.Context) {
	t := &pipeline_ts.SimpleT{}
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	pConfig := NewPipelineConfig(nil)
	ith := new(plugins_ts.InputTestHelper)
	ith.Msg = pipeline_ts.GetTestMessage()
	ith.Pack = NewPipelinePack(pConfig.InputRecycleChan())
	ith.PackSupply = make(chan *PipelinePack, 1)

	// Specify localhost, but we're not really going to use the network
	ith.AddrStr = "localhost:55565"
	ith.ResolvedAddrStr = "127.0.0.1:55565"

	// set up mock helper, input runner, and stat accumulator
	ith.MockHelper = NewMockPluginHelper(ctrl)
	ith.MockInputRunner = NewMockInputRunner(ctrl)
	mockStatAccum := NewMockStatAccumulator(ctrl)

	c.Specify("A StatsdInput", func() {
		statsdInput := StatsdInput{}
		config := statsdInput.ConfigStruct().(*StatsdInputConfig)

		config.Address = ith.AddrStr
		err := statsdInput.Init(config)
		c.Assume(err, gs.IsNil)
		realListener := statsdInput.listener
		c.Expect(realListener.LocalAddr().String(), gs.Equals, ith.ResolvedAddrStr)
		realListener.Close()
		mockListener := pipeline_ts.NewMockConn(ctrl)
		statsdInput.listener = mockListener

		ith.MockHelper.EXPECT().StatAccumulator("StatAccumInput").Return(mockStatAccum, nil)
		mockListener.EXPECT().Close()
		mockListener.EXPECT().SetReadDeadline(gomock.Any())

		c.Specify("sends a Stat to the StatAccumulator", func() {
			statName := "sample.count"
			statVal := 303
			msg := fmt.Sprintf("%s:%d|c\n", statName, statVal)
			expected := Stat{statName, strconv.Itoa(statVal), "c", float32(1)}
			mockStatAccum.EXPECT().DropStat(expected).Return(true)
			readCall := mockListener.EXPECT().Read(make([]byte, 512))
			readCall.Return(len(msg), nil)
			readCall.Do(func(msgBytes []byte) {
				copy(msgBytes, []byte(msg))
				statsdInput.Stop()
			})
			var wg sync.WaitGroup
			wg.Add(1)
			go func() {
				err = statsdInput.Run(ith.MockInputRunner, ith.MockHelper)
				c.Expect(err, gs.IsNil)
				wg.Done()
			}()
			wg.Wait()
		})
	})
}

func TestParseMessage(t *testing.T) {
	testData := map[string][]Stat{
		// without sample rate ----------------------------------

		"sample.gauge:123|g": []Stat{{
			"sample.gauge",
			"123",
			"g",
			float32(1),
		}},

		" \tsample.gauge:123|g\n": []Stat{{
			"sample.gauge",
			"123",
			"g",
			float32(1),
		}},

		"sample.count:303|c": []Stat{{
			"sample.count",
			"303",
			"c",
			float32(1),
		}},

		"sample.timer:1234|ms": []Stat{{
			"sample.timer",
			"1234",
			"ms",
			float32(1),
		}},

		"sample.histogram:1234|h": []Stat{{
			"sample.histogram",
			"1234",
			"h",
			float32(1),
		}},

		"sample.meter:1234|m": []Stat{{
			"sample.meter",
			"1234",
			"m",
			float32(1),
		}},

		// with sample rate ----------------------------------

		"sample.count.w.rate:123|c|@0.9": []Stat{{
			"sample.count.w.rate",
			"123",
			"c",
			float32(0.9),
		}},

		"sample.timer.w.rate:1234|ms|@0.5": []Stat{{
			"sample.timer.w.rate",
			"1234",
			"ms",
			float32(0.5),
		}},

		// with multiple stats -------------------------------

		"sample.counter:1234|c\nsample.counter2:2345|c\n": []Stat{{
			"sample.counter",
			"1234",
			"c",
			float32(1),
		}, Stat{
			"sample.counter2",
			"2345",
			"c",
			float32(1),
		}},
	}

	for msg, expected := range testData {
		stats, badLines := parseMessage([]byte(msg + "\n"))
		if len(badLines) != 0 {
			t.Fatalf("valid statsd stat failed to parse: %s", badLines[0])
		}
		if len(stats) != len(expected) {
			t.Fatalf("expected %d Stat objects, got %d", len(expected), len(stats))
		}

		for index, stat := range stats {
			if stat.Bucket != expected[index].Bucket {
				t.Fatalf("expected %s at index %d, got %s",
					expected[index].Bucket, index, stat.Bucket)
			}
			if stat.Value != expected[index].Value {
				t.Fatalf("expected %s at index %d, got %s",
					expected[index].Value, index, stat.Value)
			}
			if stat.Modifier != expected[index].Modifier {
				t.Fatalf("expected %s at index %d, got %s",
					expected[index].Modifier, index, stat.Modifier)
			}
			if stat.Sampling != expected[index].Sampling {
				t.Fatalf("expected %f at index %d, got %f",
					expected[index].Sampling, index, stat.Sampling)
			}
		}
	}
}

func TestParseMessageInvalid(t *testing.T) {
	messages := []string{
		"foo.bar.baz:",
		"foo.bar.baz|",
		"foo.bar.baz:1234|x",
	}

	for _, m := range messages {
		stats, badLines := parseMessage([]byte(m + "\n"))
		if len(stats) > 0 || len(badLines) != 1 || string(badLines[0]) != m {
			t.Fatalf("bad statsd stat successfully parsed: %s", m)
		}
	}
}

func BenchmarkMessageParser(b *testing.B) {
	msg := []byte("sample.gauge:123|g\n")
	for i := 0; i < b.N; i++ {
		parseMessage(msg)
	}
}
