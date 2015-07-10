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
#   Mike Trinkala (trink@mozilla.com)
#   Rob Miller (rmiller@mozilla.com)
#
# ***** END LICENSE BLOCK *****/

package plugins

import (
	"os"
	"testing"
	"time"

	"github.com/mozilla-services/heka/message"
	"github.com/pborman/uuid"
	gs "github.com/rafrombrc/gospec/src/gospec"
)

func TestAllSpecs(t *testing.T) {
	r := gs.NewRunner()
	r.Parallel = false

	r.AddSpec(InputSpec)
	r.AddSpec(FilterSpec)
	r.AddSpec(DecoderSpec)
	r.AddSpec(EncoderSpec)
	r.AddSpec(OutputSpec)

	gs.MainGoTest(r, t)
}

func getTestMessage() *message.Message {
	hostname := "my.host.name"
	field, _ := message.NewField("foo", "bar", "")
	msg := &message.Message{}
	msg.SetType("TEST")
	msg.SetTimestamp(time.Now().UnixNano())
	msg.SetUuid(uuid.NewRandom())
	msg.SetLogger("GoSpec")
	msg.SetSeverity(int32(6))
	msg.SetPayload("Test Payload")
	msg.SetEnvVersion("0.8")
	msg.SetPid(int32(os.Getpid()))
	msg.SetHostname(hostname)
	msg.AddField(field)
	return msg
}
