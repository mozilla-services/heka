package main

import (
	"flag"
	"heka/pipeline"
	"log"
	"os"
	"runtime"
	"runtime/pprof"
)

type PluginLoader struct{}

// Interface that acts as a flag indicating whether or not the PluginLoader
// should load plugins that have been defined in other packages.
type LoadsExternalPlugins interface {
	LoadExternalPlugins()
}

// Checks to see if provided object has the `LoadExternalPlugins` method and
// calls it to load any plugins defined in external packages.
func tryLoadExternalPlugins(maybeLoader interface{}) {
	pluginLoader, ok := maybeLoader.(LoadsExternalPlugins)
	if ok {
		pluginLoader.LoadExternalPlugins()
	}
}

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

	// Register any external plugins w/ the `AvailablePlugins` map.
	pluginLoader := new(PluginLoader)
	tryLoadExternalPlugins(pluginLoader)

	// Set up and load the pipeline configuration and start the daemon.
	pipeconf := pipeline.NewPipelineConfig(*poolSize)
	err := pipeconf.LoadFromConfigFile(*configFile)
	if err != nil {
		log.Fatal("Error reading config: ", err)
	}
	pipeconf.Run()
}
