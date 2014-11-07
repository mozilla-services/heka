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
#   Rob Miller (rmiller@mozilla.com)
#
# ***** END LICENSE BLOCK *****/

/*

Main entry point for the `hekad` daemon. Loads the specified config and calls
`pipeline.Run` to launch the PluginRunners and all additional goroutines.

*/
package main

import (
	"flag"
	"fmt"
	"github.com/mozilla-services/heka/pipeline"
	_ "github.com/mozilla-services/heka/plugins"
	_ "github.com/mozilla-services/heka/plugins/amqp"
	_ "github.com/mozilla-services/heka/plugins/dasher"
	_ "github.com/mozilla-services/heka/plugins/elasticsearch"
	_ "github.com/mozilla-services/heka/plugins/file"
	_ "github.com/mozilla-services/heka/plugins/graphite"
	_ "github.com/mozilla-services/heka/plugins/http"
	_ "github.com/mozilla-services/heka/plugins/irc"
	_ "github.com/mozilla-services/heka/plugins/kafka"
	_ "github.com/mozilla-services/heka/plugins/logstreamer"
	_ "github.com/mozilla-services/heka/plugins/nagios"
	_ "github.com/mozilla-services/heka/plugins/payload"
	_ "github.com/mozilla-services/heka/plugins/process"
	_ "github.com/mozilla-services/heka/plugins/smtp"
	_ "github.com/mozilla-services/heka/plugins/statsd"
	_ "github.com/mozilla-services/heka/plugins/tcp"
	_ "github.com/mozilla-services/heka/plugins/udp"
	"os"
	"path/filepath"
)

const (
	VERSION = "0.9.0"
)

func main() {
	configPath := flag.String("config", filepath.FromSlash("/etc/hekad.toml"),
		"Config file or directory. If directory is specified then all files "+
			"in the directory will be loaded.")
	version := flag.Bool("version", false, "Output version and exit")
	flag.Parse()

	if flag.NFlag() == 0 {
		flag.PrintDefaults()
		os.Exit(0)
	}

	if *version {
		fmt.Println(VERSION)
		os.Exit(0)
	}

	pipeline.Start(*configPath)
}
