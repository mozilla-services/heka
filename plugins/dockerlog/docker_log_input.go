// +build dockerlog

package dockerlog

import (
	"code.google.com/p/go-uuid/uuid"
	"fmt"
	"github.com/fsouza/go-dockerclient"
	"github.com/mozilla-services/heka/message"
	. "github.com/mozilla-services/heka/pipeline"
	"os"
	"time"
)

type DockerLogInputConfig struct {
	// A Docker endpoint
	Endpoint string `toml:"endpoint"`
	// Name of configured decoder to receive the input
	Decoder string
}

type DockerLogInput struct {
	client   *docker.Client
	conf     *DockerLogInputConfig
	stopChan chan bool
}

func (di *DockerLogInput) ConfigStruct() interface{} {
	return &DockerLogInputConfig{"unix:///var/run/docker.sock", ""}
}

func (di *DockerLogInput) Init(config interface{}) error {
	di.conf = config.(*DockerLogInputConfig)

	var err error
	di.client, err = docker.NewClient(di.conf.Endpoint)

	if err != nil {
		return fmt.Errorf("connecting to - %s", err.Error())
	}

	return nil
}

func (di *DockerLogInput) Run(ir InputRunner, h PluginHelper) error {
	var (
		pack    *PipelinePack
		dRunner DecoderRunner
		decoder Decoder
		ok      bool
		e       error
	)
	// Get the InputRunner's chan to receive empty PipelinePacks
	packSupply := ir.InChan()

	if di.conf.Decoder != "" {
		if dRunner, ok = h.DecoderRunner(di.conf.Decoder,
			fmt.Sprintf("%s-%s", ir.Name(), di.conf.Decoder)); !ok {
			return fmt.Errorf("Decoder not found: %s", di.conf.Decoder)
		}
		decoder = dRunner.Decoder()
	}

	di.stopChan = make(chan bool)
	logstream := make(chan *Log)
	defer close(logstream)

	closer := make(chan bool)
	go NewAttachManager(di.client).Listen(nil, logstream, closer)

	stopped := false

	hostname, err := os.Hostname()
	if err != nil {
		hostname = ""
	}

	for !stopped {
		e = nil
		select {
		case <-di.stopChan:
			stopped = true
		case logline := <-logstream:
			pack = <-packSupply

			pack.Message.SetType("DockerLog")
			pack.Message.SetLogger(logline.Type) // stderr or stdout
			pack.Message.SetHostname(hostname)   // Use the host's hosntame
			pack.Message.SetPayload(logline.Data)
			pack.Message.SetTimestamp(time.Now().UnixNano())
			pack.Message.SetUuid(uuid.NewRandom())
			message.NewStringField(pack.Message, "ContainerID", logline.ID)
			message.NewStringField(pack.Message, "ContainerName", logline.Name)

			var packs []*PipelinePack
			if decoder == nil {
				packs = []*PipelinePack{pack}
			} else {
				packs, e = decoder.Decode(pack)
			}
			if packs != nil {
				for _, p := range packs {
					ir.Inject(p)
				}
			} else {
				if e != nil {
					ir.LogError(fmt.Errorf("Couldn't parse DockerLog message: %s", logline.Data))
				}
				pack.Recycle()
			}
		}
	}

	closer <- true
	return nil
}

func (di *DockerLogInput) Stop() {
	close(di.stopChan)
}

func init() {
	RegisterPlugin("DockerLogInput", func() interface{} {
		return new(DockerLogInput)
	})
}
