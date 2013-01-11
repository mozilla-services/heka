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
package main

import (
	"github.com/mozilla-services/heka/pipeline"
)

func init() {
	pipeline.RegisterPlugin("UdpInput", func() interface{} {
		return new(pipeline.UdpInput)
	})
	pipeline.RegisterPlugin("JsonDecoder", func() interface{} {
		return new(pipeline.JsonDecoder)
	})
	pipeline.RegisterPlugin("MsgPackDecoder", func() interface{} {
		return new(pipeline.MsgPackDecoder)
	})
	pipeline.RegisterPlugin("StatsdUdpInput", func() interface{} {
		return pipeline.RunnerMaker(new(pipeline.StatsdInWriter))
	})
	pipeline.RegisterPlugin("LogOutput", func() interface{} {
		return new(pipeline.LogOutput)
	})
	pipeline.RegisterPlugin("CounterOutput", func() interface{} {
		return new(pipeline.CounterOutput)
	})
	pipeline.RegisterPlugin("FileOutput", func() interface{} {
		return pipeline.RunnerMaker(new(pipeline.FileWriter))
	})
}
