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
	"code.google.com/p/gomock/gomock"
	"fmt"
	ts "github.com/mozilla-services/heka/testsupport"
	gs "github.com/rafrombrc/gospec/src/gospec"
	"strconv"
	"sync"
)

func StatsdInputSpec(c gs.Context) {
	t := &ts.SimpleT{}
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	pConfig := NewPipelineConfig(nil)
	ith := new(InputTestHelper)
	ith.Msg = getTestMessage()
	ith.Pack = NewPipelinePack(pConfig.inputRecycleChan)
	ith.PackSupply = make(chan *PipelinePack, 1)

	// Specify localhost, but we're not really going to use the network
	ith.AddrStr = "localhost:55565"
	ith.ResolvedAddrStr = "127.0.0.1:55565"

	// set up mock helper, input runner, and stat accumulator
	ith.MockHelper = NewMockPluginHelper(ctrl)
	ith.MockInputRunner = NewMockInputRunner(ctrl)
	mockStatAccum := NewMockStatAccumulator(ctrl)

	c.Specify("A StatsdInput", func() {
		statsdInput := StatsdInput{}
		config := statsdInput.ConfigStruct().(*StatsdInputConfig)

		config.Address = ith.AddrStr
		err := statsdInput.Init(config)
		c.Assume(err, gs.IsNil)
		realListener := statsdInput.listener
		c.Expect(realListener.LocalAddr().String(), gs.Equals, ith.ResolvedAddrStr)
		realListener.Close()
		mockListener := ts.NewMockConn(ctrl)
		statsdInput.listener = mockListener

		ith.MockHelper.EXPECT().StatAccumulator("StatAccumInput").Return(mockStatAccum, nil)
		mockListener.EXPECT().Close()
		mockListener.EXPECT().SetReadDeadline(gomock.Any())

		c.Specify("sends a Stat to the StatAccumulator", func() {
			statName := "sample.count"
			statVal := 303
			msg := fmt.Sprintf("%s:%d|c\n", statName, statVal)
			expected := Stat{statName, strconv.Itoa(statVal), "c", float32(1)}
			mockStatAccum.EXPECT().DropStat(expected).Return(true)
			readCall := mockListener.EXPECT().Read(make([]byte, 512))
			readCall.Return(len(msg), nil)
			readCall.Do(func(msgBytes []byte) {
				copy(msgBytes, []byte(msg))
				statsdInput.Stop()
			})
			var wg sync.WaitGroup
			wg.Add(1)
			go func() {
				err = statsdInput.Run(ith.MockInputRunner, ith.MockHelper)
				c.Expect(err, gs.IsNil)
				wg.Done()
			}()
			wg.Wait()
		})
	})
}
