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

package pipeline

import (
	"github.com/mozilla-services/heka/message"
	ts "github.com/mozilla-services/heka/testsupport"
	gs "github.com/rafrombrc/gospec/src/gospec"
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
	c.Specify("Config file loading", func() {
		origGlobals := Globals

		origDecodersByEncoding := make(map[message.Header_MessageEncoding]string)
		for k, v := range DecodersByEncoding {
			origDecodersByEncoding[k] = v
		}

		origAvailablePlugins := make(map[string]func() interface{})
		for k, v := range AvailablePlugins {
			origAvailablePlugins[k] = v
		}

		pipeConfig := NewPipelineConfig(nil)
		defer func() {
			Globals = origGlobals
			DecodersByEncoding = origDecodersByEncoding
			AvailablePlugins = origAvailablePlugins
		}()

		c.Assume(pipeConfig, gs.Not(gs.IsNil))

		c.Specify("works w/ good config file", func() {
			err := pipeConfig.LoadFromConfigFile("../testsupport/config_test.toml")
			c.Assume(err, gs.IsNil)

			// We use a set of Expect's rather than c.Specify because the
			// pipeConfig can't be re-loaded per child as gospec will do
			// since each one needs to bind to the same address

			// and the decoders are loaded for the right encoding headers
			dSet := pipeConfig.DecoderSet()
			byEncodings, err := dSet.ByEncodings()
			c.Assume(err, gs.IsNil)
			dRunner := byEncodings[message.Header_JSON]
			c.Expect(dRunner, gs.Not(gs.IsNil))
			c.Expect(dRunner.Name(), gs.Equals, "JsonDecoder-0")

			dRunner = byEncodings[message.Header_PROTOCOL_BUFFER]
			c.Expect(dRunner, gs.Not(gs.IsNil))
			c.Expect(dRunner.Name(), gs.Equals, "ProtobufDecoder-0")

			// decoder channels are full
			for _, dChan := range pipeConfig.decoderChannels {
				c.Expect(len(dChan), gs.Equals, Globals().DecoderPoolSize)
			}

			// and the inputs section loads properly with a custom name
			_, ok := pipeConfig.InputRunners["UdpInput"]
			c.Expect(ok, gs.Equals, true)

			// and the decoders sections load
			_, ok = pipeConfig.DecoderWrappers["JsonDecoder"]
			c.Expect(ok, gs.Equals, true)
			_, ok = pipeConfig.DecoderWrappers["ProtobufDecoder"]
			c.Expect(ok, gs.Equals, true)

			// and the outputs section loads
			_, ok = pipeConfig.OutputRunners["LogOutput"]
			c.Expect(ok, gs.Equals, true)

			// and the filters sections loads
			_, ok = pipeConfig.FilterRunners["sample"]
			c.Expect(ok, gs.Equals, true)
		})

		c.Specify("works w/ decoder defaults", func() {
			err := pipeConfig.LoadFromConfigFile("../testsupport/config_test_defaults.toml")
			c.Assume(err, gs.Not(gs.IsNil))

			// Decoders are loaded
			c.Expect(len(pipeConfig.DecoderWrappers), gs.Equals, 2)
			c.Expect(DecodersByEncoding[message.Header_JSON], gs.Equals, "JsonDecoder")
			c.Expect(DecodersByEncoding[message.Header_PROTOCOL_BUFFER], gs.Equals,
				"ProtobufDecoder")
		})
		c.Specify("explodes w/ bad config file", func() {
			err := pipeConfig.LoadFromConfigFile("../testsupport/config_bad_test.toml")
			c.Assume(err, gs.Not(gs.IsNil))
			c.Expect(err.Error(), ts.StringContains, "2 errors loading plugins")
			c.Expect(pipeConfig.logMsgs, gs.ContainsAny, gs.Values("No such plugin: CounterOutput"))
		})

		c.Specify("handles missing config file correctly", func() {
			err := pipeConfig.LoadFromConfigFile("no_such_file.toml")
			c.Assume(err, gs.Not(gs.IsNil))
			c.Expect(err.Error(), ts.StringContains, "open no_such_file.toml: no such file or directory")
		})

		c.Specify("errors correctly w/ bad outputs config", func() {
			err := pipeConfig.LoadFromConfigFile("../testsupport/config_bad_outputs.toml")
			c.Assume(err, gs.Not(gs.IsNil))
			c.Expect(err.Error(), ts.StringContains, "1 errors loading plugins")
			msg := pipeConfig.logMsgs[0]
			c.Expect(msg, ts.StringContains, "No such plugin")
		})

		c.Specify("captures plugin Init() panics", func() {
			RegisterPlugin("PanicOutput", func() interface{} {
				return new(PanicOutput)
			})
			err := pipeConfig.LoadFromConfigFile("../testsupport/config_panic.toml")
			c.Expect(err, gs.Not(gs.IsNil))
		})

		c.Specify("for a DefaultsTestOutput", func() {
			RegisterPlugin("DefaultsTestOutput", func() interface{} {
				return new(DefaultsTestOutput)
			})
			err := pipeConfig.LoadFromConfigFile("../testsupport/config_test_defaults2.toml")
			c.Expect(err, gs.IsNil)
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
			err := pipeConfig.LoadFromConfigFile("../testsupport/config_test_defaults2.toml")
			c.Expect(err, gs.IsNil)

			data := `{"reports":[{"Plugin":"inputRecycleChan","InChanCapacity":{"value":"100", "representation":"count"},"InChanLength":{"value":"99", "representation":"count"}},{"Plugin":"injectRecycleChan","InChanCapacity":{"value":"100", "representation":"count"},"InChanLength":{"value":"98", "representation":"count"}},{"Plugin":"Router","InChanCapacity":{"value":"50", "representation":"count"},"InChanLength":{"value":"0", "representation":"count"},"ProcessMessageCount":{"value":"26", "representation":"count"}},{"Plugin":"JsonDecoder-0","InChanCapacity":{"value":"50", "representation":"count"},"InChanLength":{"value":"0", "representation":"count"}},{"Plugin":"JsonDecoder-1","InChanCapacity":{"value":"50", "representation":"count"},"InChanLength":{"value":"0", "representation":"count"}},{"Plugin":"JsonDecoder-2","InChanCapacity":{"value":"50", "representation":"count"},"InChanLength":{"value":"0", "representation":"count"}},{"Plugin":"JsonDecoder-3","InChanCapacity":{"value":"50", "representation":"count"},"InChanLength":{"value":"0", "representation":"count"}},{"Plugin":"ProtobufDecoder-0","InChanCapacity":{"value":"50", "representation":"count"},"InChanLength":{"value":"0", "representation":"count"}},{"Plugin":"ProtobufDecoder-1","InChanCapacity":{"value":"50", "representation":"count"},"InChanLength":{"value":"0", "representation":"count"}},{"Plugin":"ProtobufDecoder-2","InChanCapacity":{"value":"50", "representation":"count"},"InChanLength":{"value":"0", "representation":"count"}},{"Plugin":"ProtobufDecoder-3","InChanCapacity":{"value":"50", "representation":"count"},"InChanLength":{"value":"0", "representation":"count"}},{"Plugin":"DecoderPool-JsonDecoder","InChanCapacity":{"value":"4", "representation":"count"},"InChanLength":{"value":"4", "representation":"count"}},{"Plugin":"DecoderPool-ProtobufDecoder","InChanCapacity":{"value":"4", "representation":"count"},"InChanLength":{"value":"4", "representation":"count"}},{"Plugin":"OpsSandboxManager","RunningFilters":{"value":"0", "representation":"count"},"ProcessMessageCount":{"value":"0", "representation":"count"},"InChanCapacity":{"value":"50", "representation":"count"},"InChanLength":{"value":"0", "representation":"count"},"MatchChanCapacity":{"value":"50", "representation":"count"},"MatchChanLength":{"value":"0", "representation":"count"},"MatchAvgDuration":{"value":"0", "representation":"ns"}},{"Plugin":"hekabench_counter","Memory":{"value":"20644", "representation":"B"},"MaxMemory":{"value":"20644", "representation":"B"},"MaxInstructions":{"value":"18", "representation":"count"},"MaxOutput":{"value":"0", "representation":"B"},"ProcessMessageCount":{"value":"0", "representation":"count"},"InjectMessageCount":{"value":"0", "representation":"count"},"ProcessMessageAvgDuration":{"value":"0", "representation":"ns"},"TimerEventAvgDuration":{"value":"78532", "representation":"ns"},"InChanCapacity":{"value":"50", "representation":"count"},"InChanLength":{"value":"0", "representation":"count"},"MatchChanCapacity":{"value":"50", "representation":"count"},"MatchChanLength":{"value":"0", "representation":"count"},"MatchAvgDuration":{"value":"445", "representation":"ns"}},{"Plugin":"LogOutput","InChanCapacity":{"value":"50", "representation":"count"},"InChanLength":{"value":"0", "representation":"count"},"MatchChanCapacity":{"value":"50", "representation":"count"},"MatchChanLength":{"value":"0", "representation":"count"},"MatchAvgDuration":{"value":"406", "representation":"ns"}},{"Plugin":"DashboardOutput","InChanCapacity":{"value":"50", "representation":"count"},"InChanLength":{"value":"0", "representation":"count"},"MatchChanCapacity":{"value":"50", "representation":"count"},"MatchChanLength":{"value":"0", "representation":"count"},"MatchAvgDuration":{"value":"336", "representation":"ns"}}]} `

			report := pipeConfig.formatTextReport("heka.all-report", data)

			expected := `========[heka.all-report]========
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
JsonDecoder-0:
    InChanCapacity: 50
    InChanLength: 0
JsonDecoder-1:
    InChanCapacity: 50
    InChanLength: 0
JsonDecoder-2:
    InChanCapacity: 50
    InChanLength: 0
JsonDecoder-3:
    InChanCapacity: 50
    InChanLength: 0
ProtobufDecoder-0:
    InChanCapacity: 50
    InChanLength: 0
ProtobufDecoder-1:
    InChanCapacity: 50
    InChanLength: 0
ProtobufDecoder-2:
    InChanCapacity: 50
    InChanLength: 0
ProtobufDecoder-3:
    InChanCapacity: 50
    InChanLength: 0
DecoderPool-JsonDecoder:
    InChanCapacity: 4
    InChanLength: 4
DecoderPool-ProtobufDecoder:
    InChanCapacity: 4
    InChanLength: 4
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
========
`

			c.Expect(report, gs.Equals, expected)
		})

		c.Specify("works w/ bad param config file", func() {
			err := pipeConfig.LoadFromConfigFile("../testsupport/config_bad_params.toml")
			c.Assume(err, gs.Not(gs.IsNil))
		})

		c.Specify("works w/ common parameters that are not part of the struct", func() {
			err := pipeConfig.LoadFromConfigFile("../testsupport/config_test_common.toml")
			c.Assume(err, gs.IsNil)

		})

	})
}
