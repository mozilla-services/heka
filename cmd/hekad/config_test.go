/***** BEGIN LICENSE BLOCK *****
# This Source Code Form is subject to the terms of the Mozilla Public
# License, v. 2.0. If a copy of the MPL was not distributed with this file,
# You can obtain one at http://mozilla.org/MPL/2.0/.
#
# The Initial Developer of the Original Code is the Mozilla Foundation.
# Portions created by the Initial Developer are Copyright (C) 2012-2014
# the Initial Developer. All Rights Reserved.
#
# Contributor(s):
#   Victor Ng (vng@mozilla.com)
#   Rob Miller (rmiller@mozilla.com)
#
# ***** END LICENSE BLOCK *****/

package main

import (
	"github.com/mozilla-services/heka/pipeline"
	"github.com/mozilla-services/heka/plugins"
	"testing"
)

func TestDecode(t *testing.T) {
	_, err := LoadHekadConfig("../../pipeline/testsupport/sample-config.toml")
	if err != nil {
		t.Fatal(err)
	}
}

func TestCustomHostname(t *testing.T) {
	expected := "my.example.com"
	configPath := "../../pipeline/testsupport/sample-hostname.toml"
	config, err := LoadHekadConfig(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if config.Hostname != expected {
		t.Fatalf("HekadConfig.Hostname expected: '%s', Got: %s", expected, config.Hostname)
	}
	globals, _, _ := setGlobalConfigs(config)
	if globals.Hostname != expected {
		t.Fatalf("globals.Hostname expected: '%s', Got: %s", expected, globals.Hostname)
	}
	pConfig := pipeline.NewPipelineConfig(globals)
	if pConfig.Hostname() != expected {
		t.Fatalf("PipelineConfig.Hostname expected: '%s', Got: %s", expected, pConfig.Hostname())
	}
	err = loadFullConfig(pConfig, &configPath)
	if err != nil {
		t.Fatalf("Error loading full config: %s", err.Error())
	}
}

func TestLoadDir(t *testing.T) {
	origAvailablePlugins := make(map[string]func() interface{})
	for k, v := range pipeline.AvailablePlugins {
		origAvailablePlugins[k] = v
	}

	defer func() {
		pipeline.AvailablePlugins = origAvailablePlugins
	}()

	pipeConfig := pipeline.NewPipelineConfig(nil)
	confDirPath := "../../plugins/testsupport/config_dir"
	err := loadFullConfig(pipeConfig, &confDirPath)
	if err != nil {
		t.Fatal(err)
	}

	// verify the inputs sections load properly with a custom name
	udp, ok := pipeConfig.InputRunners["UdpInput"]
	if !ok {
		t.Fatal("No UdpInput configured.")
	}
	defer udp.Input().Stop()

	// and the decoders sections load
	_, ok = pipeConfig.DecoderMakers["ProtobufDecoder"]
	if !ok {
		t.Fatal("No ProtobufDecoder configured.")
	}

	// and the outputs sections load
	_, ok = pipeConfig.OutputRunners["LogOutput"]
	if !ok {
		t.Fatal("No LogOutput configured")
	}

	// and the filters sections load
	_, ok = pipeConfig.FilterRunners["sample"]
	if !ok {
		t.Fatal("No `sample` filter configured.")
	}

	// and the encoders sections load
	encoder, ok := pipeConfig.Encoder("PayloadEncoder", "foo")
	if !ok {
		t.Fatal("No PayloadEncoder configured.")
	}
	_, ok = encoder.(*plugins.PayloadEncoder)
	if !ok {
		t.Fatal("PayloadEncoder isn't a PayloadEncoder")
	}

	// and that the non "*.toml" file did *not* load
	_, ok = pipeConfig.FilterRunners["not_loaded"]
	if ok {
		t.Fatal("`not_loaded` filter *was* loaded, shouldn't have been!")
	}
}
