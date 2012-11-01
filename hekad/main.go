package main

import (
	"flag"
	"heka/pipeline"
	"log"
	"runtime"
)

func main() {
	configFile := flag.String("config", "agent.conf", "Agent Config file")
	maxprocs := flag.Int("maxprocs", 1, "Go runtime MAXPROCS value")
	poolSize := flag.Int("poolsize", 1000, "Pipeline pool size")
	flag.Parse()

	runtime.GOMAXPROCS(*maxprocs)

	pipe := pipeline.NewPipelineConfig(*poolSize)
	err := pipe.LoadFromConfigFile(*configFile)
	if err != nil {
		log.Fatal("Error reading config: ", err)
	}
	pipe.Run()
}
