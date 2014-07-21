package sysinfo

import (
	"bytes"
	"code.google.com/p/go-uuid/uuid"
	"errors"
	"fmt"
	"github.com/mozilla-services/heka/message"
	"github.com/mozilla-services/heka/pipeline"
	"io/ioutil"
	"os"
	"strconv"
	"syscall"
	"time"
)

const loads_shift = 1 << 16

var (
	getSysinfo            func(*syscall.Sysinfo_t) error
	proc_meminfo_location string
)

func init() {
	pipeline.RegisterPlugin("SysinfoInput", func() interface{} {
		getSysinfo = syscall.Sysinfo
		proc_meminfo_location = "/proc/meminfo"
		return new(SysinfoInput)
	})
}

// ErrCantAddField should be returned if a field cannot be added to a message
var ErrCantAddField = errors.New("Cant add field")

// SysinfoInput is the struct containing the plugins internal state and config
type SysinfoInput struct {
	*SysinfoInputConfig
	stop   chan bool
	runner pipeline.InputRunner
}

// SysinfoInputConfig contains configuration options for the SysinfoInput plugin
type SysinfoInputConfig struct {
	TickerInterval uint   `toml:"ticker_interval"`
	DecoderName    string `toml:"decoder"`
}

// ConfigStruct returns the default config for SysinfoInput
func (input *SysinfoInput) ConfigStruct() interface{} {
	return &SysinfoInputConfig{
		TickerInterval: uint(5),
	}
}

func (input *SysinfoInput) Init(config interface{}) error {
	conf := config.(*SysinfoInputConfig)
	input.SysinfoInputConfig = conf
	input.stop = make(chan bool)
	return nil
}

func (input *SysinfoInput) Stop() {
	close(input.stop)
}

func (input *SysinfoInput) Run(runner pipeline.InputRunner,
	helper pipeline.PluginHelper) error {

	var (
		info                syscall.Sysinfo_t
		pack                *pipeline.PipelinePack
		dRunner             pipeline.DecoderRunner
		router_shortcircuit bool
		ok                  bool
	)

	if input.DecoderName == "" {
		router_shortcircuit = true
	} else if dRunner, ok = helper.DecoderRunner(input.DecoderName,
		fmt.Sprintf("%s-%s", runner.Name(), input.DecoderName)); !ok {
		return fmt.Errorf("Decoder not found: %s", input.DecoderName)
	}

	input.runner = runner

	pConfig := helper.PipelineConfig()
	packSupply := runner.InChan()
	tickChan := runner.Ticker()
	hostname := pConfig.Hostname()
	meminfo := make(map[string]int)

	for {
		select {
		case <-input.stop:
			return nil
		case <-tickChan:
		}
		err := getSysinfo(&info)
		if err != nil {
			return err
		}
		pack = <-packSupply
		pack.Message.SetHostname(hostname)
		input.setSysinfoMessage(pack, &info)
		if router_shortcircuit {
			runner.Inject(pack)
		} else {
			dRunner.InChan() <- pack
		}

		err = Meminfo(meminfo)
		if err != nil {
			return err
		}
		pack = <-packSupply
		pack.Message.SetHostname(hostname)
		input.setMeminfoMessage(pack, meminfo)
		if router_shortcircuit {
			runner.Inject(pack)
		} else {
			dRunner.InChan() <- pack
		}

	}
	return nil
}

// AddField is a wrapper around Message.AddField which logs an error if it
// cannot create the message field.
func (input *SysinfoInput) AddField(pack *pipeline.PipelinePack, name string,
	value interface{}, representation string) {
	if field, err := message.NewField(name, value, representation); err == nil {
		pack.Message.AddField(field)
	} else {
		input.runner.LogError(fmt.Errorf("%s: %s", ErrCantAddField, err))
	}
}

func (input *SysinfoInput) setSysinfoMessage(pack *pipeline.PipelinePack, info *syscall.Sysinfo_t) {
	pack.Message.SetUuid(uuid.NewRandom())
	pack.Message.SetTimestamp(time.Now().UnixNano())
	pack.Message.SetType("heka.sysinfo.sysinfo")
	// Cpu load avg
	input.AddField(pack, "OneMinLoadAvg", float64(info.Loads[0])/float64(loads_shift), "")
	input.AddField(pack, "FiveMinLoadAvg", float64(info.Loads[1])/float64(loads_shift), "")
	input.AddField(pack, "FifteenMinLoadAvg", float64(info.Loads[2])/float64(loads_shift), "")
	// Memory
	input.AddField(pack, "Totalram", int(info.Totalram), "B")
	input.AddField(pack, "Freeram", int(info.Freeram), "B")
	input.AddField(pack, "Sharedram", int(info.Sharedram), "B")
	input.AddField(pack, "Bufferram", int(info.Bufferram), "B")
	input.AddField(pack, "Totalswap", int(info.Totalswap), "B")
	input.AddField(pack, "Freeswap", int(info.Freeswap), "B")
	input.AddField(pack, "Processes", int(info.Procs), "")
}

func Meminfo(meminfo map[string]int) error {
	if proc_meminfo_location == "" {
		panic("proc_meminfo_location cannot be empty.")
	}
	f, err := os.Open(proc_meminfo_location)
	if err != nil {
		return err
	}
	data, err := ioutil.ReadAll(f)
	var label string
	var val int

	lines := bytes.Split(bytes.TrimSpace(data), []byte{'\n'})
	for _, line := range lines {
		items := bytes.Fields(line)
		if len(items) < 3 {
			continue
		}
		label = string(bytes.TrimSuffix(items[0], []byte{':'}))
		val, err = strconv.Atoi(string(items[1]))
		if err == nil {
			meminfo[label] = val
		}
	}
	return nil
}

func (input *SysinfoInput) setMeminfoMessage(pack *pipeline.PipelinePack, info map[string]int) {
	pack.Message.SetUuid(uuid.NewRandom())
	pack.Message.SetTimestamp(time.Now().UnixNano())
	pack.Message.SetType("heka.sysinfo.meminfo")
	for name, value := range info {
		input.AddField(pack, name, value, "kB")
	}
}
