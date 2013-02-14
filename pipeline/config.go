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
	. "github.com/mozilla-services/heka/message"
	"github.com/rafrombrc/go-notify"
	"os"
)

// Cap size of our decoder set arrays
const MAX_HEADER_MESSAGEENCODING Header_MessageEncoding = 256

var AvailablePlugins = make(map[string]func() interface{})
var DecodersByEncoding = make(map[Header_MessageEncoding]string)
var topHeaderMessageEncoding Header_MessageEncoding

func RegisterPlugin(name string, factory func() interface{}) {
	AvailablePlugins[name] = factory
}

type PluginConfig map[string]interface{}

type PluginHelper interface {
	PackSupply() chan *PipelinePack
	NewDecoder(name string) (runner DecoderRunner, ok bool)
	NewDecoderSet() (decoders []DecoderRunner)
	ChainRouter() *ChainRouter
	StopChan() (stopChan chan interface{})
}

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
	OutputRunners      map[string]*outputRunner
	FilterChains       map[string]FilterChain
	ChainMap           map[string][]string
	DefaultFilterChain string
	PoolSize           int
	RecycleChan        chan *PipelinePack
}

// Creates and initializes a PipelineConfig object.
func NewPipelineConfig(poolSize int) (config *PipelineConfig) {
	PoolSize = poolSize
	config = new(PipelineConfig)
	config.PoolSize = poolSize
	config.FilterChains = make(map[string]FilterChain)
	config.DefaultFilterChain = "default"
	config.ChainMap = make(map[string][]string)
	config.OutputRunners = make(map[string]*outputRunner)
	config.RecycleChan = make(chan *PipelinePack, poolSize+1)
	return config
}

// Returns a slice of DecoderRunners indexed by the Header_MessageEncoding
// value that each decoder works for.
func (self *PipelineConfig) NewDecoderSet() []DecoderRunner {
	decoders := make([]DecoderRunner, topHeaderMessageEncoding+1)
	for encoding, name := range DecodersByEncoding {
		decoder, ok := self.NewDecoder(name)
		if !ok {
			continue
		}
		decoders[encoding] = decoder
	}
	return decoders
}

func (self *PipelineConfig) NewDecoder(name string) (
	runner DecoderRunner, ok bool) {

	var wrapper *PluginWrapper
	if wrapper, ok = self.Decoders[name]; !ok {
		return
	}
	router := self.ChainRouter()
	decoder := wrapper.Create().(Decoder)
	runner = NewDecoderRunner(name, decoder, router)
	runner.Start()
	return
}

func (self *PipelineConfig) ChainRouter() *ChainRouter {
	router := new(ChainRouter)
	router.InChan = make(chan *PipelinePack, PIPECHAN_BUFSIZE)
	router.ChainMap = self.ChainMap
	router.Start()
	return router
}

func (self *PipelineConfig) PackSupply() chan *PipelinePack {
	return self.RecycleChan
}

func (self *PipelineConfig) StopChan() (stopChan chan interface{}) {
	stopChan = make(chan interface{})
	notify.Start(STOP, stopChan)
	return
}

// The JSON config file spec
type ConfigFile struct {
	Inputs   []PluginConfig
	Decoders []PluginConfig
	Filters  []PluginConfig
	Outputs  []PluginConfig
	Chains   map[string]PluginConfig
}

// A plugin wrapper wraps a plugin so that more instances of it can be
// easily created and a reference can be held to the plugins global if
// it needs one
type PluginWrapper struct {
	name          string
	configCreator func() interface{}
	pluginCreator func() interface{}
	global        PluginGlobal
	firstPlugin   interface{}
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
	if self.firstPlugin != nil {
		plugin = self.firstPlugin
		self.firstPlugin = nil
		return
	}

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

// Registers a particular decoder (specified by `decoderName`) to be used for
// decoding messages sent received a particular message encoding header value
// (specified by `encodingName`).
func regDecoderForHeader(decoderName string, encodingName string) (err error) {
	var encoding Header_MessageEncoding
	var ok bool
	if encodingInt32, ok := Header_MessageEncoding_value[encodingName]; !ok {
		err = fmt.Errorf("No Header_MessageEncoding named '%s'", encodingName)
		return
	} else {
		encoding = Header_MessageEncoding(encodingInt32)
	}
	if encoding > MAX_HEADER_MESSAGEENCODING {
		err = fmt.Errorf("Header_MessageEncoding '%s' value '%d' higher than max '%d'",
			encodingName, encoding, MAX_HEADER_MESSAGEENCODING)
		return
	}
	// Be nice to be able to verify that this is actually a decoder.
	if _, ok = AvailablePlugins[decoderName]; !ok {
		err = fmt.Errorf("No decoder named '%s' registered as a plugin", decoderName)
		return
	}
	if encoding > topHeaderMessageEncoding {
		topHeaderMessageEncoding = encoding
	}
	DecodersByEncoding[encoding] = decoderName
	return
}

// loadSection can be passed a configSection, the appropriate mapping
// of available plug-ins for that section, and will then create
// PluginWrappers for each plug-in instance defined
func loadSection(sectionName string, configSection []PluginConfig) (
	config map[string]*PluginWrapper, err error) {

	var ok bool
	config = make(map[string]*PluginWrapper)
	for _, pluginConf := range configSection {
		wrapper := new(PluginWrapper)
		pluginType := pluginConf["type"].(string)

		// Use the specified plugin name or default to the type name
		if _, ok := pluginConf["name"]; ok {
			wrapper.name = pluginConf["name"].(string)
		} else {
			wrapper.name = pluginType
		}

		// For decoders check to see if we need to register against a protocol
		// header
		if sectionName == "decoders" {
			if encodingName, ok := pluginConf["encoding_name"]; ok {
				err = regDecoderForHeader(wrapper.name, encodingName.(string))
				if err != nil {
					return nil, err
				}
			}
		}

		if wrapper.pluginCreator, ok = AvailablePlugins[pluginType]; !ok {
			err := errors.New("No such plugin of that name: " + pluginType)
			return nil, err
		}

		// Create an instance so we can see if we need to marshal the JSON
		plugin := wrapper.pluginCreator()
		configLoaded, err := LoadConfigStruct(&pluginConf, plugin)
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

		if plugin, err = wrapper.CreateWithError(); err != nil {
			return config, errors.New("Unable to plugin init: " + err.Error())
		}
		wrapper.firstPlugin = plugin
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

	if self.Inputs, err = loadSection("inputs", configFile.Inputs); err != nil {
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

	self.Decoders, err = loadSection("decoders", configFile.Decoders)
	if err != nil {
		return errors.New("Error reading decoders: " + err.Error())
	}

	self.Filters, err = loadSection("filters", configFile.Filters)
	if err != nil {
		return errors.New("Error reading filters: " + err.Error())
	}

	self.Outputs, err = loadSection("outputs", configFile.Outputs)
	if err != nil {
		return errors.New("Error reading outputs: " + err.Error())
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
		if msgTypes, ok := section["message_type"]; ok {
			messageTypeList := msgTypes.([]interface{})
			var msgType string
			for _, rawType := range messageTypeList {
				// Create the string slice for this msgType if it
				// doesn't exist already.

				// TODO: Later on we'll support additional ways to
				// restrict the chain used based on more than just the
				// message type which is why we store a slice of
				// strings here at the moment
				msgType = rawType.(string)
				if _, ok := self.ChainMap[msgType]; !ok {
					self.ChainMap[msgType] = make([]string, 0, 10)
				}
				self.ChainMap[msgType] = append(self.ChainMap[msgType], name)
			}
		}
		self.FilterChains[name] = chain

	}

	return nil
}

func init() {
	RegisterPlugin("UdpInput", func() interface{} {
		return new(UdpInput)
	})
	RegisterPlugin("TcpInput", func() interface{} {
		return new(TcpInput)
	})
	RegisterPlugin("JsonDecoder", func() interface{} {
		return new(JsonDecoder)
	})
	RegisterPlugin("ProtobufDecoder", func() interface{} {
		return new(ProtobufDecoder)
	})
	RegisterPlugin("StatsdInput", func() interface{} {
		return new(StatsdInput)
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
	RegisterPlugin("LogfileInput", func() interface{} {
		return new(LogfileInput)
	})
	RegisterPlugin("TextParserDecoder", func() interface{} {
		return new(TextParserDecoder)
	})
	RegisterPlugin("TcpOutput", func() interface{} {
		return RunnerMaker(new(TcpWriter))
	})
	RegisterPlugin("StatFilter", func() interface{} {
		return new(StatFilter)
	})
}
