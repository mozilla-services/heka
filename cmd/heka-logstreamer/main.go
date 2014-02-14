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
#   Ben Bangert (bbangert@mozilla.com)
#
# ***** END LICENSE BLOCK *****/

/*

Heka Logstreamer verifier.

*/
package main

import (
	"flag"
	"fmt"
	"github.com/bbangert/toml"
	"github.com/mozilla-services/heka/logstreamer"
	"log"
	"os"
	"time"
)

// Logstreamer config struct
type LogstreamerConfig struct {
	LogDirectory   string `toml:"log_directory"`
	FileMatch      string `toml:"file_match"`
	Priority       []string
	Differentiator []string
	OldestDuration string `toml:"oldest_duration"`
	Translation    logstreamer.SubmatchTranslationMap
}

type Basic struct {
	PluginType string `toml:"type"`
}

// File's config
type FileConfig map[string]toml.Primitive

func main() {
	configFile := flag.String("config", "logstreamer.toml", "Heka Logstreamer configuration file")

	flag.Parse()

	if flag.NFlag() == 0 {
		flag.PrintDefaults()
		os.Exit(0)
	}

	fconfig := make(FileConfig)
	if _, err := toml.DecodeFile(*configFile, &fconfig); err != nil {
		log.Printf("Error decoding config file: %s", err)
		return
	}

	// Filter out logstream inputs
	basic := new(Basic)
	inputs := make(map[string]toml.Primitive)
	for name, prim := range fconfig {
		if name == "LogstreamerInput" {
			inputs[name] = prim
		} else if err := toml.PrimitiveDecode(prim, &basic); err == nil {
			if basic.PluginType == "LogstreamerInput" {
				inputs[name] = prim
			}
		}
	}

	// Go through the logstreams and parse their configs
	for name, prim := range inputs {
		parseConfig(name, prim)
	}
}

func parseConfig(name string, prim toml.Primitive) {
	config := LogstreamerConfig{
		OldestDuration: "720h",
		Differentiator: []string{name},
		LogDirectory:   "/var/log",
	}
	if err := toml.PrimitiveDecode(prim, &config); err != nil {
		log.Printf("Error decoding config file: %s", err)
		return
	}

	if len(config.FileMatch) > 0 && config.FileMatch[len(config.FileMatch)-2:] != "$" {
		config.FileMatch += "$"
	}

	sp := &logstreamer.SortPattern{
		FileMatch:      config.FileMatch,
		Translation:    config.Translation,
		Priority:       config.Priority,
		Differentiator: config.Differentiator,
	}
	oldest, _ := time.ParseDuration(config.OldestDuration)
	ls := logstreamer.NewLogstreamSet(sp, oldest, config.LogDirectory, "")
	streams, errs := ls.ScanForLogstreams()
	if errs.IsError() {
		fmt.Printf("Error scanning: %s\n", errs)
		os.Exit(0)
	}

	fmt.Printf("Found %d Logstream(s) for section [%s].\n", len(streams), name)
	for _, name := range streams {
		stream, _ := ls.GetLogstream(name)
		fmt.Printf("\nLogstream name: [%s]\n", name)
		fmt.Printf("Files: %d (printing oldest to newest)\n", len(stream.GetLogfiles()))
		for _, logfile := range stream.GetLogfiles() {
			fmt.Printf("\t%s\n", logfile.FileName)
		}
	}
}
