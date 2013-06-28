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
	VERSION = "0.3.0"
)

func main() {
	configFile := flag.String("config", "/etc/hekad.toml", "Config file")
	maxprocs := flag.Int("maxprocs", 1, "Go runtime MAXPROCS value")
	poolSize := flag.Int("poolsize", 100, "Pipeline pool size")
	decoderPoolSize := flag.Int("decoder_poolsize", 4, "Default decoder pool size")
	chanSize := flag.Int("plugin_chansize", 50, "Plugin input channel buffer size")
	cpuProfName := flag.String("cpuprof", "", "Go CPU profiler output file")
	memProfName := flag.String("memprof", "", "Go memory profiler output file")
	version := flag.Bool("version", false, "Output version and exit")
	maxMsgLoops := flag.Uint("max_message_loops", 4, "Maximum number of times a message can pass thru the system")
	maxMsgProcessInject := flag.Uint("max_process_inject", 1, "Maximum number of messages that ProcessMessage can inject in a single call")
	maxMsgTimerInject := flag.Uint("max_timer_inject", 10, "Maximum number of messages that TimerEvent can inject in a single call")
	flag.Parse()

	if len(flag.Args()) == 0 {
		flag.PrintDefaults()
		os.Exit(0)
	}

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
		profFile.Close()
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
	globals.MaxMsgProcessInject = *maxMsgProcessInject
	globals.MaxMsgTimerInject = *maxMsgTimerInject
	pipeconf := pipeline.NewPipelineConfig(globals)
	err := pipeconf.LoadFromConfigFile(*configFile)
	if err != nil {
		log.Fatal("Error reading config: ", err)
	}
	pipeline.Run(pipeconf)
}
