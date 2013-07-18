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
	TickerInterval uint   `toml:"ticker_interval"`
	DecoderName    string `toml:"decoder"`
}

func (hi *HttpInput) ConfigStruct() interface{} {
	return &HttpInputConfig{
		TickerInterval: uint(10),
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
		pack                *PipelinePack
		dRunner             DecoderRunner
		ok                  bool
		router_shortcircuit bool
	)

	ir.LogMessage(fmt.Sprintf("[HttpInput (%s)] Running...",
		hi.Monitor.url))

	go hi.Monitor.Monitor(ir)

	pConfig := h.PipelineConfig()
	hostname := pConfig.hostname
	packSupply := ir.InChan()

	dSet := h.DecoderSet()
	if hi.decoderName == "" {
		router_shortcircuit = true
	} else if dRunner, ok = dSet.ByName(hi.decoderName); !ok {
		return fmt.Errorf("Decoder not found: %s", hi.decoderName)
	}

	for {
		select {
		case data := <-hi.dataChan:
			pack = <-packSupply
			pack.Message.SetType("heka.httpdata")
			pack.Message.SetHostname(hostname)
			pack.Message.SetPayload(string(data))
			if router_shortcircuit {
				pConfig.router.InChan() <- pack
			} else {
				dRunner.InChan() <- pack
			}
		case <-hi.stopChan:
			return
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
				continue
			}

			// Read content
			body, err := ioutil.ReadAll(resp.Body)
			if err != nil {
				ir.LogError(fmt.Errorf("[HttpInputMonitor] [%s]", err.Error()))
				continue
			}
			resp.Body.Close()

			// Send it on the channel
			hm.dataChan <- body

		case <-hm.stopChan:
			ir.LogMessage(fmt.Sprintf("[HttpInputMonitor (%s)] Stop", hm.url))
			return
		}
	}
}
