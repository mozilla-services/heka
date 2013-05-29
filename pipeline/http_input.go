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
	hm       HttpMonitor
}

type HttpInputConfig struct {
	Url      string
	Interval int64
}

func (hi *HttpInput) ConfigStruct() interface{} {
	return new(HttpInputConfig)
}

func (hi *HttpInput) Init(conf interface{}) error {
	config := conf.(*HttpInputConfig)

	hi.dataChan = make(chan []byte)
	hi.stopChan = make(chan bool)

	hi.hm = HttpMonitor{config.Url, config.Interval, hi.dataChan}

	return nil
}

func (hi *HttpInput) Run(ir InputRunner, h PluginHelper) (err error) {
	ir.LogMessage("[HttpInput] Running...")
	ir.LogMessage(fmt.Sprintf("%s (%d)", hi.hm.url, hi.hm.interval))

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
			pack.Message.SetPayload(data)
			pack.Decoded = false

			ir.Inject(pack)

		case <-hi.stopChan:
			hi.hm.Stop()
			ir.LogMessage("[HttpInput] Stop")
			return nil
		}
	}

	return nil
}

func (hi *HttpInput) Stop() {
	hi.stopChan <- true
}

type HttpMonitor struct {
	url      string
	interval int64
	dataChan chan []byte
	stopChan chan bool
}

func (hm *HttpMonitor) Init(url string, interval int64, dataChan chan []byte) {
	hm.url = url
	hm.interval = interval
	hm.dataChan = dataChan
	hm.stopChan = make(chan bool)
}

func (hm *HttpMonitor) Monitor(ir InputRunner) {
	ir.LogMessage("[HttpMonitor] Monitoring...")

	for {
		select {
		case <-time.After(hm.interval * time.Millisecond):
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

		case <-hm.stopChannel:
			ir.LogMessage("[HttpMonitor] Stop")
			return
		}
	}
}

func (hm *HttpMonitor) Stop() {
	hm.stopChan <- true
}
