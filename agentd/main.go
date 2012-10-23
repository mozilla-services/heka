package main

import (
	"flag"
	"heka/agent"
	"log"
)

func main() {
	configFile := flag.String("config", "agent.conf", "Agent Config file")
	flag.Parse()

	config, err := agent.ReadConfigFile(*configFile)
	if err != nil {
		log.Fatal("Error reading config: ", err)
	}

	// Setup the system stat aggregator
	statsd := (*config).Statsd
	go agent.StatsdUdpListener(&statsd.Host)
	agent.StatsdMonitor(&statsd.Flush, &statsd.Threshold)
}
