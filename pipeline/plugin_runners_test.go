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
	"code.google.com/p/go-uuid/uuid"
	"code.google.com/p/gomock/gomock"
	"fmt"
	ts "github.com/mozilla-services/heka/testsupport"
	gs "github.com/rafrombrc/gospec/src/gospec"
	"sync"
)

type MockDRunnerPack struct {
	runner *MockDecoderRunner
	uuid   string
}

func PluginRunnerBaseSpec(c gs.Context) {
	t := &ts.SimpleT{}
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	NewMockDRunnerPack := func() *MockDRunnerPack {
		dpack := new(MockDRunnerPack)
		dpack.runner = NewMockDecoderRunner(ctrl)
		dpack.uuid = uuid.NewRandom().String()
		return dpack
	}

	NewMockSlice := func(count int) (mocks []DecoderRunner) {
		mocks = make([]DecoderRunner, count)
		var dpack *MockDRunnerPack
		for i := 0; i < count; i++ {
			dpack = NewMockDRunnerPack()
			dpack.runner.EXPECT().UUID().Times(2).Return(dpack.uuid)
			dpack.runner.EXPECT().Name().Return(fmt.Sprintf("mock%d", i))
			dpack.runner.EXPECT().setOwner(gomock.Any())
			newName := fmt.Sprintf("test-mock%d-%s", i, dpack.uuid[:6])
			dpack.runner.EXPECT().SetName(newName)
			mocks[i] = dpack.runner
		}
		return
	}

	NewMockMap := func(count int) (mocks map[string]DecoderRunner) {
		mocks = make(map[string]DecoderRunner)
		runners := NewMockSlice(count)
		for i := 0; i < count; i++ {
			name := fmt.Sprintf("mock%d", i)
			mocks[name] = runners[i]
		}
		return
	}

	c.Specify("pRunnerBase instances", func() {
		h := NewMockPluginHelper(ctrl)
		p := &pRunnerBase{
			name:         "test",
			h:            h,
			decoders:     make(map[string]DecoderRunner),
			decodersLock: new(sync.Mutex),
		}
		c.Assume(len(p.decoders), gs.Equals, 0)

		c.Specify("add decoders to the registry when NewDecoders is called", func() {
			dSet := NewMockMap(5)
			h.EXPECT().decoders().Return(dSet)
			p.NewDecoders()
			c.Expect(len(p.decoders), gs.Equals, 5)
		})

		c.Specify("add decoders to the registry when NewDecodersByEncoding is called", func() {
			dSet := NewMockSlice(5)
			h.EXPECT().decodersByEncoding().Return(dSet)
			returnedSet := p.NewDecodersByEncoding()
			for i := 0; i < 5; i++ {
				c.Expect(dSet[i], gs.Equals, returnedSet[i])
			}
			c.Expect(len(p.decoders), gs.Equals, 5)
		})
	})
}
