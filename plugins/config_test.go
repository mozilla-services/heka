/***** BEGIN LICENSE BLOCK *****
# This Source Code Form is subject to the terms of the Mozilla Public
# License, v. 2.0. If a copy of the MPL was not distributed with this file,
# You can obtain one at http://mozilla.org/MPL/2.0/.
#
# The Initial Developer of the Original Code is the Mozilla Foundation.
# Portions created by the Initial Developer are Copyright (C) 2012-2015
# the Initial Developer. All Rights Reserved.
#
# Contributor(s):
#   Rob Miller (rmiller@mozilla.com)
#   Justin Judd (justin@justinjudd.org)
#
# ***** END LICENSE BLOCK *****/

// These tests should probably be in the pipeline package, but it's much
// easier to test configuration code when there are plugin definitions that
// can be used in the test configs.

package plugins

import (
	. "github.com/mozilla-services/heka/pipeline"
	_ "github.com/mozilla-services/heka/plugins/payload"
	_ "github.com/mozilla-services/heka/plugins/statsd"
	ts "github.com/mozilla-services/heka/plugins/testsupport"
	_ "github.com/mozilla-services/heka/plugins/udp"
	gs "github.com/rafrombrc/gospec/src/gospec"
	"os"
	"path/filepath"
	"runtime"
	"sync"
)

type DefaultsTestOutput struct{}

type DefaultsTestOutputConfig struct {
	MessageMatcher string
	TickerInterval uint
}

const messageMatchStr string = "Type == 'heka.counter-output'"

func (o *DefaultsTestOutput) ConfigStruct() interface{} {
	return &DefaultsTestOutputConfig{
		MessageMatcher: messageMatchStr,
		TickerInterval: 5,
	}
}

func (o *DefaultsTestOutput) Init(config interface{}) error {
	return nil
}

func (o *DefaultsTestOutput) Run(fr FilterRunner, h PluginHelper) (err error) {
	return
}

func LoadFromConfigSpec(c gs.Context) {
	origAvailablePlugins := make(map[string]func() interface{})
	for k, v := range AvailablePlugins {
		origAvailablePlugins[k] = v
	}

	pipeConfig := NewPipelineConfig(nil)

	defer func() {
		AvailablePlugins = origAvailablePlugins
	}()

	c.Assume(pipeConfig, gs.Not(gs.IsNil))

	c.Specify("Config file loading", func() {
		c.Specify("works w/ good config file", func() {
			err := pipeConfig.PreloadFromConfigFile("./testsupport/config_test.toml")
			c.Assume(err, gs.IsNil)
			err = pipeConfig.LoadConfig()
			c.Assume(err, gs.IsNil)

			// We use a set of Expect's rather than c.Specify because the
			// pipeConfig can't be re-loaded per child as gospec will do
			// since each one needs to bind to the same address

			// and the inputs sections load properly with a custom name
			udp, ok := pipeConfig.InputRunners["UdpInput"]
			c.Expect(ok, gs.Equals, true)

			// and the decoders sections load
			_, ok = pipeConfig.DecoderMakers["JsonDecoder"]
			c.Expect(ok, gs.Equals, false)
			_, ok = pipeConfig.DecoderMakers["ProtobufDecoder"]
			c.Expect(ok, gs.Equals, true)

			// and the outputs sections load
			_, ok = pipeConfig.OutputRunners["LogOutput"]
			c.Expect(ok, gs.Equals, true)

			// and the filters sections load
			_, ok = pipeConfig.FilterRunners["sample"]
			c.Expect(ok, gs.Equals, true)

			// and the encoders sections load
			var encoder Encoder
			encoder, ok = pipeConfig.Encoder("PayloadEncoder", "foo")
			c.Expect(ok, gs.Equals, true)
			_, ok = encoder.(*PayloadEncoder)
			c.Expect(ok, gs.Equals, true)

			// Shut down UdpInput to free up the port for future tests.
			udp.Input().Stop()
		})

		c.Specify("works with env variables in config file", func() {
			err := os.Setenv("LOG_ENCODER", "PayloadEncoder")
			defer os.Setenv("LOG_ENCODER", "")
			c.Assume(err, gs.IsNil)
			err = pipeConfig.PreloadFromConfigFile("./testsupport/config_env_test.toml")
			c.Assume(err, gs.IsNil)
			err = pipeConfig.LoadConfig()
			c.Assume(err, gs.IsNil)

			log, ok := pipeConfig.OutputRunners["LogOutput"]
			c.Assume(ok, gs.IsTrue)
			var wg sync.WaitGroup
			wg.Add(1)
			err = log.Start(pipeConfig, &wg)
			c.Assume(err, gs.IsNil)
			close(log.InChan())
			wg.Wait()

			var encoder Encoder
			encoder = log.Encoder()
			_, ok = encoder.(*PayloadEncoder)
			c.Expect(ok, gs.IsTrue)
		})

		c.Specify("returns an error with invalid env variables in config", func() {
			err := pipeConfig.PreloadFromConfigFile("./testsupport/bad_envs/config_1_test.toml")
			c.Expect(err, gs.Equals, ErrMissingCloseDelim)

			err = pipeConfig.PreloadFromConfigFile("./testsupport/bad_envs/config_2_test.toml")
			c.Expect(err, gs.Equals, ErrInvalidChars)

		})

		c.Specify("works w/ decoder defaults", func() {
			err := pipeConfig.PreloadFromConfigFile("./testsupport/config_test_defaults.toml")
			c.Assume(err, gs.IsNil)
			err = pipeConfig.LoadConfig()
			c.Assume(err, gs.Not(gs.IsNil))

			// Only the ProtobufDecoder is loaded
			c.Expect(len(pipeConfig.DecoderMakers), gs.Equals, 1)
		})

		c.Specify("works w/ MultiDecoder", func() {
			err := pipeConfig.PreloadFromConfigFile("./testsupport/config_test_multidecoder.toml")
			c.Assume(err, gs.IsNil)
			err = pipeConfig.LoadConfig()
			c.Assume(err, gs.IsNil)
			hasSyncDecoder := false

			// ProtobufDecoder will always be loaded
			c.Assume(len(pipeConfig.DecoderMakers), gs.Equals, 4)

			// Check that the MultiDecoder actually loaded
			for k, _ := range pipeConfig.DecoderMakers {
				if k == "syncdecoder" {
					hasSyncDecoder = true
					break
				}
			}
			c.Assume(hasSyncDecoder, gs.IsTrue)
		})

		c.Specify("works w/ Nested MultiDecoders", func() {
			err := pipeConfig.PreloadFromConfigFile("./testsupport/config_test_multidecoder_nested.toml")
			c.Assume(err, gs.IsNil)
			err = pipeConfig.LoadConfig()
			c.Assume(err, gs.IsNil)
			hasSyncDecoder := false

			// ProtobufDecoder will always be loaded
			c.Assume(len(pipeConfig.DecoderMakers), gs.Equals, 5)

			// Check that the MultiDecoder actually loaded
			for k, _ := range pipeConfig.DecoderMakers {
				if k == "syncdecoder" {
					hasSyncDecoder = true
					break
				}
			}
			c.Assume(hasSyncDecoder, gs.IsTrue)
		})

		c.Specify("handles Cyclic Nested MultiDecoders correctly", func() {
			err := pipeConfig.PreloadFromConfigFile("./testsupport/config_test_multidecoder_nested_cyclic.toml")
			//circular dependency detected
			c.Assume(err, gs.IsNil)
			err = pipeConfig.LoadConfig()
			c.Assume(err, gs.Not(gs.IsNil))
			c.Expect(err.Error(), ts.StringContains, "circular dependency detected")

			// ProtobufDecoder will always be loaded
			c.Assume(len(pipeConfig.DecoderMakers), gs.Equals, 1)

		})

		c.Specify("explodes w/ bad config file", func() {
			err := pipeConfig.PreloadFromConfigFile("./testsupport/config_bad_test.toml")
			c.Assume(err, gs.IsNil)
			err = pipeConfig.LoadConfig()
			c.Assume(err, gs.Not(gs.IsNil))
			c.Expect(err.Error(), ts.StringContains, "2 errors loading plugins")
			c.Expect(pipeConfig.LogMsgs, gs.ContainsAny,
				gs.Values("No registered plugin type: CounterOutput"))
		})

		c.Specify("handles missing config file correctly", func() {
			err := pipeConfig.PreloadFromConfigFile("no_such_file.toml")
			c.Assume(err, gs.Not(gs.IsNil))
			if runtime.GOOS == "windows" {
				c.Expect(err.Error(), ts.StringContains, "open no_such_file.toml: The system cannot find the file specified.")
			} else {
				c.Expect(err.Error(), ts.StringContains, "open no_such_file.toml: no such file or directory")
			}
		})

		c.Specify("errors correctly w/ bad outputs config", func() {
			err := pipeConfig.PreloadFromConfigFile("./testsupport/config_bad_outputs.toml")
			c.Assume(err, gs.IsNil)
			err = pipeConfig.LoadConfig()
			c.Assume(err, gs.Not(gs.IsNil))
			c.Expect(err.Error(), ts.StringContains, "1 errors loading plugins")
			msg := pipeConfig.LogMsgs[0]
			c.Expect(msg, ts.StringContains, "No registered plugin type:")
		})

		c.Specify("for a DefaultsTestOutput", func() {
			RegisterPlugin("DefaultsTestOutput", func() interface{} {
				return new(DefaultsTestOutput)
			})
			err := pipeConfig.PreloadFromConfigFile("./testsupport/config_test_defaults2.toml")
			c.Expect(err, gs.IsNil)
			err = pipeConfig.LoadConfig()
			c.Assume(err, gs.IsNil)
			runner, ok := pipeConfig.OutputRunners["DefaultsTestOutput"]
			c.Expect(ok, gs.IsTrue)
			ticker := runner.Ticker()
			c.Expect(ticker, gs.Not(gs.IsNil))
			matcher := runner.MatchRunner().MatcherSpecification().String()
			c.Expect(matcher, gs.Equals, messageMatchStr)
		})

		c.Specify("can render JSON reports as pipe delimited data", func() {
			RegisterPlugin("DefaultsTestOutput", func() interface{} {
				return new(DefaultsTestOutput)
			})
			err := pipeConfig.PreloadFromConfigFile("./testsupport/config_test_defaults2.toml")
			c.Expect(err, gs.IsNil)
			err = pipeConfig.LoadConfig()
			c.Assume(err, gs.IsNil)

			data := `{"globals":[{"Name":"inputRecycleChan","InChanCapacity":{"value":"100", "representation":"count"},"InChanLength":{"value":"99", "representation":"count"}},{"Name":"injectRecycleChan","InChanCapacity":{"value":"100", "representation":"count"},"InChanLength":{"value":"98", "representation":"count"}},{"Name":"Router","InChanCapacity":{"value":"50", "representation":"count"},"InChanLength":{"value":"0", "representation":"count"},"ProcessMessageCount":{"value":"26", "representation":"count"}}], "inputs": [{"Name": "TcpInput"}], "decoders": [{"Name":"ProtobufDecoder","InChanCapacity":{"value":"50", "representation":"count"},"InChanLength":{"value":"0", "representation":"count"}}], "filters": [{"Name":"OpsSandboxManager","RunningFilters":{"value":"0", "representation":"count"},"ProcessMessageCount":{"value":"0", "representation":"count"},"InChanCapacity":{"value":"50", "representation":"count"},"InChanLength":{"value":"0", "representation":"count"},"MatchChanCapacity":{"value":"50", "representation":"count"},"MatchChanLength":{"value":"0", "representation":"count"},"MatchAvgDuration":{"value":"0", "representation":"ns"}},{"Name":"hekabench_counter","Memory":{"value":"20644", "representation":"B"},"MaxMemory":{"value":"20644", "representation":"B"},"MaxInstructions":{"value":"18", "representation":"count"},"MaxOutput":{"value":"0", "representation":"B"},"ProcessMessageCount":{"value":"0", "representation":"count"},"InjectMessageCount":{"value":"0", "representation":"count"},"ProcessMessageAvgDuration":{"value":"0", "representation":"ns"},"TimerEventAvgDuration":{"value":"78532", "representation":"ns"},"InChanCapacity":{"value":"50", "representation":"count"},"InChanLength":{"value":"0", "representation":"count"},"MatchChanCapacity":{"value":"50", "representation":"count"},"MatchChanLength":{"value":"0", "representation":"count"},"MatchAvgDuration":{"value":"445", "representation":"ns"}}], "outputs": [{"Name":"LogOutput","InChanCapacity":{"value":"50", "representation":"count"},"InChanLength":{"value":"0", "representation":"count"},"MatchChanCapacity":{"value":"50", "representation":"count"},"MatchChanLength":{"value":"0", "representation":"count"},"MatchAvgDuration":{"value":"406", "representation":"ns"}},{"Name":"DashboardOutput","InChanCapacity":{"value":"50", "representation":"count"},"InChanLength":{"value":"0", "representation":"count"},"MatchChanCapacity":{"value":"50", "representation":"count"},"MatchChanLength":{"value":"0", "representation":"count"},"MatchAvgDuration":{"value":"336", "representation":"ns"}}]} `

			report := pipeConfig.FormatTextReport("heka.all-report", data)

			expected := `========[heka.all-report]========

====Globals====
inputRecycleChan:
    InChanCapacity: 100
    InChanLength: 99
injectRecycleChan:
    InChanCapacity: 100
    InChanLength: 98
Router:
    InChanCapacity: 50
    InChanLength: 0
    ProcessMessageCount: 26

====Inputs====
TcpInput:

====Splitters====
NONE


====Decoders====
ProtobufDecoder:
    InChanCapacity: 50
    InChanLength: 0

====Filters====
OpsSandboxManager:
    InChanCapacity: 50
    InChanLength: 0
    MatchChanCapacity: 50
    MatchChanLength: 0
    MatchAvgDuration: 0
    ProcessMessageCount: 0
hekabench_counter:
    InChanCapacity: 50
    InChanLength: 0
    MatchChanCapacity: 50
    MatchChanLength: 0
    MatchAvgDuration: 445
    ProcessMessageCount: 0
    InjectMessageCount: 0
    Memory: 20644
    MaxMemory: 20644
    MaxInstructions: 18
    MaxOutput: 0
    ProcessMessageAvgDuration: 0
    TimerEventAvgDuration: 78532

====Outputs====
LogOutput:
    InChanCapacity: 50
    InChanLength: 0
    MatchChanCapacity: 50
    MatchChanLength: 0
    MatchAvgDuration: 406
DashboardOutput:
    InChanCapacity: 50
    InChanLength: 0
    MatchChanCapacity: 50
    MatchChanLength: 0
    MatchAvgDuration: 336

====Encoders====
NONE

========
`

			c.Expect(report, gs.Equals, expected)
		})

		c.Specify("works w/ bad param config file", func() {
			err := pipeConfig.PreloadFromConfigFile("./testsupport/config_bad_params.toml")
			c.Assume(err, gs.IsNil)
			err = pipeConfig.LoadConfig()
			c.Assume(err, gs.Not(gs.IsNil))
		})

		c.Specify("works w/ common parameters that are not part of the struct", func() {
			err := pipeConfig.PreloadFromConfigFile("./testsupport/config_test_common.toml")
			c.Assume(err, gs.IsNil)

		})

	})

	c.Specify("Config directory helpers", func() {
		globals := pipeConfig.Globals
		globals.BaseDir = "/base/dir"
		globals.ShareDir = "/share/dir"

		c.Specify("PrependBaseDir", func() {
			c.Specify("prepends for relative paths", func() {
				dir := filepath.FromSlash("relative/path")
				result := globals.PrependBaseDir(dir)
				c.Expect(result, gs.Equals, filepath.FromSlash("/base/dir/relative/path"))
			})

			c.Specify("doesn't prepend for absolute paths", func() {
				dir := filepath.FromSlash("/absolute/path")
				result := globals.PrependBaseDir(dir)
				c.Expect(result, gs.Equals, dir)
			})

			if runtime.GOOS == "windows" {
				c.Specify("doesn't prepend for absolute paths with volume", func() {
					dir := "c:\\absolute\\path"
					result := globals.PrependBaseDir(dir)
					c.Expect(result, gs.Equals, dir)
				})
			}
		})

		c.Specify("PrependShareDir", func() {
			c.Specify("prepends for relative paths", func() {
				dir := filepath.FromSlash("relative/path")
				result := globals.PrependShareDir(dir)
				c.Expect(result, gs.Equals, filepath.FromSlash("/share/dir/relative/path"))
			})

			c.Specify("doesn't prepend for absolute paths", func() {
				dir := filepath.FromSlash("/absolute/path")
				result := globals.PrependShareDir(dir)
				c.Expect(result, gs.Equals, dir)
			})

			if runtime.GOOS == "windows" {
				c.Specify("doesn't prepend for absolute path with volume", func() {
					dir := "c:\\absolute\\path"
					result := globals.PrependShareDir(dir)
					c.Expect(result, gs.Equals, dir)
				})
			}
		})
	})

	c.Specify("PluginHelper", func() {
		c.Specify("starts and stops DecoderRunners appropriately", func() {
			err := pipeConfig.PreloadFromConfigFile("./testsupport/config_test.toml")
			c.Assume(err, gs.IsNil)
			err = pipeConfig.LoadConfig()
			c.Assume(err, gs.IsNil)
			// Start two DecoderRunners.
			dr1, ok := pipeConfig.DecoderRunner("ProtobufDecoder", "ProtobufDecoder_1")
			c.Expect(ok, gs.IsTrue)
			dr2, ok := pipeConfig.DecoderRunner("ProtobufDecoder", "ProtobufDecoder_2")
			c.Expect(ok, gs.IsTrue)
			// Stop the second one.
			ok = pipeConfig.StopDecoderRunner(dr2)
			c.Expect(ok, gs.IsTrue)
			// Verify that it's stopped, i.e. InChan is closed.
			_, ok = <-dr2.InChan()
			c.Expect(ok, gs.IsFalse)

			// Verify that dr1 is *not* stopped, i.e. InChan is still open.
			rChan := make(chan *PipelinePack, 1)
			pack := NewPipelinePack(rChan)
			dr1.InChan() <- pack // <-- Failure case means this will panic.

			// Try to stop dr2 again. Shouldn't fail, but ok should be false.
			ok = pipeConfig.StopDecoderRunner(dr2)
			c.Expect(ok, gs.IsFalse)
		})
	})
}
