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
	"fmt"
	"os"
)

type ConfigFile struct {
	Inputs   []PluginConfig
	Decoders []PluginConfig
	Filters  []PluginConfig
	Outputs  []PluginConfig
	Chains   map[string]PluginConfig
}

// LoadSection can be passed a config section, the appropriate mapping
// of available plugins for that section, and will then load and
// instantiate the plugins into the config value.
func LoadSection(configSection []PluginConfig, config map[string]Plugin) error {
	var (
		plugin                 Plugin
		pluginType, pluginName string
		ok                     bool
	)
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
			return errors.New("Error: No such plugin of that name")
		} else {
			plugin = pluginFunc()
			if err := plugin.Init(&section); err != nil {
				msg := fmt.Sprintf("Unable to load config section: %s. Error: %s",
					pluginName, err)
				return errors.New(msg)
			}
			config[pluginName] = plugin
		}
	}
	return nil
}

// LoadFromConfigFile loads a JSON configuration file and stores the
// result in the value pointed to by config. The maps in the config
// will be initialized as needed.
func LoadFromConfigFile(filename string, config *GraterConfig) error {
	file, err := os.Open(filename)
	if err != nil {
		return err
	}

	jsonBytes := make([]byte, 1e5)
	n, err := file.Read(jsonBytes)
	if err != nil {
		return err
	}
	jsonBytes = jsonBytes[:n]

	configFile := ConfigFile{}
	err = json.Unmarshal(jsonBytes, configFile)
	if err != nil {
		return err
	}

	config.Inputs = make(map[string]Input)
	config.Decoders = make(map[string]Decoder)
	config.FilterChains = make(map[string]FilterChain)
	config.Outputs = make(map[string]Output)
	config.Filters = make(map[string]Filter)

	inputConfig := make(map[string]Plugin)
	decoderConfig := make(map[string]Plugin)
	filterConfig := make(map[string]Plugin)
	outputConfig := make(map[string]Plugin)

	if err := LoadSection(configFile.Inputs, inputConfig); err != nil {
		return err
	} else {
		for name, plugin := range inputConfig {
			config.Inputs[name] = plugin.(Input)
		}
	}

	if err := LoadSection(configFile.Decoders, decoderConfig); err != nil {
		return err
	} else {
		for name, plugin := range decoderConfig {
			config.Decoders[name] = plugin.(Decoder)
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
	}

	if err := LoadSection(configFile.Filters, filterConfig); err != nil {
		return err
	} else {
		for name, plugin := range filterConfig {
			config.Filters[name] = plugin.(Filter)
		}
	}

	if err := LoadSection(configFile.Outputs, outputConfig); err != nil {
		return err
	} else {
		for name, plugin := range outputConfig {
			config.Outputs[name] = plugin.(Output)
		}
	}

	for name, section := range configFile.Chains {
		chain := FilterChain{}
		if outputs, ok := section["outputs"]; ok {
			outputList := outputs.([]string)
			chain.Outputs = make([]string, len(outputList))
			for i, output := range outputList {
				chain.Outputs[i] = output
			}
		}
		if filters, ok := section["filters"]; ok {
			filterList := filters.([]string)
			chain.Filters = make([]string, len(filterList))
			for i, filter := range filterList {
				chain.Filters[i] = filter
			}
		}
		config.FilterChains[name] = chain
	}

	return nil
}
