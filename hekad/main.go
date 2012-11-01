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

	config := new(pipeline.PipelineConfig)
	config.Init()
	config.PoolSize = *poolSize
	err := pipeline.LoadFromConfigFile(*configFile, config)
	if err != nil {
		log.Fatal("Error reading config: ", err)
	}

	pipeline.Run(config)
}
