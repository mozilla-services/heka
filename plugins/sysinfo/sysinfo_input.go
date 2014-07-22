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

func init() {
	pipeline.RegisterPlugin("SysinfoInput", func() interface{} {
		return new(SysinfoInput)
	})
}

// ErrCantAddField should be returned if a field cannot be added to a message
var ErrCantAddField = errors.New("Cant add field")

// SysinfoInput is the struct containing the plugins internal state and config
type SysinfoInput struct {
	*SysinfoInputConfig
	getSysinfo            func(*syscall.Sysinfo_t) error
	proc_meminfo_location string
	stop                  chan bool
	runner                pipeline.InputRunner
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
	input.getSysinfo = syscall.Sysinfo
	input.proc_meminfo_location = "/proc/meminfo"
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

	for {
		select {
		case <-input.stop:
			return nil
		case <-tickChan:
		}
		err := input.getSysinfo(&info)
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

		meminfo, err := input.getMeminfo()
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
	unit := info.Unit
	var repr string
	switch unit {
	case 1:
		repr = "B"
	case 1 << 10:
		repr = "KB"
	case 1 << 20:
		repr = "MB"
	case 1 << 30:
		repr = "GB"
	default:
		repr = "B"
	}
	// Cpu load avg
	input.AddField(pack, "OneMinLoadAvg", float64(info.Loads[0])/float64(loads_shift), "")
	input.AddField(pack, "FiveMinLoadAvg", float64(info.Loads[1])/float64(loads_shift), "")
	input.AddField(pack, "FifteenMinLoadAvg", float64(info.Loads[2])/float64(loads_shift), "")
	// Memory
	input.AddField(pack, "Totalram", int(info.Totalram), repr)
	input.AddField(pack, "Freeram", int(info.Freeram), repr)
	input.AddField(pack, "Sharedram", int(info.Sharedram), repr)
	input.AddField(pack, "Bufferram", int(info.Bufferram), repr)
	input.AddField(pack, "Totalswap", int(info.Totalswap), repr)
	input.AddField(pack, "Freeswap", int(info.Freeswap), repr)
	input.AddField(pack, "Processes", int(info.Procs), "")
}

type MemInfo struct {
	Label string
	Value int
	Unit  string
}

func (input *SysinfoInput) getMeminfo() ([]*MemInfo, error) {
	if input.proc_meminfo_location == "" {
		panic("proc_meminfo_location cannot be empty.")
	}
	f, err := os.Open(input.proc_meminfo_location)
	if err != nil {
		return nil, err
	}
	data, err := ioutil.ReadAll(f)
	lines := bytes.Split(bytes.TrimSpace(data), []byte{'\n'})

	meminfo := make([]*MemInfo, 0)
	for _, line := range lines {
		items := bytes.Fields(line)
		val, err := strconv.Atoi(string(items[1]))
		if err != nil {
			continue
		}
		label := string(bytes.TrimSuffix(items[0], []byte{':'}))
		var unit string
		if len(items) > 2 {
			unit = string(items[2])
		}
		meminfo = append(meminfo, &MemInfo{Label: label, Value: val, Unit: unit})
	}
	return meminfo, nil
}

func (input *SysinfoInput) setMeminfoMessage(pack *pipeline.PipelinePack, info []*MemInfo) {
	pack.Message.SetUuid(uuid.NewRandom())
	pack.Message.SetTimestamp(time.Now().UnixNano())
	pack.Message.SetType("heka.sysinfo.meminfo")
	for _, item := range info {
		input.AddField(pack, item.Label, item.Value, item.Unit)
	}
}
