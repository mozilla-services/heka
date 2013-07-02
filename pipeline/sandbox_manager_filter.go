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
#   Mike Trinkala (trink@mozilla.com)
#
# ***** END LICENSE BLOCK *****/

package pipeline

import (
	"fmt"
	"github.com/bbangert/toml"
	"github.com/mozilla-services/heka/message"
	"github.com/mozilla-services/heka/sandbox"
	"io/ioutil"
	"math"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sync/atomic"
	"time"
)

// Heka Filter plugin that listens for (signed) control messages and
// dynamically creates, manages, and destroys sandboxed filter scripts as
// instructed.
type SandboxManagerFilter struct {
	maxFilters          int
	currentFilters      int
	workingDirectory    string
	processMessageCount int64
}

// Config struct for `SandboxManagerFilter`.
type SandboxManagerFilterConfig struct {
	// Maximum number of sandboxed filters this instance will be allowed to
	// manage.
	MaxFilters int `toml:"max_filters"`
	// Path to file system directory the sandbox manager can use for storing
	// dynamic filter scripts and data.
	WorkingDirectory string `toml:"working_directory"`
}

func (this *SandboxManagerFilter) ConfigStruct() interface{} {
	return new(SandboxManagerFilterConfig)
}

// Creates the working directory to store the submitted scripts,
// configurations, and data preservation files.
func (this *SandboxManagerFilter) Init(config interface{}) (err error) {
	conf := config.(*SandboxManagerFilterConfig)
	this.maxFilters = conf.MaxFilters
	this.workingDirectory, _ = filepath.Abs(conf.WorkingDirectory)
	if err = os.MkdirAll(this.workingDirectory, 0700); err != nil {
		return err
	}
	return nil
}

// Adds running filters count to the report output.
func (this *SandboxManagerFilter) ReportMsg(msg *message.Message) error {
	newIntField(msg, "RunningFilters", this.currentFilters, "count")
	newInt64Field(msg, "ProcessMessageCount", atomic.LoadInt64(&this.processMessageCount), "count")
	return nil
}

// Creates a FilterRunner for the specified sandbox name and configuration
func createRunner(dir, name string, configSection toml.Primitive) (FilterRunner, error) {
	var err error
	var pluginGlobals PluginGlobals

	wrapper := new(PluginWrapper)
	wrapper.name = name

	pluginGlobals.Retries = RetryOptions{
		MaxDelay:   "30s",
		Delay:      "250ms",
		MaxRetries: -1,
	}

	if err = toml.PrimitiveDecode(configSection, &pluginGlobals); err != nil {
		return nil, fmt.Errorf("Unable to decode config for plugin: %s, error: %s",
			wrapper.name, err.Error())
	}
	if pluginGlobals.Typ != "SandboxFilter" {
		return nil, fmt.Errorf("Plugin must be a SandboxFilter, received %s",
			pluginGlobals.Typ)
	}

	// Create plugin, test config object generation.
	wrapper.pluginCreator, _ = AvailablePlugins[pluginGlobals.Typ]
	plugin := wrapper.pluginCreator()
	var config interface{}
	if config, err = LoadConfigStruct(configSection, plugin); err != nil {
		return nil, fmt.Errorf("Can't load config for '%s': %s", wrapper.name, err)
	}
	wrapper.configCreator = func() interface{} { return config }
	conf := config.(*sandbox.SandboxConfig)
	conf.ScriptFilename = path.Join(dir, fmt.Sprintf("%s.%s", wrapper.name, conf.ScriptType))

	// Apply configuration to instantiated plugin.
	configPlugin := func() (err error) {
		defer func() {
			// Slight protection against Init call into plugin code.
			if r := recover(); r != nil {
				err = fmt.Errorf("Init() panicked: %s", r)
			}
		}()
		err = plugin.(Plugin).Init(config)
		return
	}
	if err = configPlugin(); err != nil {
		return nil, fmt.Errorf("Initialization failed for '%s': %s", name, err)
	}

	runner := NewFORunner(wrapper.name, plugin.(Plugin), &pluginGlobals)
	runner.name = wrapper.name

	if pluginGlobals.Ticker != 0 {
		runner.tickLength = time.Duration(pluginGlobals.Ticker) * time.Second
	}

	var matcher *MatchRunner
	if pluginGlobals.Matcher != "" {
		if matcher, err = NewMatchRunner(pluginGlobals.Matcher,
			pluginGlobals.Signer); err != nil {
			return nil, fmt.Errorf("Can't create message matcher for '%s': %s",
				wrapper.name, err)
		}
		runner.matcher = matcher
	}

	return runner, nil
}

// Replaces all non word characters with an underscore and returns the
// normalized string
func getNormalizedName(name string) (normalized string) {
	re, _ := regexp.Compile("\\W")
	normalized = re.ReplaceAllString(name, "_")
	return
}

// Combines the sandbox manager and filter name to create a unique namespace
// for each manager. i.e., Multiple managers can run a filter named 'Counter'
// even when sharing the same working directory.
func getSandboxName(managerName, sandboxName string) (name string) {
	name = fmt.Sprintf("%s-%s", getNormalizedName(managerName),
		getNormalizedName(sandboxName))
	return
}

// Cleans up the script and configuration files on unload or load failure.
func removeAll(dir, glob string) {
	if matches, err := filepath.Glob(path.Join(dir, glob)); err == nil {
		for _, fn := range matches {
			os.Remove(fn)
		}
	}
}

// Parses a Heka message and extracts the information necessary to start a new
// SandboxFilter
func (this *SandboxManagerFilter) loadSandbox(fr FilterRunner,
	h PluginHelper, dir string, msg *message.Message) (err error) {
	fv, _ := msg.GetFieldValue("config")
	if config, ok := fv.(string); ok {
		var configFile ConfigFile
		if _, err = toml.Decode(config, &configFile); err != nil {
			return fmt.Errorf("loadSandbox failed: %s\n", err)
		} else {
			for name, conf := range configFile {
				name = getSandboxName(fr.Name(), name)
				if _, ok := h.Filter(name); ok {
					// todo support reload
					return fmt.Errorf("loadSandbox failed: %s is already running", name)
				}
				fr.LogMessage(fmt.Sprintf("Loading: %s", name))
				confFile := path.Join(dir, fmt.Sprintf("%s.toml", name))
				err = ioutil.WriteFile(confFile, []byte(config), 0600)
				if err != nil {
					return
				}
				var sbc sandbox.SandboxConfig
				if err = toml.PrimitiveDecode(conf, &sbc); err != nil {
					return fmt.Errorf("loadSandbox failed: %s\n", err)
				}
				scriptFile := path.Join(dir, fmt.Sprintf("%s.%s", name, sbc.ScriptType))
				err = ioutil.WriteFile(scriptFile, []byte(msg.GetPayload()), 0600)
				if err != nil {
					removeAll(dir, fmt.Sprintf("%s.*", name))
					return
				}
				var runner FilterRunner
				runner, err = createRunner(dir, name, conf)
				if err != nil {
					removeAll(dir, fmt.Sprintf("%s.*", name))
					return
				}
				err = h.PipelineConfig().AddFilterRunner(runner)
				if err == nil {
					this.currentFilters++
				}
				break // only interested in the first item
			}
		}
	}
	return
}

// On Heka restarts this function reloads all previously running SandboxFilters
// using the script, configuration, and preservation files in the working
// directory.
func (this *SandboxManagerFilter) restoreSandboxes(fr FilterRunner, h PluginHelper, dir string) {
	glob := fmt.Sprintf("%s-*.toml", getNormalizedName(fr.Name()))
	if matches, err := filepath.Glob(path.Join(dir, glob)); err == nil {
		for _, fn := range matches {
			var configFile ConfigFile
			if _, err = toml.DecodeFile(fn, &configFile); err != nil {
				fr.LogError(fmt.Errorf("restoreSandboxes failed: %s\n", err))
				continue
			} else {
				for _, conf := range configFile {
					var runner FilterRunner
					name := path.Base(fn[:len(fn)-5])
					fr.LogMessage(fmt.Sprintf("Loading: %s", name))
					runner, err = createRunner(dir, name, conf)
					if err != nil {
						fr.LogError(fmt.Errorf("createRunner failed: %s\n", err.Error()))
						removeAll(dir, fmt.Sprintf("%s.*", name))
						break
					}
					err = h.PipelineConfig().AddFilterRunner(runner)
					if err != nil {
						fr.LogError(err)
					} else {
						this.currentFilters++
					}
					break // only interested in the first item
				}
			}
		}
	}
}

func (this *SandboxManagerFilter) Run(fr FilterRunner, h PluginHelper) (err error) {
	inChan := fr.InChan()

	var ok = true
	var pack *PipelinePack
	var delta int64
	this.restoreSandboxes(fr, h, this.workingDirectory)
	for ok {
		select {
		case pack, ok = <-inChan:
			if !ok {
				break
			}
			atomic.AddInt64(&this.processMessageCount, 1)
			delta = time.Now().UnixNano() - pack.Message.GetTimestamp()
			if math.Abs(float64(delta)) >= 5e9 {
				fr.LogError(fmt.Errorf("Discarded control message: %d seconds skew", delta/1e9))
				pack.Recycle()
				break
			}
			action, _ := pack.Message.GetFieldValue("action")
			switch action {
			case "load":
				if this.currentFilters < this.maxFilters {
					err := this.loadSandbox(fr, h, this.workingDirectory, pack.Message)
					if err != nil {
						fr.LogError(err)
					}
				} else {
					fr.LogError(fmt.Errorf("%s attempted to load more than %d filters", fr.Name(), this.maxFilters))
				}
			case "unload":
				fv, _ := pack.Message.GetFieldValue("name")
				if name, ok := fv.(string); ok {
					name = getSandboxName(fr.Name(), name)
					if h.PipelineConfig().RemoveFilterRunner(name) {
						this.currentFilters--
						removeAll(this.workingDirectory, fmt.Sprintf("%s.*", name))
					}
				}
			}
			pack.Recycle()
		}
	}
	return
}
