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
	gs "github.com/orfjackal/gospec/src/gospec"
	"github.com/orfjackal/gospec/src/gospec"
)

func LoadFromConfigSpec(c gospec.Context) {
	c.Specify("The good config file can be loaded", func() {
		pipeConfig := NewPipelineConfig(1)
		c.Assume(pipeConfig, gs.Not(gs.IsNil))
		pipeConfig.LoadFromConfigFile("config_test.json")

		c.Specify("and the default decoder is loaded", func() {
			c.Expect(pipeConfig.DefaultDecoder, gs.Equals, "JsonDecoder")
		})

		c.Specify("and the inputs section loads properly", func() {
			_, ok := pipeConfig.Inputs["udp_stats"]
			c.Expect(ok, gs.Equals, true)
		})

		c.Specify("and the decoders section loads", func() {
			decoders := pipeConfig.DecoderCreator()
			_, ok := decoders["default"]
			c.Expect(ok, gs.Equals, true)
		})

		c.Specify("and the filters section loads", func() {
			filters := pipeConfig.FilterCreator()
			_, ok := filters["StatRollupFilter"]
			c.Expect(ok, gs.Equals, true)
		})

		c.Specify("and the outputs section loads", func() {
			outputs := pipeConfig.OutputCreator()
			_, ok := outputs["CounterOutput"]
			c.Expect(ok, gs.Equals, true)
		})
	})
}
