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
#   Mike Trinkala (trink@mozilla.com)
#
# ***** END LICENSE BLOCK *****/

package smtp

import (
	"code.google.com/p/gomock/gomock"
	. "github.com/mozilla-services/heka/pipeline"
	pipeline_ts "github.com/mozilla-services/heka/pipeline/testsupport"
	plugins_ts "github.com/mozilla-services/heka/plugins/testsupport"
	gs "github.com/rafrombrc/gospec/src/gospec"
	"sync"
	"testing"
	"time"
)

func TestAllSpecs(t *testing.T) {
	r := gs.NewRunner()
	r.Parallel = false

	// Cannot mock the smtp.SendMail function so the tests were run against local mail server
	// r.AddSpec(SmtpOutputSpec)

	gs.MainGoTest(r, t)
}

func SmtpOutputSpec(c gs.Context) {
	t := new(pipeline_ts.SimpleT)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	oth := plugins_ts.NewOutputTestHelper(ctrl)
	var wg sync.WaitGroup
	inChan := make(chan *PipelinePack, 1)
	pConfig := NewPipelineConfig(nil)

	c.Specify("A SmtpOutput", func() {
		smtpOutput := new(SmtpOutput)

		config := smtpOutput.ConfigStruct().(*SmtpOutputConfig)
		config.SendTo = []string{"root"}

		msg := pipeline_ts.GetTestMessage()
		pack := NewPipelinePack(pConfig.InputRecycleChan())
		pack.Message = msg
		pack.Decoded = true
		inChanCall := oth.MockOutputRunner.EXPECT().InChan().AnyTimes()
		inChanCall.Return(inChan)
		runnerName := oth.MockOutputRunner.EXPECT().Name().AnyTimes()
		runnerName.Return("SmtpOutput")

		c.Specify("send email payload message", func() {
			err := smtpOutput.Init(config)
			c.Assume(err, gs.IsNil)

			outStr := "Write me out to the network"
			pack.Message.SetPayload(outStr)
			go func() {
				wg.Add(1)
				smtpOutput.Run(oth.MockOutputRunner, oth.MockHelper)
				wg.Done()
			}()
			inChan <- pack
			time.Sleep(1000) // allow time for the message output
			close(inChan)
			wg.Wait()
			// manually check the mail
		})

		c.Specify("send email json message", func() {
			config.PayloadOnly = false

			err := smtpOutput.Init(config)
			c.Assume(err, gs.IsNil)

			outStr := "Write me out to the network"
			pack.Message.SetPayload(outStr)
			go func() {
				wg.Add(1)
				smtpOutput.Run(oth.MockOutputRunner, oth.MockHelper)
				wg.Done()
			}()
			inChan <- pack
			time.Sleep(1000) // allow time for the message output
			close(inChan)
			wg.Wait()
			// manually check the mail
		})
	})
}
