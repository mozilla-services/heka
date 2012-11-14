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
	"github.com/bitly/go-notify"
	gs "github.com/rafrombrc/gospec/src/gospec"
	"github.com/rafrombrc/gospec/src/gospec"
	"io/ioutil"
	"os"
	"runtime"
	"strings"
	"time"
)

func OutputsSpec(c gospec.Context) {
	t := new(SimpleT)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	c.Specify("A FileOutput", func() {
		fileOutput := new(FileOutput)
		tmpFileName := fmt.Sprintf("fileoutput-test-%d", time.Now().UnixNano())
		tmpFilePath := fmt.Sprint(os.TempDir(), string(os.PathSeparator),
			tmpFileName)
		config := fileOutput.ConfigStruct().(*FileOutputConfig)
		config.Path = tmpFilePath

		msg := getTestMessage()
		pipelinePack := getTestPipelinePack()
		pipelinePack.Message = msg
		pipelinePack.Decoded = true

		// The actual tests are littered w/ scheduler yields (i.e.
		// runtime.Gosched() calls) so we give the output a chance to respond
		// to the messages we're sending.

		c.Specify("writes text", func() {
			err := fileOutput.Init(config)
			runtime.Gosched()
			c.Assume(err, gs.IsNil)

			c.Specify("by default", func() {
				fileOutput.Deliver(pipelinePack)
				runtime.Gosched()

				err = notify.Post(STOP, nil)
				c.Assume(err, gs.IsNil)
				runtime.Gosched()

				tmpFile, err := os.Open(tmpFilePath)
				defer tmpFile.Close()
				c.Assume(err, gs.IsNil)
				contents, err := ioutil.ReadAll(tmpFile)
				c.Assume(err, gs.IsNil)
				c.Expect(string(contents), gs.Equals, msg.Payload+"\n")
			})

			c.Specify("w/ a prepended timestamp when specified", func() {
				fileOutput.prefix_ts = true
				fileOutput.Deliver(pipelinePack)
				runtime.Gosched()

				err = notify.Post(STOP, nil)
				c.Assume(err, gs.IsNil)
				runtime.Gosched()

				tmpFile, err := os.Open(tmpFilePath)
				defer tmpFile.Close()
				c.Expect(err, gs.IsNil)
				contents, err := ioutil.ReadAll(tmpFile)
				c.Expect(err, gs.IsNil)
				strContents := string(contents)
				c.Expect(strContents, gs.Satisfies,
					strings.Contains(strContents, msg.Payload))
			})
		})

		os.Remove(tmpFilePath) // clean up after ourselves
	})
}
