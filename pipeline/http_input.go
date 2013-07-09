package pipeline

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"time"
)

type HttpInput struct {
	dataChan    chan []byte
	stopChan    chan bool
	Monitor     *HttpInputMonitor
	decoderName string
}

type HttpInputConfig struct {
	Url            string
	TickerInterval uint `toml:"ticker_interval"`

	// Names of configured `LoglineDecoder` instances.
	DecoderName string
}

func (hi *HttpInput) ConfigStruct() interface{} {
	return &HttpInputConfig{
		TickerInterval: uint(10),
		DecoderName:    "JsonDecoder",
	}
}

func (hi *HttpInput) Init(config interface{}) error {
	conf := config.(*HttpInputConfig)

	hi.dataChan = make(chan []byte)
	hi.stopChan = make(chan bool)
	hi.decoderName = conf.DecoderName

	hi.Monitor = new(HttpInputMonitor)

	hi.Monitor.Init(conf.Url, hi.dataChan, hi.stopChan)
	return nil
}

func (hi *HttpInput) Run(ir InputRunner, h PluginHelper) (err error) {
	var (
		pack    *PipelinePack
		dRunner DecoderRunner
		e       error
		ok      bool
	)

	ir.LogMessage(fmt.Sprintf("[HttpInput (%s)] Running...",
		hi.Monitor.url))

	go hi.Monitor.Monitor(ir)

	hostname := h.PipelineConfig().hostname
	packSupply := ir.InChan()

	dSet := h.DecoderSet()
	if dRunner, ok = dSet.ByName(hi.decoderName); !ok {
		return fmt.Errorf("Decoder not found: %s", hi.decoderName)
	}
	decoder := dRunner.Decoder()

	for {
		select {
		case data := <-hi.dataChan:
			pack = <-packSupply
			copy(pack.MsgBytes, data)
			pack.Message.SetType("httpdata")
			pack.Message.SetHostname(hostname)

			if e = decoder.Decode(pack); e == nil {
				ir.Inject(pack)
			} else {
				ir.LogError(fmt.Errorf("Couldn't parse HTTP data: %s", string(data)))
				pack.Recycle()
			}
		}
	}

	return nil
}

func (hi *HttpInput) Stop() {
	close(hi.stopChan)
}

type HttpInputMonitor struct {
	url      string
	dataChan chan []byte
	stopChan chan bool

	ir       InputRunner
	tickChan <-chan time.Time
}

func (hm *HttpInputMonitor) Init(url string, dataChan chan []byte, stopChan chan bool) {
	hm.url = url
	hm.dataChan = dataChan
	hm.stopChan = stopChan
}

func (hm *HttpInputMonitor) Monitor(ir InputRunner) {
	ir.LogMessage("[HttpInputMonitor] Monitoring...")


	hm.ir = ir
	hm.tickChan = ir.Ticker()

	for {
		select {
		case <-hm.tickChan:
			// Fetch URL
			resp, err := http.Get(hm.url)
			if err != nil {
				ir.LogError(fmt.Errorf("[HttpInputMonitor] %s", err))
				return
			}
			defer resp.Body.Close()

			// Read content
			body, err := ioutil.ReadAll(resp.Body)

			if err != nil {
				ir.LogError(fmt.Errorf("[HttpInputMonitor] [%s]", err.Error()))
				continue
			}

			// Send it on the channel
			hm.dataChan <- body

		case <-hm.stopChan:
			ir.LogMessage(fmt.Sprintf("[HttpInputMonitor (%s)] Stop", hm.url))
			return
		}
	}
}

func (hm *HttpInputMonitor) Stop() {
	hm.stopChan <- true
}
