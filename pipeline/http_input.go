package pipeline

import (
    "net/http"
    "io/ioutil"
    "time"
    "fmt"
)

type HttpInput struct {
    dataChan chan []byte
    stopChan chan bool
    hm HttpMonitor

    ir InputRunner
    h PluginHelper
    packSupply chan *PipelinePack
}

type HttpInputConfig struct {
    Url string `toml:"url"`
    Interval int64 `tom:"interval"`
}

func (self *HttpInput) ConfigStruct() interface{} {
    return new(HttpInputConfig)
}

func (self *HttpInput) Init(conf interface{}) error {
    config := conf.(*HttpInputConfig)

    self.dataChan = make(chan []byte)
    self.stopChan = make(chan bool)

    self.hm = HttpMonitor{config.Url, config.Interval, self.dataChan}

    return nil
}

func (self *HttpInput) Run(ir InputRunner, h PluginHelper) (err error) {
    self.ir = ir
    self.h = h

    ir.LogMessage("[HttpInput] Running...")
    ir.LogMessage(fmt.Sprintf("%s (%d)", self.hm.url, self.hm.interval))

    go self.hm.Monitor(self.ir)

    self.packSupply = self.ir.InChan()

    for {
        select {
        case data := <-self.dataChan:
            go self.handleMessage(string(data))

        case <-self.stopChan:
            ir.LogMessage("[HttpInput] Stop")
            return
        }
    }

    return nil
}

func (self *HttpInput) Stop() {
    self.stopChan <- true
}

func (self *HttpInput) handleMessage(data string) {
    self.ir.LogMessage(fmt.Sprintf("[HttpInput] Received packet: %s", data))

    pack := <-self.packSupply
    copy(pack.MsgBytes, data)
    pack.Message.SetType("httpdata")
    pack.Message.SetHostname(self.h.PipelineConfig().hostname)
    pack.Message.SetPayload(data)
    pack.Decoded = false

    self.ir.Inject(pack)
}

type HttpMonitor struct {
    url string
    interval int64
    dataChan chan []byte
}

func (hm *HttpMonitor) Init(url string, interval int64, dataChan chan []byte) {
    hm.url = url
    hm.interval = interval
    hm.dataChan = dataChan
}

func (hm *HttpMonitor) Monitor(ir InputRunner) {
    ir.LogMessage("[HttpMonitor] Monitoring...")

    for {
        func() {
            defer time.Sleep(time.Duration(hm.interval) * time.Millisecond)

            ir.LogMessage(fmt.Sprintf("[HttpMonitor] GET %s", hm.url))

            resp, err := http.Get(hm.url)
            defer resp.Body.Close()

            if err != nil {    
                ir.LogError(fmt.Errorf("[HttpMonitor] %s", err))        
                return
            }

            ir.LogMessage("[HttpMonitor] Reading...")
            body, err := ioutil.ReadAll(resp.Body)

            if err != nil {
                ir.LogError(fmt.Errorf("[HttpMonitor] %s", err))   
                return
            }        

            ir.LogMessage("[HttpMonitor] Sending...")

            hm.dataChan <- body
        }()
    }
}