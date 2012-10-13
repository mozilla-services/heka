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
	"flag"
	"heka/grater"
	"log"
	"os"
	"runtime"
	"runtime/pprof"
)

func main() {
	udpAddr := flag.String("udpaddr", "127.0.0.1:5565", "UDP address string")
	udpFdInt := flag.Uint64("udpfd", 0, "UDP socket file descriptor")
	maxprocs := flag.Int("maxprocs", 1, "Go runtime MAXPROCS value")
	pprofName := flag.String("pprof", "", "pprof output file path")
	flag.Parse()
	udpFdIntPtr := uintptr(*udpFdInt)

	runtime.GOMAXPROCS(*maxprocs)

	if *pprofName != "" {
		profFile, err := os.Create(*pprofName)
		if err != nil {
			log.Fatalln(err)
		}
		pprof.StartCPUProfile(profFile)
		defer pprof.StopCPUProfile()
	}

	config := hekagrater.GraterConfig{}

	udpInput := hekagrater.NewUdpInput(udpAddr, &udpFdIntPtr)
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

	outputNames := []string{"counter"}
	namedOutputFilter := hekagrater.NewNamedOutputFilter(&outputNames)
	filters := []hekagrater.Filter{namedOutputFilter}
	config.Filters = filters

	counterOutput := hekagrater.NewCounterOutput()
	logOutput := hekagrater.LogOutput{}
	var outputs = map[string]hekagrater.Output {
		"counter": counterOutput,
		"log": &logOutput,
	}
	config.Outputs = outputs
	config.DefaultOutputs = []string{}

	hekagrater.Run(&config)
}
