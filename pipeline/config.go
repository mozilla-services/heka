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
#   Mike Trinkala (trink@mozilla.com)
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
	"regexp"
	"strings"
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
	StopChan() (stopChan chan interface{})
}

// Indicates a plug-in has a specific-to-itself config struct that should be
// passed in to its Init method.
type HasConfigStruct interface {
	ConfigStruct() interface{}
}

// Master config object encapsulating the entire heka/pipeline configuration.
type PipelineConfig struct {
	Inputs        map[string]Input
	Decoders      map[string]*PluginWrapper // multiple instances are allowed
	FilterRunners map[string]FilterRunner
	OutputRunners map[string]OutputRunner
	PoolSize      int
	Router        *MessageRouter
	RecycleChan   chan *PipelinePack
}

// Creates and initializes a PipelineConfig object.
func NewPipelineConfig(poolSize int) (config *PipelineConfig) {
	PoolSize = poolSize
	config = new(PipelineConfig)
	config.PoolSize = poolSize
	config.Inputs = make(map[string]Input)
	config.Decoders = make(map[string]*PluginWrapper)
	config.FilterRunners = make(map[string]FilterRunner)
	config.OutputRunners = make(map[string]OutputRunner)
	config.Router = new(MessageRouter)
	config.Router.InChan = make(chan *PipelinePack, PIPECHAN_BUFSIZE)
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

func (self *PipelineConfig) PackSupply() chan *PipelinePack {
	return self.RecycleChan
}

func (self *PipelineConfig) NewDecoder(name string) (
	runner DecoderRunner, ok bool) {

	var wrapper *PluginWrapper
	if wrapper, ok = self.Decoders[name]; !ok {
		return
	}
	decoder := wrapper.Create().(Decoder)
	runner = NewDecoderRunner(name, decoder)
	runner.Start()
	return
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
	Outputs  []PluginConfig
	Filters  map[string]PluginConfig
}

// A helper function to simplify plugin creation
type PluginWrapper struct {
	name          string
	configCreator func() interface{}
	pluginCreator func() interface{}
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
	err = plugin.(Plugin).Init(self.configCreator())
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
// decoding messages with a particular message encoding header value
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

	inputs, err := loadSection("inputs", configFile.Inputs)
	if err != nil {
		return err
	}

	// Setup our message generator input
	MessageGenerator.Init()
	mgiWrapper := new(PluginWrapper)
	mgiWrapper.name = "MessageGeneratorInput"
	mgiWrapper.pluginCreator = func() interface{} { return new(MessageGeneratorInput) }
	mgiWrapper.configCreator = func() interface{} { return new(PluginConfig) }
	inputs[mgiWrapper.name] = mgiWrapper
	for k, v := range inputs {
		tmp, err := v.CreateWithError()
		if err != nil {
			return errors.New("Unable to plugin init: " + err.Error())
		}
		self.Inputs[k] = tmp.(Input)
	}

	self.Decoders, err = loadSection("decoders", configFile.Decoders)
	if err != nil {
		return errors.New("Error reading decoders: " + err.Error())
	}

	outputs, err := loadSection("outputs", configFile.Outputs)
	if err != nil {
		return errors.New("Error reading outputs: " + err.Error())
	}
	for k, v := range outputs {
		tmp, err := v.CreateWithError()
		if err != nil {
			return errors.New("Unable to plugin init: " + err.Error())
		}
		self.OutputRunners[k] = NewOutputRunner(k, tmp.(Output))
	}

	for name, section := range configFile.Filters {
		runner := &filterRunner{}
		runner.name = name
		runner.inChan = make(chan *PipelinePack, PIPECHAN_BUFSIZE)

		if _, ok := section["message_filter"]; ok {
			msgFilter := section["message_filter"].(string)
			fs, err := CreateFilterSpecification(msgFilter)
			if err != nil {
				return err
			}
			runner.messageFilter = fs
		}

		if _, ok := section["output_timer"]; ok {
			sec := section["output_timer"].(float64)
			runner.output_timer = uint(sec)
		}
		if runner.output_timer == 0 {
			runner.output_timer = 60
		}

		if outputs, ok := section["outputs"]; ok {
			outputList := outputs.([]interface{})
			runner.outputs = make([]OutputRunner, len(outputList))
			for i, output := range outputList {
				strOutput := output.(string)
				orunner, ok := self.OutputRunners[strOutput]
				if !ok {
					return errors.New("Error during chain loading. Output by name " +
						strOutput + " was not defined.")
				}
				runner.outputs[i] = orunner
			}
		}
		var ok bool
		wrapper := new(PluginWrapper)
		pluginType := section["type"].(string)
		wrapper.name = name

		if wrapper.pluginCreator, ok = AvailablePlugins[pluginType]; !ok {
			err := errors.New("No such plugin of that name: " + pluginType)
			return err
		}
		plugin := wrapper.pluginCreator()
		configLoaded, err := LoadConfigStruct(&section, plugin)
		if err != nil {
			return err
		}
		wrapper.configCreator = func() interface{} { return configLoaded }
		tmp, err := wrapper.CreateWithError()
		if err != nil {
			return errors.New("Unable to plugin init: " + err.Error())
		}
		runner.filter = tmp.(Filter)
		self.FilterRunners[name] = runner
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
	RegisterPlugin("FileOutput", func() interface{} {
		return new(FileOutput)
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
	RegisterPlugin("SandboxFilter", func() interface{} {
		return new(SandboxFilter)
	})
	RegisterPlugin("CounterFilter", func() interface{} {
		return new(CounterFilter)
	})
	//	RegisterPlugin("PassThruFilter", func() interface{} {
	//		return new(PassThruFilter)
	//	})

	// The below code should really be in an init() function in
	// date_helpers.go, but go's compiler loses track of line numbers when
	// functions are spread across multiple files, so we've merged all init()
	// functions into one file per package for now. (see
	// https://code.google.com/p/go/issues/detail?id=4020)
	HelperRegexSubs = make(map[string]string)

	smonths := "(?:" + strings.Join(shortMonthNames, "|") + ")"
	sdays := "(?:" + strings.Join(shortDayNames, "|") + ")"
	days := "(?:" + strings.Join(longDayNames, "|") + ")"

	newMatchStrings := make([]string, 0, 15)
	replaceShorts, _ := regexp.Compile("(SDAY|DAY|SMONTH)")
	for _, dateStr := range dateMatchStrings {
		newStr := replaceShorts.ReplaceAllStringFunc(dateStr,
			func(match string) string {
				switch match {
				case "SDAY":
					return sdays
				case "DAY":
					return days
				case "SMONTH":
					return smonths
				}
				return match
			})
		newMatchStrings = append(newMatchStrings, "(?:"+newStr+")")
	}
	tsRegexString := "(?P<Timestamp>" + strings.Join(newMatchStrings, "|") + ")"
	HelperRegexSubs["TIMESTAMP"] = tsRegexString
}
