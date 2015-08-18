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
#   Mike Trinkala (trink@mozilla.com)
#   Rob Miller (rmiller@mozilla.com)
#
# ***** END LICENSE BLOCK *****/

package plugins

import (
	"fmt"
	"io/ioutil"
	"math"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sync/atomic"
	"time"

	"github.com/bbangert/toml"
	"github.com/mozilla-services/heka/message"
	"github.com/mozilla-services/heka/pipeline"
	. "github.com/mozilla-services/heka/sandbox"
)

// Heka Filter plugin that listens for (signed) control messages and
// dynamically creates, manages, and destroys sandboxed filter scripts as
// instructed.
type SandboxManagerFilter struct {
	processMessageCount int64
	currentFilters      int32
	maxFilters          int
	workingDirectory    string
	moduleDirectory     string
	memoryLimit         uint
	instructionLimit    uint
	outputLimit         uint
	pConfig             *pipeline.PipelineConfig
}

// Config struct for `SandboxManagerFilter`.
type SandboxManagerFilterConfig struct {
	// Maximum number of sandboxed filters this instance will be allowed to
	// manage.
	MaxFilters int `toml:"max_filters"`
	// Path to file system directory the sandbox manager can use for storing
	// dynamic filter scripts and data. Relative paths will be relative to the
	// Heka base_dir. Defaults to a directory in ${BASE_DIR}/sbxmgrs that is
	// auto-generated based on the plugin name.
	WorkingDirectory string `toml:"working_directory"`
	// Path to the file system directory where the sandbox manager will direct
	// all SandboxFilter 'require' requests. Defaults to
	// ${SHARE_DIR}/lua_modules.
	ModuleDirectory string `toml:"module_directory"`
	// Memory limit applied to all managed sandboxes.
	MemoryLimit uint `toml:"memory_limit"`
	// Instruction limit applied to all managed sandboxes.
	InstructionLimit uint `toml:"instruction_limit"`
	// Output limit applied to all managed sandboxes.
	OutputLimit uint `toml:"output_limit"`
	// Default message matcher.
	MessageMatcher string `toml:"message_matcher"`
}

func (this *SandboxManagerFilter) ConfigStruct() interface{} {
	sbDefaults := NewSandboxConfig(this.pConfig.Globals).(*SandboxConfig)
	return &SandboxManagerFilterConfig{
		WorkingDirectory: "sbxmgrs",
		ModuleDirectory:  sbDefaults.ModuleDirectory,
		MemoryLimit:      sbDefaults.MemoryLimit,
		InstructionLimit: sbDefaults.InstructionLimit,
		OutputLimit:      sbDefaults.OutputLimit,
		MessageMatcher:   "Type == 'heka.control.sandbox'",
	}
}

func (s *SandboxManagerFilter) PluginExited() {
	atomic.AddInt32(&s.currentFilters, -1)
}

// Heka will call this before calling any other methods to give us access to
// the pipeline configuration.
func (s *SandboxManagerFilter) SetPipelineConfig(pConfig *pipeline.PipelineConfig) {
	s.pConfig = pConfig
}

// Creates the working directory to store the submitted scripts,
// configurations, and data preservation files.
func (this *SandboxManagerFilter) Init(config interface{}) (err error) {
	conf := config.(*SandboxManagerFilterConfig)
	globals := this.pConfig.Globals
	this.maxFilters = conf.MaxFilters
	this.workingDirectory = globals.PrependBaseDir(conf.WorkingDirectory)
	this.moduleDirectory = conf.ModuleDirectory
	this.memoryLimit = conf.MemoryLimit
	this.instructionLimit = conf.InstructionLimit
	this.outputLimit = conf.OutputLimit
	err = os.MkdirAll(this.workingDirectory, 0700)
	return
}

// Adds running filters count to the report output.
func (this *SandboxManagerFilter) ReportMsg(msg *message.Message) error {
	message.NewIntField(msg, "RunningFilters", int(atomic.LoadInt32(&this.currentFilters)),
		"count")
	message.NewInt64Field(msg, "ProcessMessageCount",
		atomic.LoadInt64(&this.processMessageCount), "count")
	return nil
}

// Creates a FilterRunner for the specified sandbox name and configuration.
func (this *SandboxManagerFilter) createRunner(dir, name string, configSection toml.Primitive) (
	pipeline.FilterRunner, error) {

	maker, err := pipeline.NewPluginMaker(name, this.pConfig, configSection)
	if err != nil {
		return nil, err
	}
	if maker.Type() != "SandboxFilter" {
		return nil, fmt.Errorf("Plugin must be a SandboxFilter, received %s",
			maker.Type())
	}

	// Customize the PrepConfig method so we can override any specified
	// settings with the manager's settings.
	mutMaker := maker.(pipeline.MutableMaker)
	prepConfig := func() (interface{}, error) {
		config, err := mutMaker.OrigPrepConfig()
		if err != nil {
			return nil, err
		}
		conf := config.(*SandboxConfig)
		conf.ScriptFilename = filepath.Join(dir, fmt.Sprintf("%s.%s", name, conf.ScriptType))
		conf.ModuleDirectory = this.moduleDirectory
		conf.MemoryLimit = this.memoryLimit
		conf.InstructionLimit = this.instructionLimit
		conf.OutputLimit = this.outputLimit
		conf.PluginType = "filter"
		return conf, nil
	}
	mutMaker.SetPrepConfig(prepConfig)

	// Finally call MakeRunner() to initialize the plugin and create the
	// runner.
	runner, err := mutMaker.MakeRunner(name)
	if err != nil {
		return nil, err
	}
	sbxFilter := runner.Plugin().(*SandboxFilter)
	sbxFilter.manager = this
	return runner.(pipeline.FilterRunner), nil
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
	if matches, err := filepath.Glob(filepath.Join(dir, glob)); err == nil {
		for _, fn := range matches {
			os.Remove(fn)
		}
	}
}

// Parses a Heka message and extracts the information necessary to start a new
// SandboxFilter
func (this *SandboxManagerFilter) loadSandbox(fr pipeline.FilterRunner,
	h pipeline.PluginHelper, dir string, msg *message.Message) (err error) {

	fv, _ := msg.GetFieldValue("config")
	if config, ok := fv.(string); ok {
		var configFile pipeline.ConfigFile
		if _, err = toml.Decode(config, &configFile); err != nil {
			return fmt.Errorf("loadSandbox failed: %s\n", err)
		}

		for name, conf := range configFile {
			name = getSandboxName(fr.Name(), name)
			if _, ok := h.Filter(name); ok {
				// todo support reload
				return fmt.Errorf("loadSandbox failed: %s is already running", name)
			}
			fr.LogMessage(fmt.Sprintf("Loading: %s", name))
			confFile := filepath.Join(dir, fmt.Sprintf("%s.toml", name))
			err = ioutil.WriteFile(confFile, []byte(config), 0600)
			if err != nil {
				return
			}
			var sbc SandboxConfig
			// Default, will get overwritten if necessary
			sbc.ScriptType = "lua"
			if err = toml.PrimitiveDecode(conf, &sbc); err != nil {
				return fmt.Errorf("loadSandbox failed: %s\n", err)
			}
			scriptFile := filepath.Join(dir, fmt.Sprintf("%s.%s", name, sbc.ScriptType))
			err = ioutil.WriteFile(scriptFile, []byte(msg.GetPayload()), 0600)
			if err != nil {
				removeAll(dir, fmt.Sprintf("%s.*", name))
				return
			}
			var runner pipeline.FilterRunner
			runner, err = this.createRunner(dir, name, conf)
			if err != nil {
				removeAll(dir, fmt.Sprintf("%s.*", name))
				return
			}
			err = this.pConfig.AddFilterRunner(runner)
			if err == nil {
				atomic.AddInt32(&this.currentFilters, 1)
			}
			break // only interested in the first item
		}
	}
	return
}

// On Heka restarts this function reloads all previously running SandboxFilters
// using the script, configuration, and preservation files in the working
// directory.
func (this *SandboxManagerFilter) restoreSandboxes(fr pipeline.FilterRunner,
	h pipeline.PluginHelper, dir string) {

	glob := fmt.Sprintf("%s-*.toml", getNormalizedName(fr.Name()))
	if matches, err := filepath.Glob(filepath.Join(dir, glob)); err == nil {
		for _, fn := range matches {
			var configFile pipeline.ConfigFile
			if _, err = toml.DecodeFile(fn, &configFile); err != nil {
				fr.LogError(fmt.Errorf("restoreSandboxes failed: %s\n", err))
				continue
			}
			for _, conf := range configFile {
				var runner pipeline.FilterRunner
				name := path.Base(fn[:len(fn)-5])
				fr.LogMessage(fmt.Sprintf("Loading: %s", name))
				runner, err = this.createRunner(dir, name, conf)
				if err != nil {
					fr.LogError(fmt.Errorf("createRunner failed: %s\n", err.Error()))
					removeAll(dir, fmt.Sprintf("%s.*", name))
					break
				}
				err = this.pConfig.AddFilterRunner(runner)
				if err != nil {
					fr.LogError(err)
				} else {
					atomic.AddInt32(&this.currentFilters, 1)
				}
				break // only interested in the first item
			}
		}
	}
}

func (this *SandboxManagerFilter) Run(fr pipeline.FilterRunner,
	h pipeline.PluginHelper) (err error) {

	inChan := fr.InChan()

	var ok = true
	var pack *pipeline.PipelinePack
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
				fr.UpdateCursor(pack.QueueCursor)
				pack.Recycle(fmt.Errorf("Discarded control message: %d seconds skew",
					delta/1e9))
				break
			}
			action, _ := pack.Message.GetFieldValue("action")
			switch action {
			case "load":
				current := int(atomic.LoadInt32(&this.currentFilters))
				if current < this.maxFilters {
					err := this.loadSandbox(fr, h, this.workingDirectory, pack.Message)
					if err != nil {
						p, e := h.PipelinePack(0)
						if e != nil {
							fr.LogError(err)
							fr.LogError(fmt.Errorf("can't send termination message: %s", e.Error()))
							break
						}
						p.Message.SetType("heka.sandbox-terminated")
						p.Message.SetLogger(pipeline.HEKA_DAEMON)
						message.NewStringField(p.Message, "plugin", fr.Name())
						p.Message.SetPayload(err.Error())
						fr.Inject(p)
						fr.LogError(err)
					}
				} else {
					fr.LogError(fmt.Errorf("%s attempted to load more than %d filters",
						fr.Name(), this.maxFilters))
				}
			case "unload":
				fv, _ := pack.Message.GetFieldValue("name")
				if name, ok := fv.(string); ok {
					name = getSandboxName(fr.Name(), name)
					if this.pConfig.RemoveFilterRunner(name) {
						removeAll(this.workingDirectory, fmt.Sprintf("%s.*", name))
					}
				}
			}
			pack.Recycle(nil)
		}
	}
	return
}
