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
	"heka/grater"
	"runtime"
)

func main() {
	const udpAddr = "127.0.0.1:5565"
	const MAXPROCS = 2
	runtime.GOMAXPROCS(MAXPROCS)

	config := hekagrater.GraterConfig{}

	udpInput := hekagrater.NewUdpInput(udpAddr)
	var inputs = map[string]hekagrater.Input {
		"udp": udpInput,
	}
	config.Inputs = inputs

	jsonDecoder := hekagrater.JsonDecoder{}
	var decoders = map[string]hekagrater.Decoder {
		"json": &jsonDecoder,
	}
	config.Decoders = decoders
	config.DefaultDecoder = "json"

	//logOutput := hekagrater.LogOutput{}
	counterOutput := hekagrater.NewCounterOutput()
	var outputs = map[string]hekagrater.Output {
		"counter": counterOutput,
	}
	config.Outputs = outputs

	hekagrater.Run(&config)
}
