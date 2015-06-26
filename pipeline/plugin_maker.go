/***** BEGIN LICENSE BLOCK *****
# This Source Code Form is subject to the terms of the Mozilla Public
# License, v. 2.0. If a copy of the MPL was not distributed with this file,
# You can obtain one at http://mozilla.org/MPL/2.0/.
#
# The Initial Developer of the Original Code is the Mozilla Foundation.
# Portions created by the Initial Developer are Copyright (C) 2015
# the Initial Developer. All Rights Reserved.
#
# Contributor(s):
#   Rob Miller (rmiller@mozilla.com)
#
# ***** END LICENSE BLOCK *****/

package pipeline

import (
	"errors"
	"fmt"
	"reflect"
	"strings"

	"github.com/bbangert/toml"
	"github.com/mozilla-services/heka/message"
)

type PluginMaker interface {
	Name() string
	Type() string
	Category() string
	Config() interface{}
	PrepConfig() (interface{}, error)
	Make() (Plugin, interface{}, error)
	MakeRunner(name string) (PluginRunner, error)
}

// MutableMaker is for consumers that want to override the standard PluginMaker
// behavior. Use at your own risk. Both interfaces are implemented by the same
// struct, so a PluginMaker can be turned into a MutableMaker via type
// coercion.
type MutableMaker interface {
	PluginMaker
	OrigPrepConfig() (interface{}, error)
	SetPrepConfig(func() (interface{}, error))
	OrigPrepCommonTypedConfig() (interface{}, error)
	SetPrepCommonTypedConfig(func() (interface{}, error))
	SetName(name string)
	SetType(typ string)
	SetCategory(category string)
}

type pluginMaker struct {
	name                  string
	category              string
	tomlSection           toml.Primitive
	constructor           func() interface{}
	commonConfig          CommonConfig
	prepConfig            func() (interface{}, error)
	prepCommonTypedConfig func() (interface{}, error)
	pConfig               *PipelineConfig
	plugin                Plugin
}

// NewPluginMaker creates and returns a PluginMaker that can generate running
// plugins for the provided TOML configuration. It will load the plugin type
// and extract any of the Heka-defined common config for the plugin before
// returning.
func NewPluginMaker(name string, pConfig *PipelineConfig, tomlSection toml.Primitive) (
	PluginMaker, error) {

	// Create the maker, extract plugin type, and make sure the plugin type
	// exists.
	maker := &pluginMaker{
		name:         name,
		tomlSection:  tomlSection,
		commonConfig: CommonConfig{},
		pConfig:      pConfig,
	}

	var err error
	if err = toml.PrimitiveDecode(tomlSection, &maker.commonConfig); err != nil {
		return nil, fmt.Errorf("can't decode common config for '%s': %s", name, err)
	}
	if maker.commonConfig.Typ == "" {
		maker.commonConfig.Typ = name
	}
	constructor, ok := AvailablePlugins[maker.commonConfig.Typ]
	if !ok {
		return nil, fmt.Errorf("No registered plugin type: %s", maker.commonConfig.Typ)
	}
	maker.constructor = constructor
	maker.plugin = maker.makePlugin() // Only used to generate config structs.

	// Extract plugin category and any category-specific common (i.e. Heka
	// defined) configuration.
	maker.category = getPluginCategory(maker.commonConfig.Typ)
	if maker.category == "" {
		return nil, errors.New("Unrecognized plugin category")
	}

	maker.prepCommonTypedConfig = maker.OrigPrepCommonTypedConfig
	_, err = maker.prepCommonTypedConfig()
	if err != nil {
		return nil, fmt.Errorf("can't decode common %s config for '%s': %s",
			strings.ToLower(maker.category), name, err)
	}

	maker.prepConfig = maker.OrigPrepConfig

	return maker, nil
}

// OrigPrepCommonTypedConfig is the default implementation of the
// `PrepCommonTypedConfig` function.
func (m *pluginMaker) OrigPrepCommonTypedConfig() (interface{}, error) {
	var (
		commonTypedConfig interface{}
		err               error
	)
	switch m.category {
	case "Input":
		commonInput := CommonInputConfig{
			Retries: getDefaultRetryOptions(),
		}
		err = toml.PrimitiveDecode(m.tomlSection, &commonInput)
		commonTypedConfig = commonInput
	case "Filter", "Output":
		commonFO := CommonFOConfig{
			Retries: getDefaultRetryOptions(),
		}
		err = toml.PrimitiveDecode(m.tomlSection, &commonFO)
		commonTypedConfig = commonFO
	case "Splitter":
		commonSplitter := CommonSplitterConfig{}
		err = toml.PrimitiveDecode(m.tomlSection, &commonSplitter)
		commonTypedConfig = commonSplitter
	}
	if err != nil {
		return nil, err
	}
	return commonTypedConfig, nil
}

// PrepCommonTypedConfig returns a struct containing all of the common
// configuration for the maker's plugin type, populated from the provided TOML.
func (m *pluginMaker) PrepCommonTypedConfig() (interface{}, error) {
	return m.prepCommonTypedConfig()
}

func (m *pluginMaker) Name() string {
	return m.name
}

func (m *pluginMaker) Type() string {
	return m.commonConfig.Typ
}

func (m *pluginMaker) Category() string {
	return m.category
}

// SetPrepCommonTypedConfig allows plugins a mechanism for overriding
// TOML-loaded config in the per-type common configuration settings.
func (m *pluginMaker) SetPrepCommonTypedConfig(prepCommonTypedConfig func() (interface{}, error)) {
	m.prepCommonTypedConfig = prepCommonTypedConfig
}

func (m *pluginMaker) SetName(name string) {
	m.name = name
}

func (m *pluginMaker) SetType(typ string) {
	m.commonConfig.Typ = typ
}

func (m *pluginMaker) SetCategory(category string) {
	m.category = category
}

// makePlugin instantiates a plugin instance, provides name and pConfig to the
// plugin if necessary, and returns the plugin.
func (m *pluginMaker) makePlugin() Plugin {
	plugin := m.constructor().(Plugin)
	if wantsPConfig, ok := plugin.(WantsPipelineConfig); ok {
		wantsPConfig.SetPipelineConfig(m.pConfig)
	}
	if wantsName, ok := plugin.(WantsName); ok {
		wantsName.SetName(m.name)
	}
	return plugin
}

// Config uses the maker's cached plugin instance to create a config object.
func (m *pluginMaker) Config() interface{} {
	hasConfigStruct, ok := m.plugin.(HasConfigStruct)
	if ok {
		return hasConfigStruct.ConfigStruct()
	} else {
		// If we don't have a config struct, fall back to a PluginConfig.
		return PluginConfig{}
	}
}

// OrigPrepConfig is the default implementation of the `PrepConfig` method.
func (m *pluginMaker) OrigPrepConfig() (interface{}, error) {
	config := m.Config()
	var err error

	if _, ok := config.(PluginConfig); ok {
		if err = toml.PrimitiveDecode(m.tomlSection, config); err != nil {
			return nil, fmt.Errorf("can't decode config for '%s': %s", m.name, err.Error())
		}
		return config, nil
	}

	// Use reflection to extract the fields (or TOML tag names, if available)
	// of the values that Heka has already extracted so we know they're not
	// required to be specified in the config struct.
	hekaParams := make(map[string]interface{})
	commonTypedConfig, _ := m.OrigPrepCommonTypedConfig()
	commons := []interface{}{m.commonConfig, commonTypedConfig}
	for _, common := range commons {
		if common == nil {
			continue
		}
		rt := reflect.ValueOf(common).Type()
		for i := 0; i < rt.NumField(); i++ {
			sft := rt.Field(i)
			kname := sft.Tag.Get("toml")
			if len(kname) == 0 {
				kname = sft.Name
			}
			hekaParams[kname] = true
		}
	}

	// Finally decode the TOML into the struct. Use of PrimitiveDecodeStrict
	// means that an error will be raised for any config options in the TOML
	// that don't have corresponding attributes on the struct, delta the
	// hekaParams that can be safely excluded.
	err = toml.PrimitiveDecodeStrict(m.tomlSection, config, hekaParams)
	if err != nil {
		matches := unknownOptionRegex.FindStringSubmatch(err.Error())
		if len(matches) == 2 {
			// We've got an unrecognized config option.
			err = fmt.Errorf("unknown config setting for '%s': %s", m.name, matches[1])
		}
		return nil, err
	}

	return config, nil
}

// PrepConfig decodes the maker's TOML config into a newly created config struct.
func (m *pluginMaker) PrepConfig() (interface{}, error) {
	return m.prepConfig()
}

// SetPrepConfig provides plugins a mechanism to override the TOML-loaded
// plugin specific configuration.
func (m *pluginMaker) SetPrepConfig(prepConfig func() (interface{}, error)) {
	m.prepConfig = prepConfig
}

// Make creates and returns a config struct and an initialized plugin.
func (m *pluginMaker) Make() (Plugin, interface{}, error) {
	config, err := m.prepConfig()
	if err != nil {
		return nil, nil, err
	}

	plugin := m.makePlugin()
	if err = plugin.Init(config); err != nil {
		return nil, nil, fmt.Errorf("Initialization failed for '%s': %s", m.name, err)
	}

	return plugin, config, nil
}

func (m *pluginMaker) makeSplitterRunner(name string, config interface{}, splitter Splitter) (*sRunner, error) {
	commonConfig, err := m.prepCommonTypedConfig()
	if err != nil {
		return nil, fmt.Errorf("Can't prep common typed config: %s", err.Error())
	}
	commonSplitter := commonConfig.(CommonSplitterConfig)
	if commonSplitter.KeepTruncated == nil {
		commonSplitter.KeepTruncated, err = getDefaultBool(config, "KeepTruncated")
		if err != nil {
			return nil, err
		}
	}
	if commonSplitter.UseMsgBytes == nil {
		commonSplitter.UseMsgBytes, err = getDefaultBool(config, "UseMsgBytes")
		if err != nil {
			return nil, err
		}
	}
	if commonSplitter.BufferSize == 0 {
		bufferSize := getAttr(config, "BufferSize", uint(8*1024))
		commonSplitter.BufferSize = bufferSize.(uint)
	}
	if uint32(commonSplitter.BufferSize) > message.MAX_RECORD_SIZE {
		err = fmt.Errorf("'min_buffer_size' (%d) can't be larger than MAX_RECORD_SIZE (%d)",
			commonSplitter.BufferSize, message.MAX_RECORD_SIZE)
		return nil, err
	}
	if commonSplitter.IncompleteFinal == nil {
		commonSplitter.IncompleteFinal, err = getDefaultBool(config, "IncompleteFinal")
		if err != nil {
			return nil, err
		}
	}
	sr := NewSplitterRunner(name, splitter, commonSplitter)
	sr.h = m.pConfig
	return sr, nil
}

func (m *pluginMaker) makeInputRunner(name string, config interface{}, input Input,
	defaultTick uint) (InputRunner, error) {

	commonConfig, err := m.prepCommonTypedConfig()
	if err != nil {
		return nil, fmt.Errorf("Can't prep common typed config: %s", err.Error())
	}
	commonInput := commonConfig.(CommonInputConfig)
	if commonInput.Ticker == 0 {
		commonInput.Ticker = defaultTick
	}
	if commonInput.SyncDecode == nil {
		if commonInput.SyncDecode, err = getDefaultBool(config, "SyncDecode"); err != nil {
			return nil, err
		}
	}
	if commonInput.SendDecodeFailures == nil {
		commonInput.SendDecodeFailures, err = getDefaultBool(config, "SendDecodeFailures")
		if err != nil {
			return nil, err
		}
	}
	if commonInput.CanExit == nil {
		if commonInput.CanExit, err = getDefaultBool(config, "CanExit"); err != nil {
			return nil, err
		}
	}
	if commonInput.Decoder == "" {
		decoder := getAttr(config, "Decoder", "")
		commonInput.Decoder = decoder.(string)
	}
	if commonInput.Splitter == "" {
		splitter := getAttr(config, "Splitter", "")
		commonInput.Splitter = splitter.(string)
	}
	runner := NewInputRunner(name, input, commonInput)
	return runner, nil
}

// MakeRunner returns a new, unstarted PluginRunner wrapped around a new,
// configured plugin instance. If name is provided, then the Runner will be
// given the specified name; if name is an empty string, the plugin name will
// be used.
func (m *pluginMaker) MakeRunner(name string) (PluginRunner, error) {
	if m.category == "Encoder" {
		return nil, fmt.Errorf("%s plugins don't support PluginRunners", m.category)
	}

	plugin, config, err := m.Make()
	if err != nil {
		return nil, err
	}

	if name == "" {
		name = m.name
	}

	var runner PluginRunner

	if m.category == "Decoder" {
		runner = NewDecoderRunner(name, plugin.(Decoder), m.pConfig.Globals.PluginChanSize)
		return runner, nil
	}

	if m.category == "Splitter" {
		return m.makeSplitterRunner(name, config, plugin.(Splitter))
	}

	// In some cases a plugin implementation will specify a default value for
	// one or more common config settings by including values for those
	// settings in the config struct. We extract them in this function's outer
	// scope, but we only need to use them if the common config hasn't already
	// been populated by values from the TOML.
	defaultTickerInterval := getAttr(config, "TickerInterval", uint(0))
	defaultTick := defaultTickerInterval.(uint)

	if m.category == "Input" {
		return m.makeInputRunner(name, config, plugin.(Input), defaultTick)
	}

	// We're a filter or an output.
	commonConfig, err := m.prepCommonTypedConfig()
	if err != nil {
		return nil, fmt.Errorf("Can't prep common typed config: %s", err.Error())
	}

	commonFO := commonConfig.(CommonFOConfig)
	// More checks for plugin-specified default values of common config
	// settings.
	if commonFO.Matcher == "" {
		matcherVal := getAttr(config, "MessageMatcher", "")
		commonFO.Matcher = matcherVal.(string)
	}

	if commonFO.Ticker == 0 {
		commonFO.Ticker = defaultTick
	}

	// Boolean types are tricky, we use pointer types to distinguish btn false
	// and not set, but a plugin's config struct might not be so smart, so we
	// have to account for both cases.
	if commonFO.CanExit == nil {
		if commonFO.CanExit, err = getDefaultBool(config, "CanExit"); err != nil {
			return nil, err
		}
	}

	if m.category == "Output" {
		if commonFO.UseFraming == nil {
			if commonFO.UseFraming, err = getDefaultBool(config, "UseFraming"); err != nil {
				return nil, err
			}
		}
		if commonFO.Encoder == "" {
			encoder := getAttr(config, "Encoder", "")
			commonFO.Encoder = encoder.(string)
		}
	}

	return NewFORunner(name, plugin, commonFO, m.commonConfig.Typ,
		m.pConfig.Globals.PluginChanSize)
}
