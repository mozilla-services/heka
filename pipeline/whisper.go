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

package pipeline

import (
	"fmt"
	"github.com/kisielk/whisper-go/whisper"
	"log"
	"os"
	"path"
	"strconv"
	"strings"
)

// WhisperRunners listen for *whisper.Point data values to come in on an input
// channel and write the values out to the whisper db as they do.
type WhisperRunner struct {
	path             string
	db               *whisper.Whisper
	defaultAggMethod whisper.AggregationMethod
	InChan           chan *whisper.Point
}

func NewWhisperRunner(path string, aggMethod whisper.AggregationMethod) (wr *WhisperRunner,
	err error) {
	var db *whisper.Whisper
	if db, err = whisper.Open(path); err != nil {
		if !os.IsNotExist(err) {
			// A real error.
			err = fmt.Errorf("Error opening whisper db: %s", err)
			return
		} else if db, err = whisper.Create(path, nil, 0.1, aggMethod, false); err != nil {
			err = fmt.Errorf("Error creating whisper db: %s", err)
			return
		}
	}
	inChan := make(chan *whisper.Point, 10)
	wr = &WhisperRunner{path, db, aggMethod, inChan}
	wr.start()
	return
}

func (wr *WhisperRunner) start() {
	go func() {
		var err error
		for point := range wr.InChan {
			if err = wr.db.Update(*point); err != nil {
				log.Printf("Error updating whisper db '%s': %s", wr.path, err)
			}
		}
	}()
}

// A WhisperOutput plugin will parse the stats data in the payload of a
// `statmetric` message and write the data out to a graphite-compatible
// whisper database file tree structure.
type WhisperOutput struct {
	basePath         string
	defaultAggMethod whisper.AggregationMethod
	dbs              map[string]*WhisperRunner
}

type WhisperOutputConfig struct {
	// Full file path to where the Whisper db files are stored.
	BasePath         string
	DefaultAggMethod whisper.AggregationMethod
}

func (o *WhisperOutput) ConfigStruct() interface{} {
	basePath := path.Join("var", "run", "hekad", "whisper")
	return &WhisperOutputConfig{
		BasePath:         basePath,
		DefaultAggMethod: whisper.AGGREGATION_AVERAGE,
	}
}

func (o *WhisperOutput) Init(config interface{}) (err error) {
	conf := config.(*WhisperOutputConfig)
	o.basePath = conf.BasePath
	o.defaultAggMethod = conf.DefaultAggMethod
	o.dbs = make(map[string]*WhisperRunner)
	return
}

func (o *WhisperOutput) getFsPath(statName string) (statPath string) {
	statPath = strings.Replace(statName, ".", string(os.PathSeparator), -1)
	statPath = strings.Join([]string{statPath, "wsp"}, ".")
	statPath = path.Join(o.basePath, statPath)
	return
}

func (o *WhisperOutput) Deliver(pack *PipelinePack) {
	payload := pack.Message.GetPayload()
	var fields []string
	var wr *WhisperRunner
	var unixTime uint64
	var value float64
	var err error
	for _, line := range strings.Split(payload, "\n") {
		// `fields` should be "<name> <value> <timestamp>"
		fields = strings.Fields(line)
		if len(fields) != 3 || !strings.HasPrefix(fields[0], "stats") {
			log.Println("WhisperOutput malformed statmetric line: ", line)
			continue
		}
		if wr = o.dbs[fields[0]]; wr == nil {
			wr, err = NewWhisperRunner(o.getFsPath(fields[0]), o.defaultAggMethod)
			if err != nil {
				log.Println("WhisperOutput can't create WhisperRunner: ", err)
				continue
			}
			o.dbs[fields[0]] = wr
		}
		if unixTime, err = strconv.ParseUint(fields[2], 0, 32); err != nil {
			log.Println("WhisperOutput error parsing time: ", err)
			continue
		}
		if value, err = strconv.ParseFloat(fields[1], 64); err != nil {
			log.Println("WhisperOutput error parsing value: ", err)
		}
		pt := &whisper.Point{
			Timestamp: uint32(unixTime),
			Value:     value,
		}
		wr.InChan <- pt
	}
}
