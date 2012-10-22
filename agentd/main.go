package main

import (
	"flag"
	"heka/agent"
)

func main() {
	serviceAddress := flag.String("Statsd address", "127.0.0.1:8125", "UDP service address")
	flushInterval := flag.Int64("Statsd flush-interval", 10, "Flush interval")
	percentThreshold := flag.Int("Statsd percent-threshold", 90, "Threshold percent")
	flag.Parse()

	go agent.StatsdUdpListener(serviceAddress)
	agent.StatsdMonitor(flushInterval, percentThreshold)
}
