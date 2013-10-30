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

// DecoderRunner wrapper that the MultiDecoder will hand to any subs that ask
// for one. Shadows some data and methods, but doesn't spin up any goroutines.
type mDRunner struct {
	*dRunner
	decoder Decoder
	name    string
	subName string
}

func (mdr *mDRunner) Name() string {
	return mdr.name
}

func (mdr *mDRunner) SetName(name string) {
	mdr.name = name
}

func (mdr *mDRunner) Plugin() Plugin {
	return mdr.decoder.(Plugin)
}

func (mdr *mDRunner) Decoder() Decoder {
	return mdr.decoder
}

func (mdr *mDRunner) LogError(err error) {
	log.Printf("SubDecoder '%s' error: %s", mdr.name, err)
}

func (mdr *mDRunner) LogMessage(msg string) {
	log.Printf("SubDecoder '%s': %s", mdr.name, msg)
}

type MultiDecoder struct {
	Config    *MultiDecoderConfig
	Name      string
	Decoders  map[string]Decoder
	dRunner   DecoderRunner
	CascStrat int
}

type MultiDecoderConfig struct {
	// subs is an ordered dictionary of other decoders
	Subs            map[string]interface{}
	Order           []string
	Name            string
	LogSubErrors    bool   `toml:"log_sub_errors"`
	CascadeStrategy string `toml:"cascade_strategy"`
}

const (
	CASC_FIRST_WINS = iota
	CASC_ALL
)

var mdStrategies = map[string]int{"first-wins": CASC_FIRST_WINS, "all": CASC_ALL}

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

	var ok bool
	if md.CascStrat, ok = mdStrategies[md.Config.CascadeStrategy]; !ok {
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

// Heka will call this to give us access to the runner. We'll store it for
// ourselves, but also have to pass on a wrapped version to any subdecoders
// that might need it.
func (md *MultiDecoder) SetDecoderRunner(dr DecoderRunner) {
	md.dRunner = dr
	for subName, decoder := range md.Decoders {
		if wanter, ok := decoder.(WantsDecoderRunner); ok {
			// It wants a DecoderRunner, have to create one. But first we need
			// to get our hands on a *dRunner.
			var realDRunner *dRunner
			if realDRunner, ok = dr.(*dRunner); !ok {
				// It's not a *dRunner, maybe it's an *mDRunner?
				var mdr *mDRunner
				if mdr, ok = dr.(*mDRunner); ok {
					// Bingo, we can grab its *dRunner.
					realDRunner = mdr.dRunner
				}
			}
			if realDRunner == nil {
				// Couldn't get a *dRunner. Just log an error and pass the
				// outer DecoderRunner through.
				dr.LogError(fmt.Errorf("Can't create nested DecoderRunner for '%s'",
					subName))
				wanter.SetDecoderRunner(dr)
				continue
			}
			// We have a *dRunner, use it to create an *mDRunner.
			subRunner := &mDRunner{
				realDRunner,
				decoder,
				fmt.Sprintf("%s-%s", realDRunner.name, subName),
				subName,
			}
			wanter.SetDecoderRunner(subRunner)
		}
	}
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
			if md.CascStrat == CASC_FIRST_WINS {
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
