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
	c.Specify("The config file can be loaded", func() {
		pipeConfig := NewPipelineConfig(1)
		c.Assume(pipeConfig, gs.Not(gs.IsNil))
		pipeConfig.LoadFromConfigFile("config_test.json")
		c.Expect(pipeConfig.DefaultDecoder, gs.Equals, "JsonDecoder")
	})
}
