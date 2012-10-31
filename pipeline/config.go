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
	"log"
	"os"
)

var AvailablePlugins = map[string]func() Plugin{
	"MessageGeneratorInput": func() Plugin { return new(MessageGeneratorInput) },
	"UdpInput":              func() Plugin { return new(UdpInput) },
	"UdpGobInput":           func() Plugin { return new(UdpGobInput) },
	"JsonDecoder":           func() Plugin { return new(JsonDecoder) },
	"GobDecoder":            func() Plugin { return new(GobDecoder) },
	"StatRollupFilter":      func() Plugin { return new(StatRollupFilter) },
	"NamedOutputFilter":     func() Plugin { return new(NamedOutputFilter) },
	"LogFilter":             func() Plugin { return new(LogFilter) },
	"LogOutput":             func() Plugin { return new(LogOutput) },
	"CounterOutput":         func() Plugin { return new(CounterOutput) },
}

type PluginConfig map[string]interface{}

type GraterConfig struct {
	Inputs             map[string]Input
	Decoders           map[string]Decoder
	DefaultDecoder     string
	FilterChains       map[string]FilterChain
	Filters            map[string]Filter
	DefaultFilterChain string
	Outputs            map[string]Output
	DefaultOutputs     []string
	PoolSize           int
	Lookup             MessageLookup
}

// Initialize a GraterConfig for use
func (this *GraterConfig) Init() {
	this.Inputs = make(map[string]Input)
	this.Decoders = make(map[string]Decoder)
	this.FilterChains = make(map[string]FilterChain)
	this.Outputs = make(map[string]Output)
	this.Filters = make(map[string]Filter)
	this.Lookup = MessageLookup{}
	this.Lookup.MessageType = make(map[string][]string)
	return
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

// The JSON config file spec
type ConfigFile struct {
	Inputs   []PluginConfig
	Decoders []PluginConfig
	Filters  []PluginConfig
	Outputs  []PluginConfig
	Chains   map[string]PluginConfig
}

// LoadSection can be passed a configSection, the appropriate mapping
// of available plugins for that section, and will then load and
// instantiate the plugins into the returned config value
func LoadSection(configSection []PluginConfig) (config map[string]Plugin) {
	var (
		plugin                 Plugin
		pluginType, pluginName string
		ok                     bool
	)
	config = make(map[string]Plugin)
	for _, section := range configSection {
		pluginType = section["type"].(string)

		// Use the section naming if applicable, or default to the type
		// name
		if _, ok = section["name"]; ok {
			pluginName = section["name"].(string)
		} else {
			pluginName = pluginType
		}

		if pluginFunc, ok := AvailablePlugins[pluginType]; !ok {
			log.Fatalln("Error: No such plugin of that name: ", pluginType)
		} else {
			plugin = pluginFunc()

			// Determine if we should re-marshal
			if jsonPlugin, ok := plugin.(PluginJsonConfig); ok {
				data, err := json.Marshal(section)
				if err != nil {
					log.Fatal("Error: Can't marshal section.")
				}
				newsection := jsonPlugin.JsonConfig()
				err = json.Unmarshal(data, newsection)
				if err != nil {
					log.Fatalln("Error: Can't unmarshal section again.")
				}
				if err := jsonPlugin.Init(newsection); err != nil {
					log.Fatalf("Unable to load config section: %s. Error: %s",
						pluginName, err)
				}
			} else {
				if err := plugin.Init(section); err != nil {
					log.Fatalf("Unable to load config section: %s. Error: %s",
						pluginName, err)
				}
			}
			config[pluginName] = plugin
		}
	}
	return config
}

// LoadFromConfigFile loads a JSON configuration file and stores the
// result in the value pointed to by config. The maps in the config
// will be initialized as needed.
//
// The GraterConfig should be already initialized before passed in via
// its Init function.
func LoadFromConfigFile(filename string, config *GraterConfig) error {
	file, err := os.Open(filename)
	if err != nil {
		return err
	}
	defer file.Close()

	jsonBytes := make([]byte, 1e5)
	n, err := file.Read(jsonBytes)
	if err != nil {
		return err
	}
	jsonBytes = jsonBytes[:n]

	configFile := new(ConfigFile)
	err = json.Unmarshal(jsonBytes, configFile)
	if err != nil {
		return err
	}

	for name, plugin := range LoadSection(configFile.Inputs) {
		config.Inputs[name] = plugin.(Input)
	}
	for name, plugin := range LoadSection(configFile.Decoders) {
		config.Decoders[name] = plugin.(Decoder)
	}
	for name, plugin := range LoadSection(configFile.Filters) {
		config.Filters[name] = plugin.(Filter)
	}
	for name, plugin := range LoadSection(configFile.Outputs) {
		config.Outputs[name] = plugin.(Output)
	}

	// Locate and set the default decoder
	for _, section := range configFile.Decoders {
		if _, ok := section["default"]; !ok {
			continue
		}
		// Determine if its keyed by type or name
		if name, ok := section["name"]; ok {
			config.DefaultDecoder = name.(string)
		} else {
			config.DefaultDecoder = section["type"].(string)
		}
	}

	for name, section := range configFile.Chains {
		chain := FilterChain{}
		if outputs, ok := section["outputs"]; ok {
			outputList := outputs.([]interface{})
			chain.Outputs = make([]string, len(outputList))
			for i, output := range outputList {
				strOutput := output.(string)
				if _, ok := config.Outputs[strOutput]; !ok {
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
				if _, ok := config.Filters[strFilter]; !ok {
					log.Fatalln("Error during chain loading. Filter by name ",
						filter, " was not defined.")
				}
				chain.Filters[i] = strFilter
			}
		}

		// Add the message type to the lookup table if present
		if _, ok := section["type"]; ok {
			msgType := section["type"].(string)
			msgLookup := config.Lookup
			if _, ok := msgLookup.MessageType[msgType]; !ok {
				msgLookup.MessageType[msgType] = make([]string, 0, 10)
			}
			msgLookup.MessageType[msgType] = append(msgLookup.MessageType[msgType], name)
		}
		config.FilterChains[name] = chain
	}

	return nil
}
