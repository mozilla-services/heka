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
	VERSION = "0.2.0"
)

func main() {
	configFile := flag.String("config", "/etc/hekad.json", "Config file")
	maxprocs := flag.Int("maxprocs", 1, "Go runtime MAXPROCS value")
	poolSize := flag.Int("poolsize", 1000, "Pipeline pool size")
	cpuProfName := flag.String("cpuprof", "", "Go CPU profiler output file")
	memProfName := flag.String("memprof", "", "Go memory profiler output file")
	version := flag.Bool("version", false, "Output version and exit")
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
	pipeconf := pipeline.NewPipelineConfig(*poolSize)
	err := pipeconf.LoadFromConfigFile(*configFile)
	if err != nil {
		log.Fatal("Error reading config: ", err)
	}
	pipeline.Run(pipeconf)
}
