package pipeline

import (
    "net/http"
    "io/ioutil"
    "time"
)

type HttpInput struct {
    dataChan chan []byte
    hm HttpMonitor
    loop bool
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
    self.hm = HttpMonitor{config.url, config.interval, self.dataChan}

    self.loop = true

    return nil
}

func (self *HttpInput) Run(ir InputRunner, h PluginHelper) (err error) {
    go self.hm.Monitor()

    packSupply := ir.InChan()

    for self.loop {
        json := <-self.dataChan
        pack := <-packSupply

        copy(pack.MsgBytes, json)

        ir.Inject(pack)
    }

    return nil
}

func (self *HttpInput) Stop() {
    self.loop = false
}

type HttpMonitor struct {
    url string
    interval int64
    dataChan chan []byte
}

func (hm *HttpMonitor) Init(url string, interval int64, dataChan chan string) {
    hm.url = url
    hm.interval = interval
    hm.dataChan = dataChan
}

func (hm *HttpMonitor) Monitor() {
    for {
        time.Sleep(time.Duration(hm.interval) * time.Millisecond)

        resp, err := http.Get(hm.url)

        if err != nil {            
            continue
        }

        body, err := ioutil.ReadAll(resp.Body)

        if err != nil {
            continue
        }

        hm.dataChan <- body
    }
}