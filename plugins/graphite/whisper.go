/***** BEGIN LICENSE BLOCK *****
# This Source Code Form is subject to the terms of the Mozilla Public
# License, v. 2.0. If a copy of the MPL was not distributed with this file,
# You can obtain one at http://mozilla.org/MPL/2.0/.
#
# The Initial Developer of the Original Code is the Mozilla Foundation.
# Portions created by the Initial Developer are Copyright (C) 2012-2015
# the Initial Developer. All Rights Reserved.
#
# Contributor(s):
#   Rob Miller (rmiller@mozilla.com)
#
# ***** END LICENSE BLOCK *****/

package graphite

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	. "github.com/mozilla-services/heka/pipeline"
	"github.com/mozilla-services/heka/plugins"
	"github.com/rafrombrc/whisper-go/whisper"
)

// WhisperRunners listen for *whisper.Point data values to come in on an input
// channel and write the values out to a single whisper db file as they do.
type WhisperRunner interface {
	InChan() chan *whisper.Point
	Close() error
}

type wRunner struct {
	path   string
	db     *whisper.Whisper
	inChan chan *whisper.Point
	wg     *sync.WaitGroup
}

// Creates or opens the relevant whisper db file, and returns running
// WhisperRunner that will write to that file.
func NewWhisperRunner(path_ string, archiveInfo []whisper.ArchiveInfo,
	aggMethod whisper.AggregationMethod, folderPerm os.FileMode,
	wg *sync.WaitGroup) (wr WhisperRunner, err error) {

	var db *whisper.Whisper
	if db, err = whisper.Open(path_); err != nil {
		if !os.IsNotExist(err) {
			// A real error.
			err = fmt.Errorf("Error opening whisper db: %s", err)
			return
		}

		// First make sure the folder is there.
		dir := filepath.Dir(path_)
		if _, err = os.Stat(dir); os.IsNotExist(err) {
			if err = os.MkdirAll(dir, folderPerm); err != nil {
				err = fmt.Errorf("Error creating whisper db folder '%s': %s", dir, err)
				return
			}
		} else if err != nil {
			err = fmt.Errorf("Error opening whisper db folder '%s': %s", dir, err)
		}
		createOptions := whisper.CreateOptions{0.1, aggMethod, false}
		if db, err = whisper.Create(path_, archiveInfo, createOptions); err != nil {
			err = fmt.Errorf("Error creating whisper db: %s", err)
			return
		}
	}
	inChan := make(chan *whisper.Point, 10)
	realWr := &wRunner{path_, db, inChan, wg}
	realWr.start()
	wr = realWr
	return
}

func (wr *wRunner) start() {
	go func() {
		var err error
		for point := range wr.InChan() {
			if err = wr.db.Update(*point); err != nil {
				log.Printf("Error updating whisper db '%s': %s", wr.path, err)
			}
		}
		wr.wg.Done()
		if err = wr.Close(); err != nil {
			log.Printf("Error closing whisper db file: %s\n", err.Error())
		}
	}()
}

func (wr *wRunner) Close() error {
	return wr.db.Close()
}

func (wr *wRunner) InChan() chan *whisper.Point {
	return wr.inChan
}

// A WhisperOutput plugin will parse the stats data in the payload of a
// `statmetric` message and write the data out to a graphite-compatible
// whisper database file tree structure.
type WhisperOutput struct {
	basePath           string
	defaultAggMethod   whisper.AggregationMethod
	defaultArchiveInfo []whisper.ArchiveInfo
	dbs                map[string]WhisperRunner
	folderPerm         os.FileMode
	pConfig            *PipelineConfig
}

// WhisperOutput config struct.
type WhisperOutputConfig struct {
	// File path where the Whisper db files are stored. Can be an absolute
	// path, or relative to the Heka base directory. Defaults to
	// `$(BASEDIR)/whisper`.
	BasePath string `toml:"base_path"`

	// Default mechanism whisper will use to aggregate data points as they
	// roll from more precise (i.e. more recent) to less precise storage.
	DefaultAggMethod whisper.AggregationMethod `toml:"default_agg_method"`

	// Slice of 3-tuples, each 3-tuple describes a time interval's storage policy:
	// [<offset> <# of secs per datapoint> <# of datapoints>]
	DefaultArchiveInfo [][]uint32 `toml:"default_archive_info"`

	// Permissions to apply to directories created within the database file
	// tree. Must be a string representation of an octal integer. Defaults to
	// "700".
	FolderPerm string `toml:"folder_perm"`
}

func (o *WhisperOutput) ConfigStruct() interface{} {
	return &WhisperOutputConfig{
		BasePath:         "whisper",
		DefaultAggMethod: whisper.AggregationAverage,
		FolderPerm:       "700",
	}
}

// Heka will call this before calling any other methods to give us access to
// the pipeline configuration.
func (o *WhisperOutput) SetPipelineConfig(pConfig *PipelineConfig) {
	o.pConfig = pConfig
}

func (o *WhisperOutput) Init(config interface{}) (err error) {
	conf := config.(*WhisperOutputConfig)
	globals := o.pConfig.Globals
	o.basePath = globals.PrependBaseDir(conf.BasePath)
	o.defaultAggMethod = conf.DefaultAggMethod

	var intPerm int64
	if intPerm, err = strconv.ParseInt(conf.FolderPerm, 8, 32); err != nil {
		err = fmt.Errorf("WhisperOutput '%s' can't parse `folder_perm`, is it an octal integer string?", o.basePath)
		return
	}
	o.folderPerm = os.FileMode(intPerm)

	if err = os.MkdirAll(o.basePath, o.folderPerm); err != nil {
		return
	}

	if err = plugins.CheckWritePermission(o.basePath); err != nil {
		return
	}

	if conf.DefaultArchiveInfo == nil {
		// 60 seconds per datapoint, 1440 datapoints = 1 day of retention
		// 15 minutes per datapoint, 8 datapoints = 2 hours of retention
		// 1 hour per datapoint, 7 days of retention
		// 12 hours per datapoint, 2 years of retention
		conf.DefaultArchiveInfo = [][]uint32{
			{0, 60, 1440}, {0, 900, 8}, {0, 3600, 168}, {0, 43200, 1456},
		}
	}

	o.defaultArchiveInfo = make([]whisper.ArchiveInfo, len(conf.DefaultArchiveInfo))
	for i, aiSpec := range conf.DefaultArchiveInfo {
		if len(aiSpec) != 3 {
			err = fmt.Errorf("All default archive info settings must have 3 values.")
			return
		}
		o.defaultArchiveInfo[i] = whisper.ArchiveInfo{aiSpec[0], aiSpec[1], aiSpec[2]}
	}
	o.dbs = make(map[string]WhisperRunner)
	return
}

func (o *WhisperOutput) getFsPath(statName string) (statPath string) {
	statPath = strings.Replace(statName, ".", string(os.PathSeparator), -1)
	statPath = strings.Join([]string{statPath, "wsp"}, ".")
	statPath = filepath.Join(o.basePath, statPath)
	return
}

func (o *WhisperOutput) Run(or OutputRunner, h PluginHelper) (err error) {

	var (
		fields   []string
		wr       WhisperRunner
		unixTime uint64
		value    float64
		e        error
		pack     *PipelinePack
		wg       sync.WaitGroup
	)

	for pack = range or.InChan() {
		lines := strings.Split(strings.Trim(pack.Message.GetPayload(), " \n"), "\n")
		or.UpdateCursor(pack.QueueCursor)
		pack.Recycle(nil) // Once we've copied the payload we're done w/ the pack.
		for _, line := range lines {
			// `fields` should be "<name> <value> <timestamp>"
			fields = strings.Fields(line)
			if len(fields) != 3 {
				or.LogError(fmt.Errorf("malformed statmetric line: '%s'", line))
				continue
			}
			if wr = o.dbs[fields[0]]; wr == nil {
				wg.Add(1)
				wr, e = NewWhisperRunner(o.getFsPath(fields[0]), o.defaultArchiveInfo,
					o.defaultAggMethod, o.folderPerm, &wg)
				if e != nil {
					or.LogError(fmt.Errorf("can't create WhisperRunner: %s", e))
					continue
				}
				o.dbs[fields[0]] = wr
			}
			if unixTime, e = strconv.ParseUint(fields[2], 0, 32); e != nil {
				or.LogError(fmt.Errorf("parsing time: %s", e))
				continue
			}
			if value, e = strconv.ParseFloat(fields[1], 64); e != nil {
				or.LogError(fmt.Errorf("parsing value '%s': %s", fields[1], e))
				continue
			}
			pt := &whisper.Point{
				Timestamp: uint32(unixTime),
				Value:     value,
			}
			wr.InChan() <- pt
		}
	}

	for _, wr := range o.dbs {
		close(wr.InChan())
	}
	wg.Wait()

	return
}

func init() {
	RegisterPlugin("WhisperOutput", func() interface{} {
		return new(WhisperOutput)
	})
}
