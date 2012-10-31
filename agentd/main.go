package main

import (
	"flag"
	"heka/pipeline"
	"log"
)

func main() {
	configFile := flag.String("config", "agent.conf", "Agent Config file")
	flag.Parse()

	config := new(pipeline.GraterConfig)
	config.Init()
	err := pipeline.LoadFromConfigFile(*configFile, config)
	if err != nil {
		log.Fatal("Error reading config: ", err)
	}

	pipeline.Run(config)
}
