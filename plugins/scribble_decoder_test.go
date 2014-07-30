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
#   Rob Miller (rmiller@mozilla.com)
#
# ***** END LICENSE BLOCK *****/

package plugins

import (
	. "github.com/mozilla-services/heka/pipeline"
	pipeline_ts "github.com/mozilla-services/heka/pipeline/testsupport"
	"github.com/rafrombrc/gomock/gomock"
	gs "github.com/rafrombrc/gospec/src/gospec"
)

func ScribbleDecoderSpec(c gs.Context) {
	t := &pipeline_ts.SimpleT{}
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	c.Specify("A ScribbleDecoder", func() {
		decoder := new(ScribbleDecoder)
		config := decoder.ConfigStruct().(*ScribbleDecoderConfig)
		myType := "myType"
		myPayload := "myPayload"
		config.MessageFields = MessageTemplate{"Type": myType, "Payload": myPayload}
		supply := make(chan *PipelinePack, 1)
		pack := NewPipelinePack(supply)

		c.Specify("sets basic values correctly", func() {
			decoder.Init(config)
			packs, err := decoder.Decode(pack)
			c.Expect(err, gs.IsNil)
			c.Expect(len(packs), gs.Equals, 1)
			c.Expect(pack.Message.GetType(), gs.Equals, myType)
			c.Expect(pack.Message.GetPayload(), gs.Equals, myPayload)
		})
	})
}
