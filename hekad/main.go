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
	configFile := flag.String("config", "agent.conf", "Agent Config file")
	maxprocs := flag.Int("maxprocs", 1, "Go runtime MAXPROCS value")
	poolSize := flag.Int("poolsize", 1000, "Pipeline pool size")
	pprofName := flag.String("pprof", "", "Go profiler output file")
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

	pipe := pipeline.NewPipelineConfig(*poolSize)
	err := pipe.LoadFromConfigFile(*configFile)
	if err != nil {
		log.Fatal("Error reading config: ", err)
	}
	pipe.Run()
}
