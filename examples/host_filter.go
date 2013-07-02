/***** BEGIN LICENSE BLOCK *****
# This Source Code Form is subject to the terms of the Mozilla Public
# License, v. 2.0. If a copy of the MPL was not distributed with this file,
# You can obtain one at http://mozilla.org/MPL/2.0/.
#
# The Initial Developer of the Original Code is the Mozilla Foundation.
# Portions created by the Initial Developer are Copyright (C) 2012
# the Initial Developer. All Rights Reserved.
#
# Contributor(s):
#   Rob Miller (rmiller@mozilla.com)
#
# ***** END LICENSE BLOCK *****/

package examples

import (
	"errors"
	"fmt"
	"github.com/mozilla-services/heka/pipeline"
)

type HostFilter struct {
	hosts  map[string]bool
	output string
}

// Extract hosts value from config and store it on the plugin instance.
func (f *HostFilter) Init(config interface{}) error {
	var (
		hostsConf  interface{}
		hosts      []interface{}
		host       string
		outputConf interface{}
		ok         bool
	)
	conf := config.(pipeline.PluginConfig)
	if hostsConf, ok = conf["hosts"]; !ok {
		return errors.New("No 'hosts' setting specified.")
	}
	if hosts, ok = hostsConf.([]interface{}); !ok {
		return errors.New("'hosts' setting not a sequence.")
	}
	if outputConf, ok = conf["output"]; !ok {
		return errors.New("No 'output' setting specified.")
	}
	if f.output, ok = outputConf.(string); !ok {
		return errors.New("'output' setting not a string value.")
	}
	f.hosts = make(map[string]bool)
	for _, h := range hosts {
		if host, ok = h.(string); !ok {
			return errors.New("Non-string host value.")
		}
		f.hosts[host] = true
	}
	return nil
}

// Fetch correct output and iterate over received messages, checking against
// message hostname and delivering to the output if hostname is in our config.
func (f *HostFilter) Run(runner pipeline.FilterRunner, helper pipeline.PluginHelper) (
	err error) {

	var (
		hostname string
		output   pipeline.OutputRunner
		ok       bool
	)

	if output, ok = helper.Output(f.output); !ok {
		return fmt.Errorf("No output: %s", output)
	}
	for pack := range runner.InChan() {
		hostname = pack.Message.GetHostname()
		if f.hosts[hostname] {
			output.InChan() <- pack
		} else {
			pack.Recycle()
		}
	}
	return
}

func init() {
	pipeline.RegisterPlugin("HostFilter", func() interface{} {
		return new(HostFilter)
	})
}
