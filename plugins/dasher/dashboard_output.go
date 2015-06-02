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
#   Mike Trinkala (trink@mozilla.com)
#   Rob Miller (rmiller@mozilla.com)
#
# ***** END LICENSE BLOCK *****/

package dasher

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sync"
	"time"

	"github.com/mozilla-services/heka/message"
	. "github.com/mozilla-services/heka/pipeline"
	httpPlugin "github.com/mozilla-services/heka/plugins/http"
)

type DashboardOutputConfig struct {
	// IP address of the Dashboard HTTP interface (defaults to all interfaces on
	// port 4352 (HEKA))
	Address string `toml:"address"`
	// Directory where the static dashboard content is stored. Relative paths
	// will be evaluated relative to the Heka base dir. Defaults to
	// "/usr/share/heka/dasher".
	StaticDirectory string `toml:"static_directory"`
	// Working directory where the Dashboard output is written to; it also
	// serves as the root for the HTTP fileserver.  This directory is created
	// if necessary and if it exists the previous output is wiped clean. *DO
	// NOT* store any user created content here. Relative paths will be
	// evaluated relative to the Heka base dir. Defaults to "dashboard" (i.e.
	// "$(BASE_DIR)/dashboard").
	WorkingDirectory string `toml:"working_directory"`
	// Default interval at which dashboard will update is 5 seconds.
	TickerInterval uint `toml:"ticker_interval"`
	// Default message matcher.
	MessageMatcher string
	// Custom http headers
	Headers http.Header
}

func (self *DashboardOutput) ConfigStruct() interface{} {
	return &DashboardOutputConfig{
		Address:          ":4352",
		StaticDirectory:  "dasher",
		WorkingDirectory: "dashboard",
		TickerInterval:   uint(5),
		MessageMatcher:   "Type == 'heka.all-report' || Type == 'heka.sandbox-terminated' || Type == 'heka.sandbox-output'",
	}
}

type DashboardOutput struct {
	staticDirectory  string
	workingDirectory string
	relDataPath      string
	dataDirectory    string
	server           *http.Server
	handler          http.Handler
	pConfig          *PipelineConfig
	starterFunc      func(output *DashboardOutput) error
}

// Heka will call this before calling any other methods to give us access to
// the pipeline configuration.
func (self *DashboardOutput) SetPipelineConfig(pConfig *PipelineConfig) {
	self.pConfig = pConfig
}

func (self *DashboardOutput) Init(config interface{}) (err error) {
	conf := config.(*DashboardOutputConfig)

	globals := self.pConfig.Globals
	self.staticDirectory = globals.PrependShareDir(conf.StaticDirectory)
	self.workingDirectory = globals.PrependBaseDir(conf.WorkingDirectory)
	self.relDataPath = "data"
	self.dataDirectory = filepath.Join(self.workingDirectory, self.relDataPath)

	if self.starterFunc == nil {
		self.starterFunc = defaultStarter
	}

	if err = os.MkdirAll(self.dataDirectory, 0700); err != nil {
		return fmt.Errorf("DashboardOutput: Can't create working directory: %s", err)
	}

	// Delete all previous output.
	if matches, err := filepath.Glob(filepath.Join(self.workingDirectory, "*.*")); err == nil {
		for _, fn := range matches {
			os.Remove(fn)
		}
	}

	// Copy the static content from the static dir to the working directory.
	// This function does the copying, will be passed in to filepath.Walk.
	copier := func(path string, info os.FileInfo, err error) (e error) {
		if err != nil {
			return err
		}

		var (
			relPath, destPath string
			inFile, outFile   *os.File
			fi                os.FileInfo
		)
		if relPath, e = filepath.Rel(self.staticDirectory, path); e != nil {
			return
		}
		destPath = filepath.Join(self.workingDirectory, relPath)

		if fi, e = os.Stat(path); e != nil {
			return fmt.Errorf("can't stat '%s': %s", path, e)
		}

		// Is this a directory?
		if fi.IsDir() {
			// Yes, create it in the destination spot.
			if e = os.MkdirAll(destPath, 0700); e != nil {
				return fmt.Errorf("can't create folder '%s': %s", destPath, e)
			}
		} else {
			// Not a directory, create the new file.
			if outFile, e = os.Create(destPath); e != nil {
				return fmt.Errorf("can't create destination file '%s': %s", destPath, e)
			}
			if inFile, e = os.Open(path); e != nil {
				return fmt.Errorf("can't open '%s': %s", path, e)
			}
			if _, e = io.Copy(outFile, inFile); e != nil {
				return fmt.Errorf("can't copy to '%s': %s", destPath, e)
			}
		}
		return
	}

	if self.handler == nil {
		if err = filepath.Walk(self.staticDirectory, copier); err != nil {
			return fmt.Errorf("Error copying static dashboard files: %s", err)
		}
		self.handler = http.FileServer(http.Dir(self.workingDirectory))
	}
	self.server = &http.Server{
		Addr:         conf.Address,
		Handler:      httpPlugin.CustomHeadersHandler(self.handler, conf.Headers),
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	return
}

func (self *DashboardOutput) Run(or OutputRunner, h PluginHelper) (err error) {
	inChan := or.InChan()
	ticker := or.Ticker()
	go self.starterFunc(self)

	var (
		ok   = true
		pack *PipelinePack
		msg  *message.Message
	)

	// Maps sandbox names to plugin list items used to generate the
	// sandboxes.json file.
	sandboxes := make(map[string]*DashPluginListItem)
	sbxsLock := new(sync.Mutex)
	reNotWord, _ := regexp.Compile("\\W")
	for ok {
		select {
		case pack, ok = <-inChan:
			if !ok {
				break
			}
			msg = pack.Message
			switch msg.GetType() {
			case "heka.all-report":
				fn := filepath.Join(self.dataDirectory, "heka_report.json")
				overwriteFile(fn, msg.GetPayload())
				sbxsLock.Lock()
				if err := overwritePluginListFile(self.dataDirectory, sandboxes); err != nil {
					or.LogError(fmt.Errorf("Can't write plugin list file to '%s': %s",
						self.dataDirectory, err))
				}
				sbxsLock.Unlock()
			case "heka.sandbox-output":
				tmp, _ := msg.GetFieldValue("payload_type")
				if payloadType, ok := tmp.(string); ok {
					var payloadName, nameExt string
					tmp, _ := msg.GetFieldValue("payload_name")
					if payloadName, ok = tmp.(string); ok {
						nameExt = reNotWord.ReplaceAllString(payloadName, "")
					}
					if len(nameExt) > 64 {
						nameExt = nameExt[:64]
					}
					nameExt = "." + nameExt

					payloadType = reNotWord.ReplaceAllString(payloadType, "")
					filterName := msg.GetLogger()
					fn := filterName + nameExt + "." + payloadType
					ofn := filepath.Join(self.dataDirectory, fn)
					relPath := path.Join(self.relDataPath, fn) // Used for generating HTTP URLs.
					overwriteFile(ofn, msg.GetPayload())
					sbxsLock.Lock()
					if listItem, ok := sandboxes[filterName]; !ok {
						// First time we've seen this sandbox, add it to the set.
						output := &DashPluginOutput{
							Name:     payloadName,
							Filename: relPath,
						}
						sandboxes[filterName] = &DashPluginListItem{
							Name:    filterName,
							Outputs: []*DashPluginOutput{output},
						}
					} else {
						// We've seen the sandbox, see if we already have this output.
						found := false
						for _, output := range listItem.Outputs {
							if output.Name == payloadName {
								found = true
								break
							}
						}
						if !found {
							output := &DashPluginOutput{
								Name:     payloadName,
								Filename: relPath,
							}
							listItem.Outputs = append(listItem.Outputs, output)
						}
					}
					sbxsLock.Unlock()
				}
			case "heka.sandbox-terminated":
				var filterName string
				tmp, _ := msg.GetFieldValue("plugin")
				if s, ok := tmp.(string); ok {
					filterName = s
				} else {
					break
				}
				fn := filepath.Join(self.dataDirectory, "heka_sandbox_termination.tsv")
				if file, err := os.OpenFile(fn, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0644); err == nil {
					var line string
					if _, ok := msg.GetFieldValue("ProcessMessageCount"); !ok {
						line = fmt.Sprintf("%d\t%s\t%v\n", msg.GetTimestamp()/1e9,
							filterName, msg.GetPayload())
					} else {
						pmc, _ := msg.GetFieldValue("ProcessMessageCount")
						pms, _ := msg.GetFieldValue("ProcessMessageSamples")
						pmd, _ := msg.GetFieldValue("ProcessMessageAvgDuration")
						mad, _ := msg.GetFieldValue("MatchAvgDuration")
						fcl, _ := msg.GetFieldValue("FilterChanLength")
						mcl, _ := msg.GetFieldValue("MatchChanLength")
						rcl, _ := msg.GetFieldValue("RouterChanLength")
						line = fmt.Sprintf("%d\t%s\t%v"+
							" ProcessMessageCount:%v"+
							" ProcessMessageSamples:%v"+
							" ProcessMessageAvgDuration:%v"+
							" MatchAvgDuration:%v"+
							" FilterChanLength:%v"+
							" MatchChanLength:%v"+
							" RouterChanLength:%v\n",
							msg.GetTimestamp()/1e9,
							filterName, msg.GetPayload(), pmc, pms, pmd,
							mad, fcl, mcl, rcl)
					}
					file.WriteString(line)
					file.Close()
				}
				sbxsLock.Lock()
				delete(sandboxes, filterName)
				sbxsLock.Unlock()
			}
			or.UpdateCursor(pack.QueueCursor)
			pack.Recycle(nil)
		case <-ticker:
			go h.PipelineConfig().AllReportsMsg()
		}
	}
	return
}

func defaultStarter(output *DashboardOutput) error {
	return output.server.ListenAndServe()
}

func overwriteFile(filename, s string) (err error) {
	var file *os.File
	if file, err = os.OpenFile(filename, os.O_WRONLY|os.O_TRUNC+os.O_CREATE, 0644); err == nil {
		file.WriteString(s)
		file.Close()
	}
	return
}

type DashPluginOutput struct {
	Name     string
	Filename string
}

type DashPluginListItem struct {
	Name    string
	Outputs []*DashPluginOutput
}

func overwritePluginListFile(dir string, sbxs map[string]*DashPluginListItem) (err error) {
	sbxSlice := make([]*DashPluginListItem, len(sbxs))
	i := 0
	for _, item := range sbxs {
		sbxSlice[i] = item
		i++
	}
	output := map[string][]*DashPluginListItem{
		"sandboxes": sbxSlice,
	}
	var file *os.File
	filename := filepath.Join(dir, "sandboxes.json")
	if file, err = os.OpenFile(filename, os.O_WRONLY|os.O_TRUNC+os.O_CREATE, 0644); err == nil {
		enc := json.NewEncoder(file)
		err = enc.Encode(output)
		file.Close()
	}
	return
}

func init() {
	RegisterPlugin("DashboardOutput", func() interface{} {
		return new(DashboardOutput)
	})
}
