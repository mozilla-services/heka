package main

import (
	"flag"
	"heka/agent"
)

func main() {
	serviceAddress := flag.String("Statsd address", ":8125", "UDP service address")
	flushInterval := flag.Int64("Statsd flush-interval", 10, "Flush interval")
	percentThreshold := flag.Int("Statsd percent-threshold", 90, "Threshold percent")
	flag.Parse()

	go hekaagent.StatsdUdpListener(serviceAddress)
	hekaagent.StatsdMonitor(flushInterval, percentThreshold)
}
