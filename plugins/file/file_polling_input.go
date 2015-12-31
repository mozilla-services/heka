/***** BEGIN LICENSE BLOCK *****
# This Source Code Form is subject to the terms of the Mozilla Public
# License, v. 2.0. If a copy of the MPL was not distributed with this file,
# You can obtain one at http://mozilla.org/MPL/2.0/.
#
# The Initial Developer of the Original Code is the Mozilla Foundation.
# Portions created by the Initial Developer are Copyright (C) 2014-2015
# the Initial Developer. All Rights Reserved.
#
# Contributor(s):
#   Chance Zibolski (chance.zibolski@gmail.com)
#   Rob Miller (rmiller@mozilla.com)
#
# ***** END LICENSE BLOCK *****/

package file

import (
	"fmt"
	"io"
	"os"

	"github.com/mozilla-services/heka/message"
	"github.com/mozilla-services/heka/pipeline"
)

type FilePollingInput struct {
	*FilePollingInputConfig
	stop     chan bool
	runner   pipeline.InputRunner
	hostname string
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

func (input *FilePollingInput) packDecorator(pack *pipeline.PipelinePack) {
	pack.Message.SetType("heka.file.polling")
	pack.Message.SetHostname(input.hostname)

	field, err := message.NewField("TickerInterval", int(input.TickerInterval), "")
	if err != nil {
		input.runner.LogError(
			fmt.Errorf("can't add 'TickerInterval' field: %s", err.Error()))
	} else {
		pack.Message.AddField(field)
	}
	field, err = message.NewField("FilePath", input.FilePath, "")
	if err != nil {
		input.runner.LogError(
			fmt.Errorf("can't add 'FilePath' field: %s", err.Error()))
	} else {
		pack.Message.AddField(field)
	}
}

func (input *FilePollingInput) Run(runner pipeline.InputRunner,
	helper pipeline.PluginHelper) error {

	input.runner = runner
	input.hostname = helper.PipelineConfig().Hostname()
	tickChan := runner.Ticker()
	sRunner := runner.NewSplitterRunner("")
	if !sRunner.UseMsgBytes() {
		sRunner.SetPackDecorator(input.packDecorator)
	}
	defer sRunner.Done()

	for {
		select {
		case <-input.stop:
			return nil
		case <-tickChan:
		}

		f, err := os.Open(input.FilePath)
		if err != nil {
			runner.LogError(fmt.Errorf("Error opening file: %s", err.Error()))
			continue
		}
		for err == nil {
			err = sRunner.SplitStream(f, nil)
			if err != io.EOF && err != nil {
				runner.LogError(fmt.Errorf("Error reading file: %s", err.Error()))
			}
		}
	}
}

func init() {
	pipeline.RegisterPlugin("FilePollingInput", func() interface{} {
		return new(FilePollingInput)
	})
}
