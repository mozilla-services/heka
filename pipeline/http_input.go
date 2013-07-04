package pipeline

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"time"
)

type HttpInput struct {
	dataChan chan []byte
	stopChan chan bool
	hm       *HttpMonitor
}

type HttpInputConfig struct {
	Url            string
	TickerInterval uint `toml:"ticker_interval"`
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

	hi.hm = new(HttpMonitor)

	hi.hm.Init(conf.Url, hi.dataChan, hi.stopChan)
	return nil
}

func (hi *HttpInput) Run(ir InputRunner, h PluginHelper) (err error) {
	ir.LogMessage(fmt.Sprintf("[HttpInput (%s)] Running...", hi.hm.url))

	go hi.hm.Monitor(ir)

	hostname := h.PipelineConfig().hostname
	packSupply := ir.InChan()

	for {
		select {
		case data := <-hi.dataChan:
			pack := <-packSupply
			copy(pack.MsgBytes, data)
			pack.Message.SetType("httpdata")
			pack.Message.SetHostname(hostname)

			// TODO: pull in the decoder code from LogfileInput
			ir.Inject(pack)
		}
	}

	return nil
}

func (hi *HttpInput) Stop() {
	close(hi.stopChan)
}

type HttpMonitor struct {
	url      string
	dataChan chan []byte
	stopChan chan bool

	ir       InputRunner
	tickChan <-chan time.Time
}

func (hm *HttpMonitor) Init(url string, dataChan chan []byte, stopChan chan bool) {
	hm.url = url
	hm.dataChan = dataChan
	hm.stopChan = stopChan

}

func (hm *HttpMonitor) Monitor(ir InputRunner) {
	ir.LogMessage("[HttpMonitor] Monitoring...")

	hm.ir = ir
	hm.tickChan = ir.Ticker()

	for {
		select {
		case <-hm.tickChan:
			// Fetch URL
			resp, err := http.Get(hm.url)
			defer resp.Body.Close()

			if err != nil {
				ir.LogError(fmt.Errorf("[HttpMonitor] %s", err))
				return
			}

			// Read content
			body, err := ioutil.ReadAll(resp.Body)

			if err != nil {
				ir.LogError(fmt.Errorf("[HttpMonitor] %s", err))
				return
			}

			// Send it on the channel
			hm.dataChan <- body

		case <-hm.stopChan:
			ir.LogMessage(fmt.Sprintf("[HttpMonitor (%s)] Stop", hm.url))
			return
		}
	}
}

func (hm *HttpMonitor) Stop() {
	hm.stopChan <- true
}
