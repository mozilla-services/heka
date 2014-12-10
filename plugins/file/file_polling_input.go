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
#   Chance Zibolski (chance.zibolski@gmail.com)
#
# ***** END LICENSE BLOCK *****/

package file

import (
	"code.google.com/p/go-uuid/uuid"
	"fmt"
	"github.com/mozilla-services/heka/message"
	"github.com/mozilla-services/heka/pipeline"
	"io/ioutil"
	"time"
)

type FilePollingInput struct {
	*FilePollingInputConfig
	stop   chan bool
	runner pipeline.InputRunner
}

type FilePollingInputConfig struct {
	TickerInterval uint   `toml:"ticker_interval"`
	FilePath       string `toml:"file_path"`
}

func (input *FilePollingInput) ConfigStruct() interface{} {
	return &FilePollingInputConfig{
		TickerInterval: uint(5),
	}
}

func (input *FilePollingInput) Init(config interface{}) error {
	conf := config.(*FilePollingInputConfig)
	input.FilePollingInputConfig = conf
	input.stop = make(chan bool)
	return nil
}

func (input *FilePollingInput) Stop() {
	close(input.stop)
}

func (input *FilePollingInput) Run(runner pipeline.InputRunner,
	helper pipeline.PluginHelper) error {

	var (
		data []byte
		pack *pipeline.PipelinePack
		err  error
	)

	input.runner = runner
	hostname := helper.PipelineConfig().Hostname()
	packSupply := runner.InChan()
	tickChan := runner.Ticker()

	for {
		select {
		case <-input.stop:
			return nil
		case <-tickChan:
		}

		data, err = ioutil.ReadFile(input.FilePath)
		if err != nil {
			runner.LogError(fmt.Errorf("Error reading file: %s", err))
			continue
		}

		pack = <-packSupply
		pack.Message.SetUuid(uuid.NewRandom())
		pack.Message.SetTimestamp(time.Now().UnixNano())
		pack.Message.SetType("heka.file.polling")
		pack.Message.SetHostname(hostname)
		pack.Message.SetPayload(string(data))
		if field, err := message.NewField("TickerInterval", int(input.TickerInterval), ""); err != nil {
			runner.LogError(err)
		} else {
			pack.Message.AddField(field)
		}
		if field, err := message.NewField("FilePath", input.FilePath, ""); err != nil {
			runner.LogError(err)
		} else {
			pack.Message.AddField(field)
		}
		runner.Deliver(pack)
	}

	return nil
}

func init() {
	pipeline.RegisterPlugin("FilePollingInput", func() interface{} {
		return new(FilePollingInput)
	})
}
