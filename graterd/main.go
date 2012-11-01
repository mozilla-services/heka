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
	"heka/pipeline"
	"log"
	"os"
	"runtime"
	"runtime/pprof"
)

func main() {
	udpAddr := flag.String("udpaddr", "127.0.0.1:5565", "UDP address string")
	maxprocs := flag.Int("maxprocs", 1, "Go runtime MAXPROCS value")
	pprofName := flag.String("pprof", "", "pprof output file path")
	poolSize := flag.Int("poolsize", 1000, "Pipeline pool size")
	decoder := flag.String("decoder", "json", "Default decoder")
	flag.Parse()

	runtime.GOMAXPROCS(*maxprocs)

	if *pprofName != "" {
		profFile, err := os.Create(*pprofName)
		if err != nil {
			log.Fatalln(err)
		}
		pprof.StartCPUProfile(profFile)
		defer pprof.StopCPUProfile()
	}

	config := pipeline.PipelineConfig{}

	udpInput := &pipeline.UdpInput{}
	udpInput.Init(pipeline.UdpInputConfig{*udpAddr})
	var inputs = map[string]pipeline.Input{
		"udp": udpInput,
	}
	config.Inputs = inputs

	jsonDecoder := pipeline.JsonDecoder{}
	gobDecoder := pipeline.GobDecoder{}
	var decoders = map[string]pipeline.Decoder{
		"json": &jsonDecoder,
		"gob":  &gobDecoder,
	}
	config.Decoders = decoders
	config.DefaultDecoder = *decoder

	outputNames := []string{"counter"}
	namedOutputFilter := &pipeline.NamedOutputFilter{}
	namedOutputFilter.Init(map[string][]string{"OutputNames": outputNames})
	config.Filters = map[string]pipeline.Filter{
		"NamedOutputFilter": namedOutputFilter,
	}
	defaultChain := pipeline.FilterChain{Filters: []string{"NamedOutputFilter"}}
	config.FilterChains = map[string]pipeline.FilterChain{
		"default": defaultChain,
	}
	config.DefaultFilterChain = "default"

	counterOutput := pipeline.NewCounterOutput()
	logOutput := pipeline.LogOutput{}
	var outputs = map[string]pipeline.Output{
		"counter": counterOutput,
		"log":     &logOutput,
	}
	config.Outputs = outputs
	config.DefaultOutputs = []string{}
	config.PoolSize = *poolSize

	pipeline.Run(&config)
}
