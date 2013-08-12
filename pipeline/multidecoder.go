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
	Decoders map[string]Decoder
	Order    []string

	dRunner DecoderRunner
}

type MultiDecoderConfig struct {
	// subs is an ordered dictionary of other decoders
	Subs  map[string]interface{}
	Order []string
}

func (md *MultiDecoder) ConfigStruct() interface{} {
	subs := make(map[string]interface{})
	order := make([]string, 0)
	return &MultiDecoderConfig{Subs: subs, Order: order}
}

func (md *MultiDecoder) Init(config interface{}) (err error) {
	conf := config.(*MultiDecoderConfig)
	md.Order = conf.Order
	md.Decoders = make(map[string]Decoder, 0)

	// run PrimitiveDecode against each subsection here and bind
	// it into the md.Decoders map

	for name, conf := range conf.Subs {
		md.log(fmt.Sprintf("MultiDecoder Loading: %s", name))
		decoder, err := md.loadSection(name, conf)
		if err != nil {
			return err
		}
		md.Decoders[name] = decoder
	}
	return nil
}

func (md *MultiDecoder) log(msg string) {
	log.Println(msg)
}

// loadSection must be passed a plugin name and the config for that plugin. It
// will create a PluginWrapper (i.e. a factory). For decoders we store the
// PluginWrappers and create pools of DecoderRunners for each type, stored in
// our decoder channels. For the other plugin types, we create the plugin,
// configure it, then create the appropriate plugin runner.
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
		err = fmt.Errorf("Unable to decode config for plugin: %s, error: %s", wrapper.name, err.Error())
		md.log(err.Error())
		return
	}

	if pluginGlobals.Typ == "" {
		pluginType = sectionName
	} else {
		pluginType = pluginGlobals.Typ
	}

	if wrapper.pluginCreator, ok = AvailablePlugins[pluginType]; !ok {
		err = fmt.Errorf("No such plugin: %s (type: %s)", wrapper.name, pluginType)
		md.log(err.Error())
		return
	}

	// Create plugin, test config object generation.
	plugin = wrapper.pluginCreator().(Decoder)
	var config interface{}
	if config, err = LoadConfigStruct(configSection, plugin); err != nil {
		err = fmt.Errorf("Can't load config for %s '%s': %s", sectionName,
			wrapper.name, err)
		md.log(err.Error())
		return
	}
	wrapper.configCreator = func() interface{} { return config }

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

// Runs the message payload against each of the decoders
func (md *MultiDecoder) Decode(pack *PipelinePack) (err error) {
	var d Decoder
	for _, decoder_name := range md.Order {
		d = md.Decoders[decoder_name]
		if err = d.Decode(pack); err == nil {
			return
		}
	}
	return fmt.Errorf("Unable to decode message with any contained decoder: [%s]", pack)
}
