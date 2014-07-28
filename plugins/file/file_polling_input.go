package file

import (
	"code.google.com/p/go-uuid/uuid"
	"fmt"
	"github.com/mozilla-services/heka/message"
	"github.com/mozilla-services/heka/pipeline"
	"io/ioutil"
	"time"
)

type FilePollingInput struct {
	*FilePollingInputConfig
	decoderChan chan *pipeline.PipelinePack
	stop        chan bool
	runner      pipeline.InputRunner
}

type FilePollingInputConfig struct {
	TickerInterval uint   `toml:"ticker_interval"`
	DecoderName    string `toml:"decoder"`
	FilePath       string `toml:"file_path"`
}

func (input *FilePollingInput) ConfigStruct() interface{} {
	return &FilePollingInputConfig{
		TickerInterval: uint(5),
	}
}

func (input *FilePollingInput) Init(config interface{}) error {
	conf := config.(*FilePollingInputConfig)
	input.FilePollingInputConfig = conf
	input.stop = make(chan bool)
	return nil
}

func (input *FilePollingInput) Stop() {
	close(input.stop)
}

func (input *FilePollingInput) Run(runner pipeline.InputRunner,
	helper pipeline.PluginHelper) error {

	var (
		data    []byte
		pack    *pipeline.PipelinePack
		dRunner pipeline.DecoderRunner
		ok      bool
		err     error
	)

	if input.DecoderName != "" {
		if dRunner, ok = helper.DecoderRunner(input.DecoderName,
			fmt.Sprintf("%s-%s", runner.Name(), input.DecoderName)); !ok {
			return fmt.Errorf("Decoder not found: %s", input.DecoderName)
		}
		input.decoderChan = dRunner.InChan()
	}
	input.runner = runner

	hostname := helper.PipelineConfig().Hostname()
	packSupply := runner.InChan()
	tickChan := runner.Ticker()

	for {
		select {
		case <-input.stop:
			return nil
		case <-tickChan:
		}

		data, err = ioutil.ReadFile(input.FilePath)
		if err != nil {
			runner.LogError(fmt.Errorf("Error reading file: %s", err))
			continue
		}

		pack = <-packSupply
		pack.Message.SetUuid(uuid.NewRandom())
		pack.Message.SetTimestamp(time.Now().UnixNano())
		pack.Message.SetType("heka.file.polling")
		pack.Message.SetHostname(hostname)
		pack.Message.SetPayload(string(data))
		if field, err := message.NewField("FilePath", input.FilePath, ""); err != nil {
			runner.LogError(err)
		} else {
			pack.Message.AddField(field)
		}
		input.sendPack(pack)
	}

	return nil
}

func (input *FilePollingInput) sendPack(pack *pipeline.PipelinePack) {
	if input.decoderChan != nil {
		input.decoderChan <- pack
	} else {
		input.runner.Inject(pack)
	}
}

func init() {
	pipeline.RegisterPlugin("FilePollingInput", func() interface{} {
		return new(FilePollingInput)
	})
}
