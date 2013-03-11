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
	"log"
	"os"
	"regexp"
	"strings"
	"time"
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
	Decoder(name string) (dRunner DecoderRunner, ok bool)
	Decoders() (decoders map[string]DecoderRunner)
	DecodersByEncoding() (decoders []DecoderRunner)
	Output(name string) (oRunner OutputRunner, ok bool)
	Router() (router *MessageRouter)
}

// Indicates a plug-in has a specific-to-itself config struct that should be
// passed in to its Init method.
type HasConfigStruct interface {
	ConfigStruct() interface{}
}

// Master config object encapsulating the entire heka/pipeline configuration.
type PipelineConfig struct {
	InputRunners    map[string]InputRunner
	DecoderWrappers map[string]*PluginWrapper // multiple instances are allowed
	FilterRunners   map[string]FilterRunner
	OutputRunners   map[string]OutputRunner
	PoolSize        int
	router          *MessageRouter
	RecycleChan     chan *PipelinePack
}

// Creates and initializes a PipelineConfig object.
func NewPipelineConfig(poolSize int) (config *PipelineConfig) {
	PoolSize = poolSize
	config = new(PipelineConfig)
	config.PoolSize = poolSize
	config.InputRunners = make(map[string]InputRunner)
	config.DecoderWrappers = make(map[string]*PluginWrapper)
	config.FilterRunners = make(map[string]FilterRunner)
	config.OutputRunners = make(map[string]OutputRunner)
	config.router = NewMessageRouter()
	config.RecycleChan = make(chan *PipelinePack, poolSize+1)
	return config
}

// Returns a running DecoderRunner for the registered decoder of the
// given name.
func (self *PipelineConfig) Decoder(name string) (
	dRunner DecoderRunner, ok bool) {

	var wrapper *PluginWrapper
	if wrapper, ok = self.DecoderWrappers[name]; !ok {
		return
	}
	decoder := wrapper.Create().(Decoder)
	dRunner = NewDecoderRunner(name, decoder)
	dRunner.Start()
	return
}

// Returns a map[string]DecoderRunner containing all registered decoders.
func (self *PipelineConfig) Decoders() (decoders map[string]DecoderRunner) {
	decoders = make(map[string]DecoderRunner)
	var runner DecoderRunner
	for name, wrapper := range self.DecoderWrappers {
		decoder := wrapper.Create().(Decoder)
		runner = NewDecoderRunner(name, decoder)
		runner.Start()
		decoders[name] = runner
	}
	return
}

// Returns a slice of DecoderRunners indexed by the Header_MessageEncoding
// value that each decoder works for.
func (self *PipelineConfig) DecodersByEncoding() []DecoderRunner {
	decoders := make([]DecoderRunner, topHeaderMessageEncoding+1)
	for encoding, name := range DecodersByEncoding {
		decoder, ok := self.Decoder(name)
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

func (self *PipelineConfig) Output(name string) (oRunner OutputRunner, ok bool) {
	oRunner, ok = self.OutputRunners[name]
	return
}

func (self *PipelineConfig) Router() (router *MessageRouter) {
	return self.router
}

// The JSON config file spec
type ConfigFile struct {
	Inputs   []PluginConfig
	Decoders []PluginConfig
	Outputs  []PluginConfig
	Filters  []PluginConfig
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

// loadSection must be passed a section name and the config for that section.
// It will create a PluginWrapper (i.e. a factory) for each plugin. For
// decoders (which are created as needed) the PluginWrappers are stored for
// later use. For the other plugin types, we'll create the plugin, configure
// it, then create the appropriate plugin runner.
func (self *PipelineConfig) loadSection(sectionName string,
	configSection []PluginConfig) (errcnt uint) {

	var ok bool
	var err error
	for _, pluginConf := range configSection {
		wrapper := new(PluginWrapper)
		pluginType := pluginConf["type"].(string)

		// Use the specified plugin name or default to the type name
		if _, ok = pluginConf["name"]; ok {
			wrapper.name = pluginConf["name"].(string)
		} else {
			wrapper.name = pluginType
		}

		if wrapper.pluginCreator, ok = AvailablePlugins[pluginType]; !ok {
			log.Printf("No such plugin: %s", wrapper.name)
			errcnt++
			continue
		}

		// Create and configure the plugin.
		plugin := wrapper.pluginCreator()
		if config, err := LoadConfigStruct(&pluginConf, plugin); err != nil {
			log.Printf("Can't load config for %s '%s': %s", sectionName, wrapper.name,
				err)
			errcnt++
			continue
		} else {
			if err = plugin.(Plugin).Init(config); err != nil {
				log.Printf("Initialization failed for %s '%s': %s", sectionName,
					wrapper.name, err)
				errcnt++
				continue
			}
		}

		// For decoders check to see if we need to register against a protocol
		// header, store the wrapper and continue.
		if sectionName == "decoders" {
			if encodingName, ok := pluginConf["encoding_name"]; ok {
				err = regDecoderForHeader(wrapper.name, encodingName.(string))
				if err != nil {
					log.Printf("Can't register decoder '%s' for encoding '%s': %s",
						wrapper.name, encodingName, err)
					errcnt++
					continue
				}
			}
			self.DecoderWrappers[wrapper.name] = wrapper
			continue
		}

		// For inputs just store the runner and continue.
		if sectionName == "inputs" {
			self.InputRunners[wrapper.name] = NewInputRunner(wrapper.name, plugin.(Input))
			continue
		}

		// Filters and outputs have a few more config settings.
		runner := NewFORunner(wrapper.name, plugin.(Plugin))
		runner.name = wrapper.name
		var tickLength uint
		if _, ok = pluginConf["ticker_interval"]; ok {
			sec := pluginConf["ticker_interval"].(float64)
			tickLength = uint(sec)
		}

		if tickLength != 0 {
			runner.tickLength = time.Duration(tickLength) * time.Second
		}

		var fs *FilterSpecification
		if matchText, ok := pluginConf["message_filter"]; ok {
			matchStr := matchText.(string)
			if fs, err = CreateFilterSpecification(matchStr); err != nil {
				log.Printf("Can't create message matcher for '%s': %s", wrapper.name,
					err)
				errcnt++
				continue
			}
			runner.messageFilter = fs
		}

		switch sectionName {
		case "filters":
			if fs != nil {
				self.router.fMatchers = append(self.router.fMatchers, fs)
			}
			self.FilterRunners[runner.name] = runner
		case "outputs":
			if fs != nil {
				self.router.oMatchers = append(self.router.oMatchers, fs)
			}
			self.OutputRunners[runner.name] = runner
		}
	}

	return
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
		return
	}

	var errcnt uint

	if errcnt = self.loadSection("inputs", configFile.Inputs); errcnt != 0 {
		return fmt.Errorf("%d errors loading inputs", errcnt)
	}

	// Setup our message generator input
	MessageGenerator.Init()
	mgiWrapper := new(PluginWrapper)
	mgiWrapper.name = "MessageGeneratorInput"
	mgiWrapper.pluginCreator = func() interface{} { return new(MessageGeneratorInput) }
	mgiWrapper.configCreator = func() interface{} { return new(PluginConfig) }
	if mgi, err := mgiWrapper.CreateWithError(); err != nil {
		return fmt.Errorf("Error creating MGI: %s", err)
	} else {
		self.InputRunners[mgiWrapper.name] = NewInputRunner(mgiWrapper.name,
			mgi.(Input))
	}

	if errcnt = self.loadSection("decoders", configFile.Decoders); errcnt != 0 {
		return fmt.Errorf("%d errors loading decoders", errcnt)
	}

	if errcnt = self.loadSection("outputs", configFile.Outputs); errcnt != 0 {
		return fmt.Errorf("%d errors loading outputs", errcnt)
	}

	if errcnt = self.loadSection("filters", configFile.Filters); errcnt != 0 {
		return fmt.Errorf("%d errors loading filters", errcnt)
	}

	return
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
	RegisterPlugin("WhisperOutput", func() interface{} {
		return new(WhisperOutput)
	})
	RegisterPlugin("LogfileInput", func() interface{} {
		return new(LogfileInput)
	})
	RegisterPlugin("TextParserDecoder", func() interface{} {
		return new(TextParserDecoder)
	})
	RegisterPlugin("TcpOutput", func() interface{} {
		return new(TcpOutput)
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
