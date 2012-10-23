package agent

import (
	"encoding/json"
	"log"
	"os"
)

type Streams struct {
	Json chan *[]byte
	Stat chan *Packet
	Quit chan int
}

type statsdConfig struct {
	Host      string
	Flush     int64
	Threshold int
}

type agentConfig struct {
	Statsd statsdConfig
	Inputs []InputConfig
}

func ReadConfigFile(filename string) (*agentConfig, error) {
	file, err := os.Open(filename)
	if err != nil {
		log.Fatal("Unable to open config file: ", err)
	}

	jsonBytes := make([]byte, 1e5)
	n, err := file.Read(jsonBytes)
	if err != nil {
		log.Fatal("Unable to load config file: ", err)
	}
	jsonBytes = jsonBytes[:n]

	config := &agentConfig{Statsd: statsdConfig{"127.0.0.1:8125", 10, 90}}
	err = json.Unmarshal(jsonBytes, config)
	if err != nil {
		log.Fatal("Unable to parse config: ", err)
	}
	return config, err
}
