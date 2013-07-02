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
	version := flag.Bool("version", false, "Output version and exit")
	flag.Parse()

	if flag.NFlag() == 0 {
		flag.PrintDefaults()
		os.Exit(0)
	}

	if *version {
		fmt.Println(VERSION)
		os.Exit(0)
	}

	config, err := LoadHekadConfig(*configFile)
	if err != nil {
		log.Fatal("Error reading config: ", err)
	}

	maxprocs := config.Maxprocs
	poolSize := config.PoolSize
	decoderPoolSize := config.DecoderPoolSize
	chanSize := config.ChanSize
	cpuProfName := config.CpuProfName
	memProfName := config.MemProfName
	maxMsgLoops := config.MaxMsgLoops
	maxMsgProcessInject := config.MaxMsgProcessInject
	maxMsgTimerInject := config.MaxMsgTimerInject

	runtime.GOMAXPROCS(maxprocs)

	if cpuProfName != "" {
		profFile, err := os.Create(cpuProfName)
		if err != nil {
			log.Fatalln(err)
		}
		profFile.Close()
		pprof.StartCPUProfile(profFile)
		defer pprof.StopCPUProfile()
	}

	if memProfName != "" {
		defer func() {
			profFile, err := os.Create(memProfName)
			if err != nil {
				log.Fatalln(err)
			}
			pprof.WriteHeapProfile(profFile)
			profFile.Close()
		}()
	}

	// Set up and load the pipeline configuration and start the daemon.
	globals := pipeline.DefaultGlobals()
	globals.PoolSize = poolSize
	globals.DecoderPoolSize = decoderPoolSize
	globals.PluginChanSize = chanSize
	globals.MaxMsgLoops = maxMsgLoops
	if globals.MaxMsgLoops == 0 {
		globals.MaxMsgLoops = 1
	}
	globals.MaxMsgProcessInject = maxMsgProcessInject
	globals.MaxMsgTimerInject = maxMsgTimerInject
	pipeconf := pipeline.NewPipelineConfig(globals)

	err = pipeconf.LoadFromConfigFile(*configFile)
	if err != nil {
		log.Fatal("Error reading config: ", err)
	}
	pipeline.Run(pipeconf)
}
