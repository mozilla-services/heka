package pipeline

import (
    "net/http"
    "io/ioutil"
    "fmt"
)

type HttpInput struct {
    dataChan chan []byte
    stopChan chan bool
    hm HttpMonitor
}

type HttpInputConfig struct {
    url string
    interval int64
}

func (self *HttpInput) ConfigStruct() interface{} {
    return new(HttpInputConfig)
}

func (self *HttpInput) Init(conf interface{}) error {
    config := conf.(*HttpInputConfig)

    self.dataChan = make(chan []byte)
    self.stopChan = make(chan bool)

    self.hm = HttpMonitor{config.url, config.interval, self.dataChan}

    return nil
}

func (self *HttpInput) Run(ir InputRunner, h PluginHelper) (err error) {
    ir.LogMessage("[HttpInput] Running...")
    ir.LogMessage(fmt.Sprintf("%s (%d)", self.hm.url, self.hm.interval))

    go self.hm.Monitor(ir)

    packSupply := ir.InChan()

    for {
        select {
        case json := <-self.dataChan:
            pack := <-packSupply

            ir.LogMessage(fmt.Sprintf("[HttpInput] Received packet: %s", json))

            copy(pack.MsgBytes, json)

            ir.Inject(pack)

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

    ir.LogMessage(fmt.Sprintf("[HttpMonitor] GET %s", hm.url))

    resp, err := http.Get(hm.url)

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

    resp.Body.Close()
}