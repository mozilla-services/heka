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

/*

Hekad daemon.

This daemon runs the heka/pipeline Plugin's and runners for a complete
message processing platform.

*/
package main

import (
	"flag"
	"fmt"
	"github.com/mozilla-services/heka/pipeline"
	"log"
	"os"
	"runtime"
	"runtime/pprof"
)

const (
	VERSION = "0.2.0b1"
)

func main() {
	configFile := flag.String("config", "/etc/hekad.toml", "Config file")
	maxprocs := flag.Int("maxprocs", 1, "Go runtime MAXPROCS value")
	poolSize := flag.Int("poolsize", 100, "Pipeline pool size")
	decoderPoolSize := flag.Int("decoder_poolsize", 4, "Decoder pool size")
	chanSize := flag.Int("plugin_chansize", 50, "Plugin input channel buffer size")
	cpuProfName := flag.String("cpuprof", "", "Go CPU profiler output file")
	memProfName := flag.String("memprof", "", "Go memory profiler output file")
	version := flag.Bool("version", false, "Output version and exit")
	maxMsgLoops := flag.Uint("max_message_loops", 4, "Maximum number of times a message can pass thru the system")
	flag.Parse()

	if *version {
		fmt.Println(VERSION)
		os.Exit(0)
	}

	runtime.GOMAXPROCS(*maxprocs)

	if *cpuProfName != "" {
		profFile, err := os.Create(*cpuProfName)
		if err != nil {
			log.Fatalln(err)
		}
		pprof.StartCPUProfile(profFile)
		defer pprof.StopCPUProfile()
	}

	if *memProfName != "" {
		defer func() {
			profFile, err := os.Create(*memProfName)
			if err != nil {
				log.Fatalln(err)
			}
			pprof.WriteHeapProfile(profFile)
			profFile.Close()
		}()
	}

	// Set up and load the pipeline configuration and start the daemon.
	globals := pipeline.DefaultGlobals()
	globals.PoolSize = *poolSize
	globals.DecoderPoolSize = *decoderPoolSize
	globals.PluginChanSize = *chanSize
	globals.MaxMsgLoops = *maxMsgLoops
	if globals.MaxMsgLoops == 0 {
		globals.MaxMsgLoops = 1
	}
	pipeconf := pipeline.NewPipelineConfig(globals)
	err := pipeconf.LoadFromConfigFile(*configFile)
	if err != nil {
		log.Fatal("Error reading config: ", err)
	}
	pipeline.Run(pipeconf)
}
