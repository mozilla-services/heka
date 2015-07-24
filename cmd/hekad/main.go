/***** BEGIN LICENSE BLOCK *****
# This Source Code Form is subject to the terms of the Mozilla Public
# License, v. 2.0. If a copy of the MPL was not distributed with this file,
# You can obtain one at http://mozilla.org/MPL/2.0/.
#
# The Initial Developer of the Original Code is the Mozilla Foundation.
# Portions created by the Initial Developer are Copyright (C) 2012-2015
# the Initial Developer. All Rights Reserved.
#
# Contributor(s):
#   Rob Miller (rmiller@mozilla.com)
#
# ***** END LICENSE BLOCK *****/

/*

Main entry point for the `hekad` daemon. Loads the specified config and calls
`pipeline.Run` to launch the PluginRunners and all additional goroutines.

*/
package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"strconv"
	"strings"
	"syscall"

	"github.com/mozilla-services/heka/message"
	"github.com/mozilla-services/heka/pipeline"
	_ "github.com/mozilla-services/heka/plugins"
	_ "github.com/mozilla-services/heka/plugins/amqp"
	_ "github.com/mozilla-services/heka/plugins/dasher"
	_ "github.com/mozilla-services/heka/plugins/elasticsearch"
	_ "github.com/mozilla-services/heka/plugins/file"
	_ "github.com/mozilla-services/heka/plugins/graphite"
	_ "github.com/mozilla-services/heka/plugins/http"
	_ "github.com/mozilla-services/heka/plugins/irc"
	_ "github.com/mozilla-services/heka/plugins/kafka"
	_ "github.com/mozilla-services/heka/plugins/logstreamer"
	_ "github.com/mozilla-services/heka/plugins/nagios"
	_ "github.com/mozilla-services/heka/plugins/payload"
	_ "github.com/mozilla-services/heka/plugins/process"
	_ "github.com/mozilla-services/heka/plugins/smtp"
	_ "github.com/mozilla-services/heka/plugins/statsd"
	_ "github.com/mozilla-services/heka/plugins/tcp"
	_ "github.com/mozilla-services/heka/plugins/udp"
)

const (
	VERSION = "0.11.0"
)

func setGlobalConfigs(config *HekadConfig) (*pipeline.GlobalConfigStruct, string, string) {
	maxprocs := config.Maxprocs
	poolSize := config.PoolSize
	chanSize := config.ChanSize
	cpuProfName := config.CpuProfName
	memProfName := config.MemProfName
	maxMsgLoops := config.MaxMsgLoops
	maxMsgProcessInject := config.MaxMsgProcessInject
	maxMsgProcessDuration := config.MaxMsgProcessDuration
	maxMsgTimerInject := config.MaxMsgTimerInject

	runtime.GOMAXPROCS(maxprocs)

	globals := pipeline.DefaultGlobals()
	globals.PoolSize = poolSize
	globals.PluginChanSize = chanSize
	globals.MaxMsgLoops = maxMsgLoops
	if globals.MaxMsgLoops == 0 {
		globals.MaxMsgLoops = 1
	}
	globals.MaxMsgProcessInject = maxMsgProcessInject
	globals.MaxMsgProcessDuration = maxMsgProcessDuration
	globals.MaxMsgTimerInject = maxMsgTimerInject
	globals.BaseDir = config.BaseDir
	globals.ShareDir = config.ShareDir
	globals.SampleDenominator = config.SampleDenominator
	globals.Hostname = config.Hostname

	return globals, cpuProfName, memProfName
}

func main() {
	configPath := flag.String("config", filepath.FromSlash("/etc/hekad.toml"),
		"Config file or directory. If directory is specified then all files "+
			"in the directory will be loaded.")
	version := flag.Bool("version", false, "Output version and exit")
	flag.Parse()

	config := &HekadConfig{}
	var err error
	var cpuProfName string
	var memProfName string

	if *version {
		fmt.Println(VERSION)
		os.Exit(0)
	}

	config, err = LoadHekadConfig(*configPath)
	if err != nil {
		pipeline.LogError.Fatal("Error reading config: ", err)
	}
	if config.SampleDenominator <= 0 {
		pipeline.LogError.Fatalln("'sample_denominator' value must be greater than 0.")
	}
	globals, cpuProfName, memProfName := setGlobalConfigs(config)

	if err = os.MkdirAll(globals.BaseDir, 0755); err != nil {
		pipeline.LogError.Fatalf("Error creating 'base_dir' %s: %s", config.BaseDir, err)
	}

	if config.MaxMessageSize > 1024 {
		message.SetMaxMessageSize(config.MaxMessageSize)
	} else if config.MaxMessageSize > 0 {
		pipeline.LogError.Fatalln("Error: 'max_message_size' setting must be greater than 1024.")
	}
	if config.PidFile != "" {
		contents, err := ioutil.ReadFile(config.PidFile)
		if err == nil {
			pid, err := strconv.Atoi(strings.TrimSpace(string(contents)))
			if err != nil {
				pipeline.LogError.Fatalf("Error reading proccess id from pidfile '%s': %s",
					config.PidFile, err)
			}

			process, err := os.FindProcess(pid)

			// on Windows, err != nil if the process cannot be found
			if runtime.GOOS == "windows" {
				if err == nil {
					pipeline.LogError.Fatalf("Process %d is already running.", pid)
				}
			} else if process != nil {
				// err is always nil on POSIX, so we have to send the process
				// a signal to check whether it exists
				if err = process.Signal(syscall.Signal(0)); err == nil {
					pipeline.LogError.Fatalf("Process %d is already running.", pid)
				}
			}
		}
		if err = ioutil.WriteFile(config.PidFile, []byte(strconv.Itoa(os.Getpid())),
			0644); err != nil {

			pipeline.LogError.Fatalf("Unable to write pidfile '%s': %s", config.PidFile, err)
		}
		pipeline.LogInfo.Printf("Wrote pid to pidfile '%s'", config.PidFile)
		defer func() {
			if err = os.Remove(config.PidFile); err != nil {
				pipeline.LogError.Printf("Unable to remove pidfile '%s': %s", config.PidFile, err)
			}
		}()
	}

	if cpuProfName != "" {
		profFile, err := os.Create(cpuProfName)
		if err != nil {
			pipeline.LogError.Fatalln(err)
		}

		pprof.StartCPUProfile(profFile)
		defer func() {
			pprof.StopCPUProfile()
			profFile.Close()
		}()
	}

	if memProfName != "" {
		defer func() {
			profFile, err := os.Create(memProfName)
			if err != nil {
				pipeline.LogError.Fatalln(err)
			}
			pprof.WriteHeapProfile(profFile)
			profFile.Close()
		}()
	}

	// Set up and load the pipeline configuration and start the daemon.
	pipeconf := pipeline.NewPipelineConfig(globals)
	if err = loadFullConfig(pipeconf, configPath); err != nil {
		pipeline.LogError.Fatal("Error reading config: ", err)
	}
	pipeline.Run(pipeconf)
}

func loadFullConfig(pipeconf *pipeline.PipelineConfig, configPath *string) (err error) {
	p, err := os.Open(*configPath)
	if err != nil {
		return fmt.Errorf("error opening file: %s", err.Error())
	}
	fi, err := p.Stat()
	if err != nil {
		return fmt.Errorf("can't stat file: %s", err.Error())
	}

	if fi.IsDir() {
		files, _ := ioutil.ReadDir(*configPath)
		for _, f := range files {
			fName := f.Name()
			if !strings.HasSuffix(fName, ".toml") {
				// Skip non *.toml files in a config dir.
				continue
			}
			err = pipeconf.PreloadFromConfigFile(filepath.Join(*configPath, fName))
			if err != nil {
				break
			}
		}
	} else {
		err = pipeconf.PreloadFromConfigFile(*configPath)
	}
	if err == nil {
		err = pipeconf.LoadConfig()
	}
	return err
}
