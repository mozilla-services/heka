/***** BEGIN LICENSE BLOCK *****
# This Source Code Form is subject to the terms of the Mozilla Public
# License, v. 2.0. If a copy of the MPL was not distributed with this file,
# You can obtain one at http://mozilla.org/MPL/2.0/.
#
# The Initial Developer of the Original Code is the Mozilla Foundation.
# Portions created by the Initial Developer are Copyright (C) 2012
# the Initial Developer. All Rights Reserved.
#
# Contributor(s):
#   Victor Ng (vng@mozilla.com)
#
#***** END LICENSE BLOCK *****/

package process

import (
	"fmt"
	. "github.com/mozilla-services/heka/pipeline"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type ProcessDirectoryInputConfig struct {
	ScheduledJobDir string `toml:"scheduled_jobs"`
}

type ProcessDirectoryInput struct {
	plugins  map[string]interface{}
	stopChan chan bool
	cron_dir string
	ir       InputRunner
	h        PluginHelper
}

func (pdi *ProcessDirectoryInput) Init(config interface{}) (err error) {
	conf := config.(*ProcessDirectoryInputConfig)
	pdi.cron_dir = GetHekaConfigDir(conf.ScheduledJobDir)

	fmt.Printf("ScheduledJobDir: [%s]\n", conf.ScheduledJobDir)
	fmt.Printf("CronDir : [%s]\n", pdi.cron_dir)

	pdi.plugins = make(map[string]interface{})
	pdi.stopChan = make(chan bool)
	return
}

// ConfigStruct implements the HasConfigStruct interface and sets
// defaults.
func (pdi *ProcessDirectoryInput) ConfigStruct() interface{} {
	return &ProcessDirectoryInputConfig{}
}

func (pdi *ProcessDirectoryInput) Run(ir InputRunner, h PluginHelper) (err error) {
	pdi.ir = ir
	pdi.h = h

	fmt.Printf("Walking from root of: [%s]\n", pdi.cron_dir)
	filepath.Walk(pdi.cron_dir, pdi.CronWalk)

	fmt.Printf("Waiting for stopchan in pdi\n")
	select {
	case <-pdi.stopChan:
		// Just halt
	}

	return
}

func (pdi *ProcessDirectoryInput) Stop() {
	close(pdi.stopChan)
}

// CleanupForRestart implements the Restarting interface.
func (pdi *ProcessDirectoryInput) CleanupForRestart() {
	pdi.Stop()
}

func (pdi *ProcessDirectoryInput) CronWalk(path string, info os.FileInfo, err error) error {
	var ok bool
	var pluginGlobals PluginGlobals

	if info == nil {
		// info will be nil in the case that the filepath doesn't
		// actually exist
		fmt.Printf("filepath doesn't exist skipping: [%s]\n", path)
		return nil
	}

	cron_dir := pdi.cron_dir

	pluginGlobals.Retries = RetryOptions{
		MaxDelay:   "30s",
		Delay:      "250ms",
		MaxRetries: -1,
	}

	if info.IsDir() {
		// Skip directories
		fmt.Printf("Skipping: [%s]\n", path)
		return nil
	}

	// Make sure that the path doesn't get deeper than
	// one level past cron_dir.
	// It should match this pattern: /cron_dir/<time_interval>/
	dir, scriptName := filepath.Split(path)

	fmt.Printf("script name is: [%s]\n", scriptName)
	if strings.HasPrefix(scriptName, ".") {
		// Skip hidden files
		fmt.Printf("Skipping: [%s]\n", path)
		return nil
	}

	dir = strings.TrimSuffix(dir, string(os.PathSeparator))

	actual_cron_dir, time_interval := filepath.Split(dir)
	actual_cron_dir = strings.TrimSuffix(actual_cron_dir, string(os.PathSeparator))
	if actual_cron_dir != cron_dir {
		fmt.Printf("Checking scirpt path of : [%s]\n", path)
		fmt.Printf("Invalid directory structure.  Expected [%s] got [%s]",
			cron_dir, actual_cron_dir)
		return fmt.Errorf("Invalid directory structure.  Expected [%s] got [%s]",
			cron_dir, actual_cron_dir)
	}

	wrapper := new(PluginWrapper)
	sectionName := path
	wrapper.Name = sectionName

	if wrapper.PluginCreator, ok = AvailablePlugins["ProcessInput"]; !ok {
		fmt.Printf("No such plugin: %s", wrapper.Name)
		pdi.ir.LogMessage(fmt.Sprintf("No such plugin: %s", wrapper.Name))
		// This shouldn't happen unless heka was build without
		// ProcessInput support for some reason
		return fmt.Errorf("No such plugin: %s", wrapper.Name)
	}

	fmt.Printf("Initializig processinput\n")
	// Create plugin, test config object generation.
	plugin := wrapper.PluginCreator()
	config := plugin.(*ProcessInput).ConfigStruct().(*ProcessInputConfig)
	// Set the ProcessInput name to be the full path to the script
	config.Name = sectionName

	wrapper.ConfigCreator = func() interface{} { return config }

	// Apply configuration to instantiated plugin.
	cfg := cmd_config{}
	cfg.Bin = path
	config.Command = make(map[string]cmd_config)
	config.Command["0"] = cfg

	if err = plugin.(*ProcessInput).Init(config); err != nil {
		pdi.ir.LogMessage(fmt.Sprintf("Initialization failed for '%s': %s",
			sectionName, err))
	}

	// Always set the ticker_interval value to be what is specified in
	// the directory path.
	var tick_interval int
	tick_interval, err = strconv.Atoi(time_interval)
	if err != nil {
		return fmt.Errorf("Ticker interval could not be parsed for [%s]", path)
	}
	if tick_interval < 0 {
		return fmt.Errorf("A negative ticker interval was parsed for [%s]", path)
	}
	pluginGlobals.Ticker = uint(tick_interval)

	// Determine the plugin type
	tickLength := time.Duration(pluginGlobals.Ticker) * time.Second

	ir := NewInputRunner(wrapper.Name, plugin.(Input), &pluginGlobals)
	ir.SetTickLength(tickLength)
	fmt.Printf("Adding input runner [%s]\n", wrapper.Name)
	pdi.ir.LogMessage(fmt.Sprintf("Adding input runner [%s]", wrapper.Name))
	err = pdi.h.PipelineConfig().AddInputRunner(ir, wrapper)

	// Keep track of all plugins
	pdi.plugins[wrapper.Name] = ir

	return nil
}

func init() {
	RegisterPlugin("ProcessDirectoryInput", func() interface{} {
		return new(ProcessDirectoryInput)
	})
}
