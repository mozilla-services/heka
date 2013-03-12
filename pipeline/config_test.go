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

func LoadFromConfigSpec(c gs.Context) {
	c.Specify("Config file loading", func() {
		origPoolSize := PoolSize
		pipeConfig := NewPipelineConfig(1)
		defer func() {
			PoolSize = origPoolSize
		}()

		c.Assume(pipeConfig, gs.Not(gs.IsNil))

		c.Specify("works w/ good config file", func() {
			err := pipeConfig.LoadFromConfigFile("../testsupport/config_test.json")
			c.Assume(err, gs.IsNil)

			// We use a set of Expect's rather than c.Specify because the
			// pipeConfig can't be re-loaded per child as gospec will do
			// since each one needs to bind to the same address

			// and the decoders are loaded for the right encoding headers
			c.Expect(pipeConfig.DecodersByEncoding()[message.Header_JSON].Name(),
				gs.Equals, "JsonDecoder")
			c.Expect(pipeConfig.DecodersByEncoding()[message.Header_PROTOCOL_BUFFER].Name(),
				gs.Equals, "ProtobufDecoder")

			// and the inputs section loads properly with a custom name
			_, ok := pipeConfig.InputRunners["udp_stats"]
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

		c.Specify("explodes w/ bad config file", func() {
			err := pipeConfig.LoadFromConfigFile("../testsupport/config_bad_test.json")
			c.Assume(err, gs.Not(gs.IsNil))
			c.Expect(err.Error(), ts.StringContains, "1 errors loading inputs")
			msg := pipeConfig.logMsgs[0]
			c.Expect(msg, ts.StringContains, "'udp_stats': ResolveUDPAddr failed")
		})

		c.Specify("handles missing config file correctly", func() {
			err := pipeConfig.LoadFromConfigFile("no_such_file.json")
			c.Assume(err, gs.Not(gs.IsNil))
			c.Expect(err.Error(), ts.StringContains, "Unable to open file")
			c.Expect(err.Error(), ts.StringContains, "no such file or directory")
		})

		c.Specify("errors correctly w/ bad outputs config", func() {
			err := pipeConfig.LoadFromConfigFile("../testsupport/config_bad_outputs.json")
			c.Assume(err, gs.Not(gs.IsNil))
			c.Expect(err.Error(), ts.StringContains, "1 errors loading outputs")
			msg := pipeConfig.logMsgs[0]
			c.Expect(msg, ts.StringContains, "No such plugin")
		})
	})
}
