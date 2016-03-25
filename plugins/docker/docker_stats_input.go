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
#   Karl Matthias (karl.matthias@gonitro.com)
#   Guy Templeton (guy.templeton@skyscanner.net)
#
# ***** END LICENSE BLOCK *****/

package docker

import (
	"errors"
	"fmt"

	"github.com/mozilla-services/heka/message"
	"github.com/mozilla-services/heka/pipeline"
	"github.com/pborman/uuid"
)

type DockerStatsInputConfig struct {
	// A Docker endpoint.
	Endpoint         string   `toml:"endpoint"`
	CertPath         string   `toml:"cert_path"`
	NameFromEnv      string   `toml:"name_from_env_var"`
	FieldsFromEnv    []string `toml:"fields_from_env"`
	FieldsFromLabels []string `toml:"fields_from_labels"`
}

type DockerStatsInput struct {
	conf         *DockerStatsInputConfig
	stopChan     chan error
	closer       chan struct{}
	statsstream  chan *DockerStat
	attachErrors chan error
	statsMgr     *StatsManager
}

func (di *DockerStatsInput) ConfigStruct() interface{} {
	return &DockerStatsInputConfig{
		Endpoint: "unix:///var/run/docker.sock",
		CertPath: "",
	}
}

func (di *DockerStatsInput) Init(config interface{}) error {

	di.conf = config.(*DockerStatsInputConfig)
	di.stopChan = make(chan error)
	di.closer = make(chan struct{})
	di.statsstream = make(chan *DockerStat)
	di.attachErrors = make(chan error)

	m, err := NewStatsManager(di.conf.Endpoint, di.conf.CertPath, di.attachErrors,
		di.conf.NameFromEnv, di.conf.FieldsFromEnv, di.conf.FieldsFromLabels)
	if err != nil {
		return fmt.Errorf("DockerStatsInput: failed to attach: %s", err.Error())
	}

	di.statsMgr = m
	return nil
}

func (di *DockerStatsInput) Run(ir pipeline.InputRunner, h pipeline.PluginHelper) error {
	var (
		ok   bool
		pack *pipeline.PipelinePack
	)
	di.statsMgr.ir = ir
	hostname := h.Hostname()

	go di.statsMgr.Run(di.statsstream, di.closer, di.stopChan)

	err := withRetries(func() error {
		return di.statsMgr.client.AddEventListener(di.statsMgr.events)
	})
	// Get the InputRunner's chan to receive empty PipelinePacks
	packSupply := ir.InChan()

	ok = true
	for ok {
		select {
		case statsline := <-di.statsstream:
			pack = <-packSupply

			pack.Message.SetType("DockerStats")
			pack.Message.SetLogger(statsline.Container)
			pack.Message.SetHostname(hostname) // Use the host's hosntame
			pack.Message.SetPayload(statsline.StatsString)
			pack.Message.SetTimestamp(statsline.Time.UnixNano())
			pack.Message.SetUuid(uuid.NewRandom())

			for name, value := range statsline.Fields {
				field, err := message.NewField(name, value, "")
				if err != nil {
					ir.LogError(
						fmt.Errorf("can't add '%s' field: %s", name, err.Error()),
					)
					continue
				}

				pack.Message.AddField(field)
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
	return nil
}

func (di *DockerStatsInput) CleanupForRestart() {
	// Intentionally left empty. Cleanup happens in Run()
}

func (di *DockerStatsInput) Stop() {
	close(di.stopChan)
}

func init() {
	pipeline.RegisterPlugin("DockerStatsInput", func() interface{} {
		return new(DockerStatsInput)
	})
}
