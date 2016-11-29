package graylog

import (
	"strings"

	"github.com/pborman/uuid"

	"github.com/mozilla-services/heka/pipeline"
	"github.com/mozilla-services/heka/message"

	"github.com/Graylog2/go-gelf/gelf"
)

type GraylogInputConfig struct{
	Address string `toml:"address"`
}

type GraylogInput struct {
	config *GraylogInputConfig
	reader *gelf.Reader

	ctrlMsgs chan gelfCtrl
	stopChan chan bool
}

func (g *GraylogInput) ConfigStruct() interface{} {
	return &GraylogInputConfig{
	}
}

func (g *GraylogInput) Init(config interface{}) (err error) {
	g.config = config.(*GraylogInputConfig)	
	g.ctrlMsgs = make(chan gelfCtrl)
	g.stopChan = make(chan bool)
	g.reader,err = gelf.NewReader(g.config.Address)
	if err != nil {
		return
	}

	return
}

type gelfCtrl struct {
	err error
	message *gelf.Message
}

func (g *GraylogInput) Run(ir pipeline.InputRunner, h pipeline.PluginHelper) (err error) {
	go func() {
		for {
			select {
			case <-g.stopChan:
				break
			default:
				message,err := g.reader.ReadMessage()
				g.ctrlMsgs <- gelfCtrl {
					err: err,
					message: message,
				}
				if err != nil {
					break
				}
			}
		}
		close(g.ctrlMsgs)

	}()

	MsgLoop:
	for ctrlMsg := range g.ctrlMsgs {
		if ctrlMsg.err != nil {
			ir.LogError(ctrlMsg.err)
			err = ctrlMsg.err
			break MsgLoop
		}

		msg := ctrlMsg.message

		pack := <-ir.InChan()
		if msg.Full != "" {
			pack.Message.SetPayload(msg.Full)
		} else {
			pack.Message.SetPayload(msg.Short)
		}

		pack.Message.SetUuid(uuid.NewRandom())
		pack.Message.SetTimestamp(int64(msg.TimeUnix) * 1000000000)
		pack.Message.SetType("heka.graylog")
		pack.Message.SetHostname(msg.Host)
		pack.Message.SetSeverity(msg.Level)
		pack.Message.SetLogger(g.config.Address)
		for k,v := range msg.Extra {
			cleanedK := cleanKeyForKibana(k)
			field,err := message.NewField(cleanedK, v, "")
			if err != nil {
				ir.LogError(err)
				break MsgLoop
			}
			pack.Message.AddField(field)
		}

		ir.Deliver(pack)
	}

	return
}

func cleanKeyForKibana(k string) (output string) {
	return strings.TrimPrefix(k, "_")
}

func (g *GraylogInput) Stop() {
	close(g.stopChan)
}

func init() {
	pipeline.RegisterPlugin("GraylogInput", func() interface{} {
		return new(GraylogInput)
	})
}