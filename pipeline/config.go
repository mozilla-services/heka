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
	. "heka/message"
	"log"
	"os"
)

var AvailablePlugins = map[string]func() Plugin{
	"MessageGeneratorInput": func() Plugin { return new(MessageGeneratorInput) },
	"UdpInput":              func() Plugin { return new(UdpInput) },
	"JsonDecoder":           func() Plugin { return new(JsonDecoder) },
	"MsgPackDecoder":        func() Plugin { return new(MsgPackDecoder) },
	"StatRollupFilter":      func() Plugin { return new(StatRollupFilter) },
	"NamedOutputFilter":     func() Plugin { return new(NamedOutputFilter) },
	"LogFilter":             func() Plugin { return new(LogFilter) },
	"LogOutput":             func() Plugin { return new(LogOutput) },
	"CounterOutput":         func() Plugin { return new(CounterOutput) },
	"FileOutput":            func() Plugin { return new(FileOutput) },
}

type PluginConfig map[string]interface{}

type HasConfigStruct interface {
	ConfigStruct() interface{}
}

type PipelineConfig struct {
	Inputs             map[string]Input
	DefaultDecoder     string
	DecoderCreator     func() map[string]Decoder
	FilterChains       map[string]FilterChain
	FilterCreator      func() map[string]Filter
	OutputCreator      func() map[string]Output
	DefaultFilterChain string
	PoolSize           int
	Lookup             *MessageLookup
}

// The JSON config file spec
type ConfigFile struct {
	Inputs   []PluginConfig
	Decoders []PluginConfig
	Filters  []PluginConfig
	Outputs  []PluginConfig
	Chains   map[string]PluginConfig
}

type FilterChain struct {
	Outputs []string
	Filters []string
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

// InitPlugin initializes a plugin with its section while also
// attempting to use a config struct if available
func initPlugin(plugin Plugin, section *PluginConfig) {
	pluginName := (*section)["type"].(string)
	// Determine if we should re-marshal
	if hasConfigStruct, ok := plugin.(HasConfigStruct); ok {
		data, err := json.Marshal(section)
		if err != nil {
			log.Fatal("Error: Can't marshal section.")
		}
		configStruct := hasConfigStruct.ConfigStruct()
		err = json.Unmarshal(data, configStruct)
		if err != nil {
			log.Fatalln("Error: Can't unmarshal section again.")
		}
		if err := plugin.Init(configStruct); err != nil {
			log.Fatalf("Unable to load config section: %s. Error: %s",
				pluginName, err)
		}
	} else {
		if err := plugin.Init(section); err != nil {
			log.Fatalf("Unable to load config section: %s. Error: %s",
				pluginName, err)
		}
	}
	return
}

// loadSection can be passed a configSection, the appropriate mapping
// of available plugins for that section, and will then load and
// instantiate the plugins into the returned config value
func loadSection(configSection []PluginConfig) (config map[string]Plugin) {
	var pluginName string
	config = make(map[string]Plugin)
	for _, section := range configSection {
		pluginType := section["type"].(string)

		// Use the section naming if applicable, or default to the type
		// name
		if _, ok := section["name"]; ok {
			pluginName = section["name"].(string)
		} else {
			pluginName = pluginType
		}

		pluginFunc, ok := AvailablePlugins[pluginType]
		if !ok {
			log.Fatalln("Error: No such plugin of that name: ", pluginType)
		}

		plugin := pluginFunc()
		initPlugin(plugin, &section)
		config[pluginName] = plugin
	}
	return config
}

// Given a filename string and JSON structure, read the file and
// un-marshal it into the structure
func readJsonFromFile(filename string, configFile *ConfigFile) {
	file, err := os.Open(filename)
	if err != nil {
		log.Fatalf("Error reading file: %s", err)
	}
	defer file.Close()

	jsonBytes := make([]byte, 1e5)
	n, err := file.Read(jsonBytes)
	if err != nil {
		log.Fatalf("Error reading byte in file: %s", err)
	}
	jsonBytes = jsonBytes[:n]

	if err = json.Unmarshal(jsonBytes, configFile); err != nil {
		log.Fatalf("Error with Unmarshal: %s", err)
	}
	return
}

// LoadFromConfigFile loads a JSON configuration file and stores the
// result in the value pointed to by config. The maps in the config
// will be initialized as needed.
//
// The PipelineConfig should be already initialized before passed in via
// its Init function.
func (self *PipelineConfig) LoadFromConfigFile(filename string) error {
	configFile := new(ConfigFile)
	readJsonFromFile(filename, configFile)

	for name, plugin := range loadSection(configFile.Inputs) {
		self.Inputs[name] = plugin.(Input)
	}

	self.DecoderCreator = func() (decoders map[string]Decoder) {
		decoders = make(map[string]Decoder)
		for name, plugin := range loadSection(configFile.Decoders) {
			decoders[name] = plugin.(Decoder)
		}
		return decoders
	}

	self.FilterCreator = func() (filters map[string]Filter) {
		filters = make(map[string]Filter)
		for name, plugin := range loadSection(configFile.Filters) {
			filters[name] = plugin.(Filter)
		}
		return filters
	}

	self.OutputCreator = func() (outputs map[string]Output) {
		outputs = make(map[string]Output)
		for name, plugin := range loadSection(configFile.Outputs) {
			outputs[name] = plugin.(Output)
		}
		return outputs
	}

	availOutputs := self.OutputCreator()
	availFilters := self.FilterCreator()

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
				if _, ok := availOutputs[strOutput]; !ok {
					log.Fatalln("Error during chain loading. Output by name ",
						output, " was not defined.")
				}
				chain.Outputs[i] = strOutput
			}
		}
		if filters, ok := section["filters"]; ok {
			filterList := filters.([]interface{})
			chain.Filters = make([]string, len(filterList))
			for i, filter := range filterList {
				strFilter := filter.(string)
				if _, ok := availFilters[strFilter]; !ok {
					log.Fatalln("Error during chain loading. Filter by name ",
						filter, " was not defined.")
				}
				chain.Filters[i] = strFilter
			}
		}

		// Add the message type to the lookup table if present
		if _, ok := section["type"]; ok {
			msgType := section["type"].(string)
			msgLookup := self.Lookup
			if _, ok := msgLookup.MessageType[msgType]; !ok {
				msgLookup.MessageType[msgType] = make([]string, 0, 10)
			}
			msgLookup.MessageType[msgType] = append(msgLookup.MessageType[msgType], name)
		}
		self.FilterChains[name] = chain
	}
	return nil
}
