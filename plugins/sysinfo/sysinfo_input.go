package sysinfo

import (
	"bytes"
	"code.google.com/p/go-uuid/uuid"
	"fmt"
	"github.com/mozilla-services/heka/message"
	"github.com/mozilla-services/heka/pipeline"
	"io/ioutil"
	"os"
	"strconv"
	"syscall"
	"time"
)

func init() {
	pipeline.RegisterPlugin("SysinfoInput", func() interface{} {
		return new(SysinfoInput)
	})
}

var ErrCantAddField = "Cant add field: %s"

type SysinfoInput struct {
	SysinfoInputConfig
	stop   chan bool
	runner pipeline.InputRunner
}

type SysinfoInputConfig struct {
	TickerInterval uint   `toml:"ticker_interval"`
	DecoderName    string `toml:"decoder"`
}

func (input *SysinfoInput) ConfigStruct() interface{} {
	return &SysinfoInputConfig{
		TickerInterval: uint(5),
	}
}

func (input *SysinfoInput) Init(config interface{}) error {
	// conf := config.(SysinfoInputConfig)
	input.stop = make(chan bool)
	return nil
}

func (input *SysinfoInput) Stop() {
	close(input.stop)
}

func (input *SysinfoInput) Run(runner pipeline.InputRunner,
	helper pipeline.PluginHelper) error {

	var (
		info syscall.Sysinfo_t
		pack *pipeline.PipelinePack
	)

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
		err := syscall.Sysinfo(&info)
		if err != nil {
			return err
		}
		pack = <-packSupply
		pack.Message.SetHostname(hostname)
		input.setSysinfoMessage(pack, &info)
		runner.Inject(pack)

		err = Meminfo(meminfo)
		if err != nil {
			return err
		}
		pack = <-packSupply
		pack.Message.SetHostname(hostname)
		input.setMeminfoMessage(pack, meminfo)
		runner.Inject(pack)

	}
	return nil
}

func (input *SysinfoInput) AddField(pack *pipeline.PipelinePack, name string, value interface{}, representation string) {
	if field, err := message.NewField(name, value, representation); err == nil {
		pack.Message.AddField(field)
	} else {
		input.runner.LogError(fmt.Errorf(ErrCantAddField, err))
	}
}

func (input *SysinfoInput) setSysinfoMessage(pack *pipeline.PipelinePack, info *syscall.Sysinfo_t) {
	pack.Message.SetUuid(uuid.NewRandom())
	pack.Message.SetTimestamp(time.Now().UnixNano())
	pack.Message.SetType("heka.sysinfo.sysinfo")
	// Cpu load avg
	input.AddField(pack, "OneMinLoadAvg", float64(info.Loads[0])/float64(1<<16), "")
	input.AddField(pack, "FiveMinLoadAvg", float64(info.Loads[1])/float64(1<<16), "")
	input.AddField(pack, "FifteenMinLoadAvg", float64(info.Loads[2])/float64(1<<16), "")
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
	f, err := os.Open("/proc/meminfo")
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
		label = string(items[0])
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
