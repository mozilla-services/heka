/***** BEGIN LICENSE BLOCK *****
# This Source Code Form is subject to the terms of the Mozilla Public
# License, v. 2.0. If a copy of the MPL was not distributed with this file,
# You can obtain one at http://mozilla.org/MPL/2.0/.
#
# The Initial Developer of the Original Code is the Mozilla Foundation.
# Portions created by the Initial Developer are Copyright (C) 2015
# the Initial Developer. All Rights Reserved.
#
# Contributor(s):
#   Mike Trinkala (trink@mozilla.com)
#   Christian Vozar (christian@bellycard.com)
#   Kun Liu (git@lk.vc)
#
# ***** END LICENSE BLOCK *****/

package smtp

import (
	"bytes"
	"fmt"
	. "github.com/mozilla-services/heka/pipeline"
	pipeline_ts "github.com/mozilla-services/heka/pipeline/testsupport"
	"github.com/mozilla-services/heka/plugins"
	plugins_ts "github.com/mozilla-services/heka/plugins/testsupport"
	"github.com/rafrombrc/gomock/gomock"
	gs "github.com/rafrombrc/gospec/src/gospec"
	"net/smtp"
	"sync"
	//"time"
)

var sendCount int

func testSendMail(addr string, a smtp.Auth, from string, to []string, msg []byte) error {
	results := [][]byte{
		[]byte("From: heka@localhost.localdomain\r\nSubject: Heka [SmtpOutput]\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=\"utf-8\"\r\nContent-Transfer-Encoding: base64\r\n\r\nV3JpdGUgbWUgb3V0IHRvIHRoZSBuZXR3b3Jr"),
	}

	switch sendCount {
	case 0:
		if bytes.Compare(msg, results[0]) != 0 {
			return fmt.Errorf("Expected %s, Received %s", results[0], msg)
		}
	default:
		return fmt.Errorf("too many calls to SendMail")
	}
	sendCount++
	return nil
}

func SmtpOutputSpec(c gs.Context) {
	t := new(pipeline_ts.SimpleT)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	oth := plugins_ts.NewOutputTestHelper(ctrl)
	var wg sync.WaitGroup
	inChan := make(chan *PipelinePack, 1)
	pConfig := NewPipelineConfig(nil)

	encoder := new(plugins.PayloadEncoder)
	econfig := encoder.ConfigStruct().(*plugins.PayloadEncoderConfig)
	econfig.AppendNewlines = false
	encoder.Init(econfig)

	c.Specify("A SmtpOutput", func() {
		smtpOutput := new(SmtpOutput)

		config := smtpOutput.ConfigStruct().(*SmtpOutputConfig)
		config.SendTo = []string{"root"}

		msg := pipeline_ts.GetTestMessage()
		pack := NewPipelinePack(pConfig.InputRecycleChan())
		pack.Message = msg
		inChanCall := oth.MockOutputRunner.EXPECT().InChan().AnyTimes()
		inChanCall.Return(inChan)
		runnerName := oth.MockOutputRunner.EXPECT().Name().AnyTimes()
		runnerName.Return("SmtpOutput")
		oth.MockOutputRunner.EXPECT().Encoder().Return(encoder)
		encCall := oth.MockOutputRunner.EXPECT().Encode(pack)

		c.Specify("send email payload message", func() {
			err := smtpOutput.Init(config)
			c.Assume(err, gs.IsNil)
			smtpOutput.sendFunction = testSendMail

			outStr := "Write me out to the network"
			pack.Message.SetPayload(outStr)
			encCall.Return(encoder.Encode(pack))
			wg.Add(1)
			go func() {
				smtpOutput.Run(oth.MockOutputRunner, oth.MockHelper)
				wg.Done()
			}()
			inChan <- pack
			close(inChan)
			wg.Wait()
		})
	})

	c.Specify("SmtpOutput Message Body Encoding", func() {
		smtpOutput := new(SmtpOutput)
		chars := "123456789012345678901234567890123456789012345678901234567"
		charsE := "MTIzNDU2Nzg5MDEyMzQ1Njc4OTAxMjM0NTY3ODkwMTIzNDU2Nzg5MDEyMzQ1Njc4OTAxMjM0NTY3"
		examples := [][]string{
			{"Hello", "SGVsbG8="},
			{chars, charsE},
			{chars + chars, charsE + "\r\n" + charsE},
			{chars + chars + "Hello", charsE + "\r\n" + charsE + "\r\n" + "SGVsbG8="},
			{"", ""},
			{"1", "MQ=="},
		}
		for _, example := range examples {
			smtpOutput.encodeFullMsg([]byte(example[0]))
			c.Expect(string(smtpOutput.fullMsg), gs.Equals, example[1])
		}
	})

	// // Use this test with a real server
	// c.Specify("Real SmtpOutput output", func() {
	// 	smtpOutput := new(SmtpOutput)

	// 	config := smtpOutput.ConfigStruct().(*SmtpOutputConfig)
	// 	config.SendTo = []string{"root"}

	// 	msg := pipeline_ts.GetTestMessage()
	// 	pack := NewPipelinePack(pConfig.InputRecycleChan())
	// 	pack.Message = msg
	// 	pack.Decoded = true
	// 	inChanCall := oth.MockOutputRunner.EXPECT().InChan().AnyTimes()
	// 	inChanCall.Return(inChan)
	// 	runnerName := oth.MockOutputRunner.EXPECT().Name().AnyTimes()
	// 	runnerName.Return("SmtpOutput")
	// 	oth.MockOutputRunner.EXPECT().Encoder().Return(encoder)
	// 	encCall := oth.MockOutputRunner.EXPECT().Encode(pack)

	// 	c.Specify("send a real email essage", func() {

	// 		err := smtpOutput.Init(config)
	// 		c.Assume(err, gs.IsNil)

	// 		outStr := "Write me out to the network"
	// 		pack.Message.SetPayload(outStr)
	// 		encCall.Return(encoder.Encode(pack))
	// 		go func() {
	// 			wg.Add(1)
	// 			smtpOutput.Run(oth.MockOutputRunner, oth.MockHelper)
	// 			wg.Done()
	// 		}()
	// 		inChan <- pack
	// 		time.Sleep(1000) // allow time for the message output
	// 		close(inChan)
	// 		wg.Wait()
	// // manually check the mail
	//	})
	// })
}
