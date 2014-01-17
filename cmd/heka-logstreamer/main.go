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

Example TOML config:

    log_directory = "$HOME/heka/logstreamer/testdir/"
    file_match = '/(?P<Year>\d+)/(?P<Month>\d+)/(?P<Type>\w+)\.log(\.(?P<Seq>\d+))?'
    priority = ["Year", "Month", "Day", "^Seq"]
    differentiator = ["website-", "Type"]

*/
package main

import (
	"flag"
	"fmt"
	"github.com/bbangert/toml"
	"github.com/mozilla-services/heka/logstreamer"
	"log"
	"os"
	"regexp"
	"sort"
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

func main() {
	configFile := flag.String("config", "logstreamer.toml", "Heka Logstreamer configuration file")

	flag.Parse()

	if flag.NFlag() == 0 {
		flag.PrintDefaults()
		os.Exit(0)
	}

	var config LogstreamerConfig
	if _, err := toml.DecodeFile(*configFile, &config); err != nil {
		log.Printf("Error decoding config file: %s", err)
		return
	}

	matchRegex := regexp.MustCompile(config.FileMatch)
	logfiles := logstreamer.ScanDirectoryForLogfiles(config.LogDirectory, matchRegex)
	err := logfiles.PopulateMatchParts(matchRegex, config.Translation)
	if err != nil {
		fmt.Printf("Unable to populate match parts with the regex: %s", err)
		os.Exit(0)
	}

	// Drop the oldest
	if config.OldestDuration != "" {
		dur, err := time.ParseDuration(config.OldestDuration)
		if err != nil {
			fmt.Printf("Unable to parse duration: %s", err)
			os.Exit(0)
		}
		logfiles = logfiles.FilterOld(time.Now().Add(-dur))
	}

	byp := logstreamer.ByPriority{Logfiles: logfiles, Priority: config.Priority}
	sort.Sort(byp)

	mfs := logstreamer.FilterMultipleStreamFiles(logfiles, config.Differentiator)
	fmt.Printf("Found %d Logstream(s).\n", len(mfs))
	for name, logfiles := range mfs {
		fmt.Printf("\nLogstream name: %s\n", name)
		fmt.Printf("Files: %d (printing oldest to newest)\n", len(logfiles))
		for _, logfile := range logfiles {
			fmt.Printf("\t%s\n", logfile.FileName)
		}
	}
}
