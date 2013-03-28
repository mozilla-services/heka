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
	"fmt"
	"github.com/BurntSushi/toml"
	. "github.com/mozilla-services/heka/message"
	"log"
	"regexp"
	"time"
)

// Cap size of our decoder set arrays
const MAX_HEADER_MESSAGEENCODING Header_MessageEncoding = 256

var (
	AvailablePlugins         = make(map[string]func() interface{})
	DecodersByEncoding       = make(map[Header_MessageEncoding]string)
	topHeaderMessageEncoding Header_MessageEncoding
	PluginTypeRegex          = regexp.MustCompile("^.*(Decoder|Filter|Input|Output)$")
)

func RegisterPlugin(name string, factory func() interface{}) {
	AvailablePlugins[name] = factory
}

type PluginConfig map[string]toml.Primitive

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
	logMsgs         []string
	decoderRunners  [][]DecoderRunner
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
	config.logMsgs = make([]string, 0, 4)
	config.decoderRunners = make([][]DecoderRunner, 0, 4)
	return config
}

// Actually creates and starts a decoder instance.
func (self *PipelineConfig) makeDecoder(name string) (dRunner DecoderRunner, ok bool) {
	var wrapper *PluginWrapper
	if wrapper, ok = self.DecoderWrappers[name]; !ok {
		return
	}
	decoder := wrapper.Create().(Decoder)
	dRunner = NewDecoderRunner(name, decoder)
	dRunner.Start()
	return
}

// Returns a running DecoderRunner for the registered decoder of the
// given name.
func (self *PipelineConfig) Decoder(name string) (dRunner DecoderRunner, ok bool) {
	dRunner, ok = self.makeDecoder(name)
	self.decoderRunners = append(self.decoderRunners, []DecoderRunner{dRunner})
	return
}

// Returns a map[string]DecoderRunner containing all registered decoders.
func (self *PipelineConfig) Decoders() (decoders map[string]DecoderRunner) {
	decoders = make(map[string]DecoderRunner)
	dSlice := make([]DecoderRunner, 0, len(self.DecoderWrappers))
	var runner DecoderRunner
	for name, wrapper := range self.DecoderWrappers {
		decoder := wrapper.Create().(Decoder)
		runner = NewDecoderRunner(name, decoder)
		runner.Start()
		decoders[name] = runner
		dSlice = append(dSlice, runner)
	}
	self.decoderRunners = append(self.decoderRunners, dSlice)
	return
}

// Returns a slice of DecoderRunners indexed by the Header_MessageEncoding
// value that each decoder works for.
func (self *PipelineConfig) DecodersByEncoding() []DecoderRunner {
	decoders := make([]DecoderRunner, topHeaderMessageEncoding+1)
	for encoding, name := range DecodersByEncoding {
		decoder, ok := self.makeDecoder(name)
		if !ok {
			continue
		}
		decoders[encoding] = decoder
	}
	self.decoderRunners = append(self.decoderRunners, decoders)
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

// The TOML config file spec
type ConfigFile PluginConfig
type PluginGlobals struct {
	Typ      string  `toml:"type"`
	Ticker   float64 `toml:"ticker_interval"`
	Encoding string  `toml:"encoding_name"`
	Matcher  string  `toml:"message_matcher"`
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
	defer func() {
		// Slight protection against Init call into plugin code.
		if r := recover(); r != nil {
			plugin = nil
			err = fmt.Errorf("'%s' Init() panicked: %s", self.name, r)
		}
	}()

	plugin = self.pluginCreator()
	err = plugin.(Plugin).Init(self.configCreator())
	return
}

// If `configable` supports the `HasConfigStruct` interface this will use said
// interface to fetch a config struct object and populate it w/ the values in
// provided `config`. If not, simply returns `config` unchanged.
func LoadConfigStruct(config toml.Primitive, configable interface{}) (
	configStruct interface{}, err error) {

	// On two lines for scoping reasons.
	hasConfigStruct, ok := configable.(HasConfigStruct)
	if !ok {
		// If we don't have a config struct, change it to a PluginConfig
		configStruct = new(PluginConfig)
		if err = toml.PrimitiveDecode(config, configStruct); err != nil {
			configStruct = nil
		}
		return
	}

	defer func() {
		// Slight protection against ConfigStruct call into plugin code.
		if r := recover(); r != nil {
			configStruct = nil
			err = fmt.Errorf("ConfigStruct() panicked: %s", r)
		}
	}()

	configStruct = hasConfigStruct.ConfigStruct()
	if err = toml.PrimitiveDecode(config, configStruct); err != nil {
		configStruct = nil
		err = fmt.Errorf("Can't unmarshal config: %s", err)
	}
	return
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

func (self *PipelineConfig) log(msg string) {
	self.logMsgs = append(self.logMsgs, msg)
	log.Println(msg)
}

// loadSection must be passed a plugin name and the config for that plugin.
// It will create a PluginWrapper (i.e. a factory). For
// decoders (which are created as needed) the PluginWrappers are stored for
// later use. For the other plugin types, we'll create the plugin, configure
// it, then create the appropriate plugin runner.
func (self *PipelineConfig) loadSection(sectionName string,
	configSection toml.Primitive) (errcnt uint) {
	var ok bool
	var err error
	var pluginGlobals PluginGlobals
	var pluginType string

	wrapper := new(PluginWrapper)
	wrapper.name = sectionName

	if err = toml.PrimitiveDecode(configSection, &pluginGlobals); err != nil {
		self.log(fmt.Sprint("Unable to locate type in plugin: %s", wrapper.name))
		errcnt++
		return
	}
	pluginType = pluginGlobals.Typ

	if wrapper.pluginCreator, ok = AvailablePlugins[pluginType]; !ok {
		self.log(fmt.Sprintf("No such plugin: %s", wrapper.name))
		errcnt++
		return
	}

	// Create plugin, test config object generation.
	plugin := wrapper.pluginCreator()
	var config interface{}
	if config, err = LoadConfigStruct(configSection, plugin); err != nil {
		self.log(fmt.Sprintf("Can't load config for %s '%s': %s", sectionName,
			wrapper.name, err))
		errcnt++
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
		self.log(fmt.Sprintf("Initialization failed for '%s': %s",
			sectionName, err))
		errcnt++
		return
	}

	// Determine the plugin type
	pluginCats := PluginTypeRegex.FindStringSubmatch(pluginGlobals.Typ)
	if len(pluginCats) < 2 {
		self.log(fmt.Sprintf("Type doesn't contain valid plugin name: %s", pluginGlobals.Typ))
		errcnt++
		return
	}
	pluginCategory := pluginCats[1]

	// For decoders check to see if we need to register against a protocol
	// header, store the wrapper and continue.
	if pluginCategory == "Decoder" {
		if pluginGlobals.Encoding != "" {
			err = regDecoderForHeader(pluginGlobals.Typ, pluginGlobals.Encoding)
			if err != nil {
				self.log(fmt.Sprintf(
					"Can't register decoder '%s' for encoding '%s': %s",
					wrapper.name, pluginGlobals.Encoding, err))
				errcnt++
				return
			}
		}
		self.DecoderWrappers[wrapper.name] = wrapper
		return
	}

	// For inputs just store the runner and continue.
	if pluginCategory == "Input" {
		self.InputRunners[wrapper.name] = NewInputRunner(wrapper.name, plugin.(Input))
		return
	}

	// Filters and outputs have a few more config settings.
	runner := NewFORunner(wrapper.name, plugin.(Plugin))
	runner.name = wrapper.name
	var tickLength uint
	if pluginGlobals.Ticker != 0 {
		sec := pluginGlobals.Ticker
		tickLength = uint(sec)
	}

	if tickLength != 0 {
		runner.tickLength = time.Duration(tickLength) * time.Second
	}

	var matcher *MatchRunner
	if pluginGlobals.Matcher != "" {
		if matcher, err = NewMatchRunner(pluginGlobals.Matcher); err != nil {
			self.log(fmt.Sprintf("Can't create message matcher for '%s': %s",
				wrapper.name, err))
			errcnt++
			return
		}
		runner.matcher = matcher
	}

	switch pluginCategory {
	case "Filter":
		if matcher != nil {
			self.router.fMatchers = append(self.router.fMatchers, matcher)
		}
		self.FilterRunners[runner.name] = runner
	case "Output":
		if matcher != nil {
			self.router.oMatchers = append(self.router.oMatchers, matcher)
		}
		self.OutputRunners[runner.name] = runner
	}

	return
}

// LoadFromConfigFile loads a TOML configuration file and stores the
// result in the value pointed to by config. The maps in the config
// will be initialized as needed.
//
// The PipelineConfig should be already initialized before passed in via
// its Init function.
func (self *PipelineConfig) LoadFromConfigFile(filename string) (err error) {
	var configFile ConfigFile
	if _, err := toml.DecodeFile(filename, &configFile); err != nil {
		return fmt.Errorf("Error decoding config file: %s", err)
	}

	// Load all the plugins
	var errcnt uint
	for name, conf := range configFile {
		log.Println("Loading: ", name)
		errcnt += self.loadSection(name, conf)
	}
	if errcnt != 0 {
		return fmt.Errorf("%d errors loading plugins", errcnt)
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
}
