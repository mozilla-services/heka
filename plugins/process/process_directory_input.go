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
#   Rob Miller (rmiller@mozilla.com)
#
#***** END LICENSE BLOCK *****/

package process

import (
	"errors"
	"fmt"
	"github.com/bbangert/toml"
	. "github.com/mozilla-services/heka/pipeline"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type ProcessEntry struct {
	ir     InputRunner
	config *ProcessInputConfig
}

type ProcessDirectoryInputConfig struct {
	// Root folder of the tree where the scheduled jobs are defined.
	ProcessDir string `toml:"process_dir"`

	// Number of seconds to wait between scans of the job directory. Defaults
	// to 300.
	TickerInterval uint `toml:"ticker_interval"`
}

type ProcessDirectoryInput struct {
	// The actual running InputRunners.
	inputs map[string]*ProcessEntry
	// Set of InputRunners that should exist as specified by walking
	// the process directory.
	specified map[string]*ProcessEntry
	stopChan  chan bool
	procDir   string
	ir        InputRunner
	h         PluginHelper
}

func (pdi *ProcessDirectoryInput) Init(config interface{}) (err error) {
	conf := config.(*ProcessDirectoryInputConfig)
	pdi.inputs = make(map[string]*ProcessEntry)
	pdi.stopChan = make(chan bool)
	pdi.procDir = filepath.Clean(PrependShareDir(conf.ProcessDir))
	return
}

// ConfigStruct implements the HasConfigStruct interface and sets defaults.
func (pdi *ProcessDirectoryInput) ConfigStruct() interface{} {
	return &ProcessDirectoryInputConfig{
		ProcessDir:     "processes",
		TickerInterval: 300,
	}
}

func (pdi *ProcessDirectoryInput) Stop() {
	close(pdi.stopChan)
}

// CleanupForRestart implements the Restarting interface.
func (pdi *ProcessDirectoryInput) CleanupForRestart() {
	pdi.Stop()
}

func (pdi *ProcessDirectoryInput) Run(ir InputRunner, h PluginHelper) (err error) {
	pdi.ir = ir
	pdi.h = h
	if err = pdi.loadInputs(); err != nil {
		return
	}

	var ok = true
	ticker := ir.Ticker()

	for ok {
		select {
		case _, ok = <-pdi.stopChan:
		case <-ticker:
			if err = pdi.loadInputs(); err != nil {
				return
			}
		}
	}

	return
}

// Reload the set of running ProcessInput InputRunners. Not reentrant, should
// only be called from the ProcessDirectoryInput's main Run goroutine.
func (pdi *ProcessDirectoryInput) loadInputs() (err error) {
	// Clear out pdi.specified and then populate it from the file tree.
	pdi.specified = make(map[string]*ProcessEntry)
	if err = filepath.Walk(pdi.procDir, pdi.procDirWalkFunc); err != nil {
		return
	}

	// Remove any running inputs that are no longer specified.
	for name, entry := range pdi.inputs {
		if _, ok := pdi.specified[name]; !ok {
			pdi.h.PipelineConfig().RemoveInputRunner(entry.ir)
			delete(pdi.inputs, name)
			pdi.ir.LogMessage(fmt.Sprintf("Removed: %s", name))
		}
	}

	// Iterate through the specified inputs and activate any that are new or
	// have been modified.
	for name, newEntry := range pdi.specified {
		if runningEntry, ok := pdi.inputs[name]; ok {
			if runningEntry.config.Equals(newEntry.config) {
				// Nothing has changed, let this one keep running.
				continue
			}
			// It has changed, stop the old one.
			pdi.h.PipelineConfig().RemoveInputRunner(runningEntry.ir)
			pdi.ir.LogMessage(fmt.Sprintf("Removed: %s", name))
			delete(pdi.inputs, name)
		}

		// Start up a new input.
		if newEntry.ir, err = pdi.startInput(name, newEntry.config); err != nil {
			pdi.ir.LogError(fmt.Errorf("Error creating input '%s': %s", name, err))
			continue
		}
		pdi.inputs[name] = newEntry
		pdi.ir.LogMessage(fmt.Sprintf("Added: %s", name))
	}
	return
}

// Function of type filepath.WalkFunc, called repeatedly when we walk a
// directory tree using filepath.Walk. This function is not reentrant, it
// should only ever be triggered from the similarly not reentrant loadInputs
// method.
func (pdi *ProcessDirectoryInput) procDirWalkFunc(path string, info os.FileInfo,
	err error) error {

	if err != nil {
		pdi.ir.LogError(fmt.Errorf("Error walking '%s': %s", path, err))
		return nil
	}
	// info == nil => filepath doesn't actually exist.
	if info == nil {
		return nil
	}
	// Skip directories or anything that doesn't end in `.toml`.
	if info.IsDir() || filepath.Ext(path) != ".toml" {
		return nil
	}

	// Make sure that the path doesn't get deeper than one level past
	// procDir. It should match `/procDir/<time_interval>/`.
	dir := filepath.Dir(path)
	parentDir, timeInterval := filepath.Split(dir)
	parentDir = strings.TrimSuffix(parentDir, string(os.PathSeparator))
	if parentDir != pdi.procDir {
		pdi.ir.LogError(fmt.Errorf("Invalid ProcessInput path: %s.", path))
		return nil
	}

	// Extract and validate ticker interval from file path.
	var tickInterval int
	if tickInterval, err = strconv.Atoi(timeInterval); err != nil {
		pdi.ir.LogError(fmt.Errorf("Ticker interval could not be parsed for '%s'.",
			path))
		return nil
	}
	if tickInterval < 0 {
		pdi.ir.LogError(fmt.Errorf("A negative ticker interval was parsed for '%s'.",
			path))
		return nil
	}

	// Things look good so far. Try to load the data into a config struct.
	entry := new(ProcessEntry)
	if entry.config, err = pdi.loadProcessFile(path); err != nil {
		pdi.ir.LogError(fmt.Errorf("Error loading process file '%s': %s", path, err))
		return nil
	}
	// Override the ticker interval and store the entry.
	entry.config.TickerInterval = uint(tickInterval)
	pdi.specified[path] = entry
	return nil
}

func (pdi *ProcessDirectoryInput) loadProcessFile(path string) (config *ProcessInputConfig,
	err error) {

	unparsedConfig := make(map[string]toml.Primitive)
	if _, err = toml.DecodeFile(path, &unparsedConfig); err != nil {
		return
	}
	section, ok := unparsedConfig["ProcessInput"]
	if !ok {
		err = errors.New("No `ProcessInput` section.")
		return
	}
	config = &ProcessInputConfig{
		ParserType:  "token",
		ParseStdout: true,
		Trim:        true,
	}
	if err = toml.PrimitiveDecodeStrict(section, config, nil); err != nil {
		return nil, err
	}
	return
}

func (pdi *ProcessDirectoryInput) startInput(name string, config *ProcessInputConfig) (
	ir InputRunner, err error) {

	input := new(ProcessInput)
	input.SetName(name)
	if err = input.Init(config); err != nil {
		return
	}

	var pluginGlobals PluginGlobals
	pluginGlobals.Retries = RetryOptions{
		MaxDelay:   "30s",
		Delay:      "250ms",
		MaxRetries: -1,
	}
	pluginGlobals.Ticker = uint(config.TickerInterval)
	tickLength := time.Duration(pluginGlobals.Ticker) * time.Second
	ir = NewInputRunner(name, input, &pluginGlobals, true)
	ir.SetTickLength(tickLength)
	if err = pdi.h.PipelineConfig().AddInputRunner(ir, nil); err != nil {
		return nil, err
	}
	return
}

func init() {
	RegisterPlugin("ProcessDirectoryInput", func() interface{} {
		return new(ProcessDirectoryInput)
	})
}
