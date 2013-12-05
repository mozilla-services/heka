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
#   Mike Trinkala (trink@mozilla.com)
#
# ***** END LICENSE BLOCK *****/

package plugins

import (
	"github.com/mozilla-services/heka/message"
	. "github.com/mozilla-services/heka/pipeline"
	"log"
	"time"
)

// Output plugin that writes message contents out using Go standard library's
// `log` package.
type LogOutput struct {
	payloadOnly bool
}

func (self *LogOutput) Init(config interface{}) (err error) {
	conf := config.(PluginConfig)
	if p, ok := conf["payload_only"]; ok {
		self.payloadOnly, ok = p.(bool)
	}
	return
}

func (self *LogOutput) Run(or OutputRunner, h PluginHelper) (err error) {
	inChan := or.InChan()

	var (
		pack *PipelinePack
		msg  *message.Message
	)
	for pack = range inChan {
		msg = pack.Message
		if self.payloadOnly {
			log.Printf(msg.GetPayload())
		} else {
			log.Printf("<\n\tTimestamp: %s\n"+
				"\tType: %s\n"+
				"\tHostname: %s\n"+
				"\tPid: %d\n"+
				"\tUUID: %s\n"+
				"\tLogger: %s\n"+
				"\tPayload: %s\n"+
				"\tEnvVersion: %s\n"+
				"\tSeverity: %d\n"+
				"\tFields: %+v\n>\n",
				time.Unix(0, msg.GetTimestamp()), msg.GetType(),
				msg.GetHostname(), msg.GetPid(), msg.GetUuidString(),
				msg.GetLogger(), msg.GetPayload(), msg.GetEnvVersion(),
				msg.GetSeverity(), msg.Fields)
		}
		pack.Recycle()
	}
	return
}

func init() {
	RegisterPlugin("LogOutput", func() interface{} {
		return new(LogOutput)
	})
}
