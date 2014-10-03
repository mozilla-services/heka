// +build dockerplugins

/***** BEGIN LICENSE BLOCK *****
# This Source Code Form is subject to the terms of the Mozilla Public
# License, v. 2.0. If a copy of the MPL was not distributed with this file,
# You can obtain one at http://mozilla.org/MPL/2.0/.
#
# The Initial Developer of the Original Code is the Mozilla Foundation.
# Portions created by the Initial Developer are Copyright (C) 2014
# the Initial Developer. All Rights Reserved.
#
# Contributor(s):
#   Anton Lindström (carlantonlindstrom@gmail.com)
#   Rob Miller (rmiller@mozilla.com)
#
# ***** END LICENSE BLOCK *****/

package docker

import (
	"code.google.com/p/go-uuid/uuid"
	"fmt"
	"github.com/fsouza/go-dockerclient"
	"github.com/mozilla-services/heka/message"
	"github.com/mozilla-services/heka/pipeline"
	"os"
	"time"
)

type DockerLogInputConfig struct {
	// A Docker endpoint
	Endpoint string `toml:"endpoint"`
	// Name of configured decoder to receive the input
	Decoder string `toml:"decoder"`
}

type DockerLogInput struct {
	conf         *DockerLogInputConfig
	stopChan     chan bool
	closer       chan bool
	logstream    chan *Log
	attachErrors chan *AttachError
}

func (di *DockerLogInput) ConfigStruct() interface{} {
	return &DockerLogInputConfig{
		Endpoint: "unix:///var/run/docker.sock",
	}
}

func (di *DockerLogInput) Init(config interface{}) error {
	di.conf = config.(*DockerLogInputConfig)
	di.stopChan = make(chan bool)
	di.closer = make(chan bool)
	di.logstream = make(chan *Log)
	di.attachErrors = make(chan *AttachError)

	client, err := docker.NewClient(di.conf.Endpoint)
	if err != nil {
		return fmt.Errorf("DockerLogInput: connecting to - %s", err.Error())
	}

	m, err := NewAttachManager(client, di.attachErrors)
	if err != nil {
		return fmt.Errorf("DockerLogInput: failed to attach: %s", err.Error())
	}
	go m.Listen(di.logstream, di.closer)

	return nil
}

func (di *DockerLogInput) Run(ir pipeline.InputRunner, h pipeline.PluginHelper) error {
	var (
		pack    *pipeline.PipelinePack
		dRunner pipeline.DecoderRunner
		ok      bool
	)

	hostname, err := os.Hostname()
	if err != nil {
		hostname = "unknown"
	}

	// Get the InputRunner's chan to receive empty PipelinePacks
	packSupply := ir.InChan()

	if di.conf.Decoder != "" {
		if dRunner, ok = h.DecoderRunner(di.conf.Decoder,
			fmt.Sprintf("%s-%s", ir.Name(), di.conf.Decoder)); !ok {
			return fmt.Errorf("Decoder not found: %s", di.conf.Decoder)
		}
	}

	for {
		select {
		case logline := <-di.logstream:
			pack = <-packSupply

			pack.Message.SetType("DockerLog")
			pack.Message.SetLogger(logline.Type) // stderr or stdout
			pack.Message.SetHostname(hostname)   // Use the host's hosntame
			pack.Message.SetPayload(logline.Data)
			pack.Message.SetTimestamp(time.Now().UnixNano())
			pack.Message.SetUuid(uuid.NewRandom())
			message.NewStringField(pack.Message, "ContainerID", logline.ID)
			message.NewStringField(pack.Message, "ContainerName", logline.Name)

			if dRunner == nil {
				ir.Inject(pack)
			} else {
				dRunner.InChan() <- pack
			}

		case attachError := <-di.attachErrors:
			ir.LogError(fmt.Errorf("Attacher error: %s", attachError.Error))

		case <-di.stopChan:
			di.closer <- true
			close(di.logstream)
			return nil
		}
	}
}

func (di *DockerLogInput) Stop() {
	close(di.stopChan)
}

func init() {
	pipeline.RegisterPlugin("DockerLogInput", func() interface{} {
		return new(DockerLogInput)
	})
}
