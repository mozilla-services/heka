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
# ***** END LICENSE BLOCK *****/

package pipeline

import (
	"fmt"
	"github.com/bbangert/toml"
	"log"
)

type MultiDecoder struct {
	Config   *MultiDecoderConfig
	Name     string
	Decoders map[string]Decoder
	dRunner  DecoderRunner
}

type MultiDecoderConfig struct {
	// subs is an ordered dictionary of other decoders
	Subs            map[string]interface{}
	Order           []string
	Name            string
	LogSubErrors    bool   `toml:"log_sub_errors"`
	CascadeStrategy string `toml:"cascade_strategy"`
}

var mdStrategies = map[string]bool{"first-wins": false, "all": false}

func (md *MultiDecoder) ConfigStruct() interface{} {
	subs := make(map[string]interface{})
	order := make([]string, 0)
	name := fmt.Sprintf("MultiDecoder-%p", md)
	return &MultiDecoderConfig{subs, order, name, false, "first-wins"}
}

func (md *MultiDecoder) Init(config interface{}) (err error) {
	md.Config = config.(*MultiDecoderConfig)
	md.Decoders = make(map[string]Decoder, 0)
	md.Name = md.Config.Name

	if _, ok := mdStrategies[md.Config.CascadeStrategy]; !ok {
		return fmt.Errorf("Unrecognized cascade strategy: %s", md.Config.CascadeStrategy)
	}

	// run PrimitiveDecode against each subsection here and bind
	// it into the md.Decoders map
	for name, conf := range md.Config.Subs {
		md.log(fmt.Sprintf("MultiDecoder[%s] Loading: %s", md.Name, name))
		decoder, err := md.loadSection(name, conf)
		if err != nil {
			return err
		}
		md.Decoders[name] = decoder
	}
	return nil
}

func (md *MultiDecoder) log(msg string) {
	if md.dRunner != nil {
		md.dRunner.LogMessage(msg)
	} else {
		log.Println(msg)
	}
}

// loadSection must be passed a plugin name and the config for that plugin. It
// will create a PluginWrapper (i.e. a factory).
func (md *MultiDecoder) loadSection(sectionName string,
	configSection toml.Primitive) (plugin Decoder, err error) {
	var ok bool
	var pluginGlobals PluginGlobals
	var pluginType string

	wrapper := new(PluginWrapper)
	wrapper.name = sectionName

	// Setup default retry policy
	pluginGlobals.Retries = RetryOptions{
		MaxDelay:   "30s",
		Delay:      "250ms",
		MaxRetries: -1,
	}

	if err = toml.PrimitiveDecode(configSection, &pluginGlobals); err != nil {
		err = fmt.Errorf("%s Unable to decode config for plugin: %s, error: %s", md.Name, wrapper.name, err.Error())
		md.log(err.Error())
		return
	}

	if pluginGlobals.Typ == "" {
		pluginType = sectionName
	} else {
		pluginType = pluginGlobals.Typ
	}

	if wrapper.pluginCreator, ok = AvailablePlugins[pluginType]; !ok {
		err = fmt.Errorf("%s No such plugin: %s (type: %s)", md.Name, wrapper.name, pluginType)
		md.log(err.Error())
		return
	}

	// Create plugin, test config object generation.
	plugin = wrapper.pluginCreator().(Decoder)
	var config interface{}
	if config, err = LoadConfigStruct(configSection, plugin); err != nil {
		err = fmt.Errorf("%s Can't load config for %s '%s': %s", md.Name,
			sectionName,
			wrapper.name, err)
		md.log(err.Error())
		return
	}
	wrapper.configCreator = func() interface{} { return config }

	// Apply configuration to instantiated plugin.
	if err = plugin.(Plugin).Init(config); err != nil {
		err = fmt.Errorf("Initialization failed for '%s': %s",
			sectionName, err)
		md.log(err.Error())
		return
	}
	return
}

// Heka will call this to give us access to the runner.
func (md *MultiDecoder) SetDecoderRunner(dr DecoderRunner) {
	md.dRunner = dr
}

// Runs the message payload against each of the decoders.
func (md *MultiDecoder) Decode(pack *PipelinePack) (err error) {
	var (
		d       Decoder
		newType string
	)

	if pack.Message.GetType() == "" {
		newType = fmt.Sprintf("heka.%s", md.Name)
	} else {
		newType = fmt.Sprintf("heka.%s.%s", md.Name, pack.Message.Type)
	}
	pack.Message.SetType(newType)

	anyMatch := false
	for _, decoder_name := range md.Config.Order {
		d = md.Decoders[decoder_name]

		if err = d.Decode(pack); err == nil {
			if md.Config.CascadeStrategy == "first-wins" {
				return
			} else { // cascade_strategy == "all"
				anyMatch = true
			}
		}
		if md.Config.LogSubErrors && err != nil {
			err = fmt.Errorf("Subdecoder '%s' decode error: %s", decoder_name, err)
			md.dRunner.LogError(err)
		}
	}

	// `anyMatch` can only be set to true if cascade_strategy == "all".
	if anyMatch {
		return nil
	}
	pack.Recycle()
	return fmt.Errorf("Unable to decode message with any contained decoder.")
}
