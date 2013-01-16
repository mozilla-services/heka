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
	VERSION = "0.1"
)

func main() {
	configFile := flag.String("config", "/etc/hekad.json", "Config file")
	maxprocs := flag.Int("maxprocs", 1, "Go runtime MAXPROCS value")
	poolSize := flag.Int("poolsize", 1000, "Pipeline pool size")
	pprofName := flag.String("pprof", "", "Go profiler output file")
	version := flag.Bool("version", false, "Output version and exit")
	flag.Parse()

	if *version {
		fmt.Println(VERSION)
		os.Exit(0)
	}

	runtime.GOMAXPROCS(*maxprocs)

	if *pprofName != "" {
		profFile, err := os.Create(*pprofName)
		if err != nil {
			log.Fatalln(err)
		}
		pprof.StartCPUProfile(profFile)
		defer pprof.StopCPUProfile()
	}

	// Set up and load the pipeline configuration and start the daemon.
	pipeconf := pipeline.NewPipelineConfig(*poolSize)
	err := pipeconf.LoadFromConfigFile(*configFile)
	if err != nil {
		log.Fatal("Error reading config: ", err)
	}
	pipeline.Run(pipeconf)
}
