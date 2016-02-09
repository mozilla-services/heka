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
#
# ***** END LICENSE BLOCK *****/

package docker

import (
	"fmt"

	"github.com/mozilla-services/heka/pipeline"
)

type DockerLogInputConfig struct {
	// A Docker endpoint.
	Endpoint         string   `toml:"endpoint"`
	CertPath         string   `toml:"cert_path"`
	NameFromEnv      string   `toml:"name_from_env_var"`
	FieldsFromEnv    []string `toml:"fields_from_env"`
	FieldsFromLabels []string `toml:"fields_from_labels"`
}

type DockerLogInput struct {
	conf      *DockerLogInputConfig
	stopChan  chan error
	closer    chan struct{}
	attachMgr *AttachManager
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

	m, err := NewAttachManager(
		di.conf.Endpoint,
		di.conf.CertPath,
		di.conf.NameFromEnv,
		di.conf.FieldsFromEnv,
		di.conf.FieldsFromLabels,
	)
	if err != nil {
		return fmt.Errorf("DockerLogInput: failed to attach: %s", err.Error())
	}

	di.attachMgr = m
	return nil
}

func (di *DockerLogInput) Run(ir pipeline.InputRunner, h pipeline.PluginHelper) error {
	return di.attachMgr.Run(ir, h.Hostname(), di.stopChan)
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
