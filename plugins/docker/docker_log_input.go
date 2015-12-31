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
	"errors"
	"fmt"
	"time"

	"github.com/mozilla-services/heka/message"
	"github.com/mozilla-services/heka/pipeline"
	"github.com/pborman/uuid"
)

type DockerLogInputConfig struct {
	// A Docker endpoint.
	Endpoint      string   `toml:"endpoint"`
	CertPath      string   `toml:"cert_path"`
	NameFromEnv   string   `toml:"name_from_env_var"`
	FieldsFromEnv []string `toml:"fields_from_env"`
}

type DockerLogInput struct {
	conf         *DockerLogInputConfig
	stopChan     chan error
	closer       chan struct{}
	logstream    chan *Log
	attachErrors chan error
	attachMgr    *AttachManager
}

func (di *DockerLogInput) ConfigStruct() interface{} {
	return &DockerLogInputConfig{
		Endpoint: "unix:///var/run/docker.sock",
		CertPath: "",
	}
}

func (di *DockerLogInput) Init(config interface{}) error {
	di.conf = config.(*DockerLogInputConfig)
	di.stopChan = make(chan error)
	di.closer = make(chan struct{})
	di.logstream = make(chan *Log)
	di.attachErrors = make(chan error)

	m, err := NewAttachManager(di.conf.Endpoint, di.conf.CertPath, di.attachErrors, di.conf.NameFromEnv, di.conf.FieldsFromEnv)
	if err != nil {
		return fmt.Errorf("DockerLogInput: failed to attach: %s", err.Error())
	}

	di.attachMgr = m
	return nil
}

func (di *DockerLogInput) Run(ir pipeline.InputRunner, h pipeline.PluginHelper) error {
	var (
		pack *pipeline.PipelinePack
		ok   bool
	)

	hostname := h.Hostname()

	go di.attachMgr.Listen(di.logstream, di.closer)

	// Get the InputRunner's chan to receive empty PipelinePacks
	packSupply := ir.InChan()

	ok = true
	var err error
	for ok {
		select {
		case logline := <-di.logstream:
			pack = <-packSupply

			pack.Message.SetType("DockerLog")
			pack.Message.SetLogger(logline.Type) // stderr or stdout
			pack.Message.SetHostname(hostname)   // Use the host's hosntame
			pack.Message.SetPayload(logline.Data)
			pack.Message.SetTimestamp(time.Now().UnixNano())
			pack.Message.SetUuid(uuid.NewRandom())
			for k, v := range logline.Fields {
				message.NewStringField(pack.Message, k, v)
			}

			ir.Deliver(pack)

		case err, ok = <-di.attachErrors:
			if !ok {
				err = errors.New("Docker event channel closed")
				break
			}
			ir.LogError(fmt.Errorf("Attacher error: %s", err))

		case err = <-di.stopChan:
			ok = false
		}
	}

	di.closer <- struct{}{}
	close(di.logstream)
	return err
}

func (di *DockerLogInput) CleanupForRestart() {
	// Intentially left empty. Cleanup happens in Run()
}

func (di *DockerLogInput) Stop() {
	close(di.stopChan)
}

func init() {
	pipeline.RegisterPlugin("DockerLogInput", func() interface{} {
		return new(DockerLogInput)
	})
}
