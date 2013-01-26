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
#   Rob Miller (rmiller@mozilla.com)
#
# ***** END LICENSE BLOCK *****/
package pipeline

import (
	"encoding/json"
	"errors"
	. "github.com/mozilla-services/heka/message"
	"os"
)

var AvailablePlugins = make(map[string]func() interface{})

func RegisterPlugin(name string, factory func() interface{}) {
	AvailablePlugins[name] = factory
}

type PluginConfig map[string]interface{}

// Indicates a plug-in has a specific-to-itself config struct that should be
// passed in to its Init method.
type HasConfigStruct interface {
	ConfigStruct() interface{}
}

type FilterChain struct {
	Outputs []string
	Filters []string
}

// Master config object encapsulating the entire heka/pipeline configuration.
type PipelineConfig struct {
	Inputs             map[string]*PluginWrapper
	Decoders           map[string]*PluginWrapper
	Filters            map[string]*PluginWrapper
	Outputs            map[string]*PluginWrapper
	FilterChains       map[string]FilterChain
	DefaultDecoder     string
	DefaultFilterChain string
	PoolSize           int
	Lookup             *MessageLookup
}

// Creates and initializes a PipelineConfig object.
func NewPipelineConfig(poolSize int) (config *PipelineConfig) {
	PoolSize = poolSize
	config = new(PipelineConfig)
	config.PoolSize = poolSize
	config.FilterChains = make(map[string]FilterChain)
	config.DefaultFilterChain = "default"
	config.Lookup = new(MessageLookup)
	config.Lookup.MessageType = make(map[string][]string)
	return config
}

// The JSON config file spec
type ConfigFile struct {
	Inputs   []PluginConfig
	Decoders []PluginConfig
	Filters  []PluginConfig
	Outputs  []PluginConfig
	Chains   map[string]PluginConfig
}

// Represents message lookup hashes
//
// MessageType is populated such that a message type should exactly
// match and return a list representing keys in FilterChains. At the
// moment the list will always be a single element, but in the future
// with more ways to restrict the filter chain to other components of
// the message narrowing down the set for several will be needed.
type MessageLookup struct {
	MessageType map[string][]string
}

func (self *MessageLookup) LocateChain(message *Message) (string, bool) {
	if chains, ok := self.MessageType[message.Type]; ok {
		return chains[0], true
	}
	return "", false
}

// A plugin wrapper wraps a plugin so that more instances of it can be
// easily created and a reference can be held to the plugins global if
// it needs one
type PluginWrapper struct {
	name          string
	configCreator func() interface{}
	pluginCreator func() interface{}
	global        PluginGlobal
}

// Create a new instance of the plugin and return it
//
// Errors are ignored. Call with CreateWithError if an error is needed
func (self *PluginWrapper) Create() (plugin interface{}) {
	plugin, _ = self.CreateWithError()
	return
}

// Creates a new instance
func (self *PluginWrapper) CreateWithError() (plugin interface{}, err error) {
	plugin = self.pluginCreator()
	if self.global == nil {
		err = plugin.(Plugin).Init(self.configCreator())
	} else {
		// pluginCreator only returns a Plugin int, so we type assert
		err = plugin.(PluginWithGlobal).Init(self.global, self.configCreator())
	}
	return
}

// If `configable` supports the `HasConfigStruct` interface this will use said
// interface to fetch a config struct object and populate it w/ the values in
// provided `config`. If not, simply returns `config` unchanged.
func LoadConfigStruct(config *PluginConfig, configable interface{}) (interface{}, error) {
	hasConfigStruct, ok := configable.(HasConfigStruct)
	if !ok {
		return config, nil
	}

	data, err := json.Marshal(config)
	if err != nil {
		return nil, errors.New("Unable to marshal: " + err.Error())
	}
	configStruct := hasConfigStruct.ConfigStruct()
	err = json.Unmarshal(data, configStruct)
	if err != nil {
		return nil, errors.New("Unable to unmarshal: " + err.Error())
	}
	return configStruct, nil
}

// loadSection can be passed a configSection, the appropriate mapping
// of available plug-ins for that section, and will then create
// PluginWrappers for each plug-in instance defined
func loadSection(configSection []PluginConfig) (config map[string]*PluginWrapper, err error) {
	var ok bool
	config = make(map[string]*PluginWrapper)
	for _, section := range configSection {
		wrapper := new(PluginWrapper)
		pluginType := section["type"].(string)

		// Use the section naming if applicable, or default to the type
		// name
		if _, ok := section["name"]; ok {
			wrapper.name = section["name"].(string)
		} else {
			wrapper.name = pluginType
		}

		if wrapper.pluginCreator, ok = AvailablePlugins[pluginType]; !ok {
			err := errors.New("No such plugin of that name: " + pluginType)
			return nil, err
		}

		// Create an instance so we can see if we need to marshal the JSON
		plugin := wrapper.pluginCreator()
		configLoaded, err := LoadConfigStruct(&section, plugin)
		if err != nil {
			return nil, err
		}
		wrapper.configCreator = func() interface{} { return configLoaded }

		// Determine if this plug-in has a global, if it does, make it now
		if hasPluginGlobal, ok := plugin.(PluginWithGlobal); ok {
			wrapper.global, err = hasPluginGlobal.InitOnce(wrapper.configCreator())
			if err != nil {
				return config, errors.New("Unable to InitOnce: " + err.Error())
			}
		}

		config[wrapper.name] = wrapper
	}
	return config, nil
}

// Given a filename string and JSON structure, read the file and
// un-marshal it into the structure
func readJsonFromFile(filename string, configFile *ConfigFile) error {
	file, err := os.Open(filename)
	if err != nil {
		return errors.New("Unable to open file: " + err.Error())
	}
	defer file.Close()

	jsonBytes := make([]byte, 1e5)
	n, err := file.Read(jsonBytes)
	if err != nil {
		return errors.New("Error reading byte in file: " + err.Error())
	}
	jsonBytes = jsonBytes[:n]

	if err = json.Unmarshal(jsonBytes, configFile); err != nil {
		return errors.New("Error with config file unmarshal: " + err.Error())
	}
	return nil
}

// LoadFromConfigFile loads a JSON configuration file and stores the
// result in the value pointed to by config. The maps in the config
// will be initialized as needed.
//
// The PipelineConfig should be already initialized before passed in via
// its Init function.
func (self *PipelineConfig) LoadFromConfigFile(filename string) (err error) {
	configFile := new(ConfigFile)
	if err = readJsonFromFile(filename, configFile); err != nil {
		return err
	}

	if self.Inputs, err = loadSection(configFile.Inputs); err != nil {
		return err
	} else {
		// Setup our message generator input
		MessageGenerator.Init()
		mgiWrapper := new(PluginWrapper)
		mgiWrapper.name = "MessageGeneratorInput"
		mgiWrapper.pluginCreator = func() interface{} { return new(MessageGeneratorInput) }
		mgiWrapper.configCreator = func() interface{} { return new(PluginConfig) }
		self.Inputs["MessageGeneratorInput"] = mgiWrapper
	}

	self.Decoders, err = loadSection(configFile.Decoders)
	if err != nil {
		return errors.New("Error reading decoders: " + err.Error())
	}

	self.Filters, err = loadSection(configFile.Filters)
	if err != nil {
		return errors.New("Error reading filters: " + err.Error())
	}

	self.Outputs, err = loadSection(configFile.Outputs)
	if err != nil {
		return errors.New("Error reading outputs: " + err.Error())
	}

	// Locate and set the default decoder
	for _, section := range configFile.Decoders {
		if _, ok := section["default"]; !ok {
			continue
		}
		// Determine if its keyed by type or name
		if name, ok := section["name"]; ok {
			self.DefaultDecoder = name.(string)
		} else {
			self.DefaultDecoder = section["type"].(string)
		}
	}

	for name, section := range configFile.Chains {
		chain := FilterChain{}
		if outputs, ok := section["outputs"]; ok {
			outputList := outputs.([]interface{})
			chain.Outputs = make([]string, len(outputList))
			for i, output := range outputList {
				strOutput := output.(string)
				if _, ok := self.Outputs[strOutput]; !ok {
					return errors.New("Error during chain loading. Output by name " +
						strOutput + " was not defined.")
				}
				chain.Outputs[i] = strOutput
			}
		}
		if filters, ok := section["filters"]; ok {
			filterList := filters.([]interface{})
			chain.Filters = make([]string, len(filterList))
			for i, filter := range filterList {
				strFilter := filter.(string)
				if _, ok := self.Filters[strFilter]; !ok {
					return errors.New("Error during chain loading. Filter by name " +
						strFilter + " was not defined.")
				}
				chain.Filters[i] = strFilter
			}
		}

		// Add the message type to the lookup table if present
		if _, ok := section["message_type"]; ok {
			messageTypeList := section["message_type"].([]interface{})
			msgLookup := self.Lookup
			var msgType string
			for _, rawType := range messageTypeList {
				// Create the string slice for this msgType if it
				// doesn't exist already.

				// TODO: Later on we'll support additional ways to
				// restrict the chain used based on more than just the
				// message type which is why we store a slice of
				// strings here at the moment
				msgType = rawType.(string)
				if _, ok := msgLookup.MessageType[msgType]; !ok {
					msgLookup.MessageType[msgType] = make([]string, 0, 10)
				}
				msgLookup.MessageType[msgType] = append(msgLookup.MessageType[msgType], name)
			}
		}
		self.FilterChains[name] = chain

	}

	// Ensure there's a default decoder available
	if self.DefaultDecoder == "" {
		return errors.New("No default decoder defined.")
	}

	return nil
}

func init() {
	RegisterPlugin("UdpInput", func() interface{} {
		return new(UdpInput)
	})
	RegisterPlugin("JsonDecoder", func() interface{} {
		return new(JsonDecoder)
	})
	RegisterPlugin("MsgPackDecoder", func() interface{} {
		return new(MsgPackDecoder)
	})
	RegisterPlugin("StatsdUdpInput", func() interface{} {
		return RunnerMaker(new(StatsdInWriter))
	})
	RegisterPlugin("LogOutput", func() interface{} {
		return new(LogOutput)
	})
	RegisterPlugin("CounterOutput", func() interface{} {
		return new(CounterOutput)
	})
	RegisterPlugin("FileOutput", func() interface{} {
		return RunnerMaker(new(FileWriter))
	})
}
