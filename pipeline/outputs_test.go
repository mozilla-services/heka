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
	"encoding/json"
	"fmt"
	gs "github.com/rafrombrc/gospec/src/gospec"
	ts "heka/testsupport"
	"io/ioutil"
	"os"
	"time"
)

func OutputsSpec(c gs.Context) {
	t := new(ts.SimpleT)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	c.Specify("A FileWriter", func() {
		fileWriter := new(FileWriter)

		tmpFileName := fmt.Sprintf("fileoutput-test-%d", time.Now().UnixNano())
		tmpFilePath := fmt.Sprint(os.TempDir(), string(os.PathSeparator),
			tmpFileName)
		config := fileWriter.ConfigStruct().(*FileWriterConfig)
		config.Path = tmpFilePath

		msg := getTestMessage()
		pipelinePack := getTestPipelinePack()
		pipelinePack.Message = msg
		pipelinePack.Decoded = true

		stopAndDelete := func() {
			os.Remove(tmpFilePath)
			fileWriter.Event(STOP)
		}

		toString := func(outData interface{}) string {
			return string(*(outData.(*[]byte)))
		}

		c.Specify("makes a pointer to a byte slice", func() {
			outData := fileWriter.MakeOutData()
			_, ok := outData.(*[]byte)
			c.Expect(ok, gs.IsTrue)
		})

		c.Specify("zeroes a byte slice", func() {
			outBytes := make([]byte, 0, 100)
			str := "This is a test"
			outBytes = append(outBytes, []byte(str)...)
			c.Expect(len(outBytes), gs.Equals, len(str))
			fileWriter.ZeroOutData(&outBytes)
			c.Expect(len(outBytes), gs.Equals, 0)
		})

		c.Specify("correctly formats text output", func() {
			err := fileWriter.Init(config)
			defer stopAndDelete()
			c.Assume(err, gs.IsNil)
			outData := fileWriter.MakeOutData()

			c.Specify("by default", func() {
				fileWriter.PrepOutData(pipelinePack, outData, nil)
				c.Expect(toString(outData), gs.Equals, msg.Payload+"\n")
			})

			c.Specify("w/ a prepended timestamp when specified", func() {
				fileWriter.prefix_ts = true
				fileWriter.PrepOutData(pipelinePack, outData, nil)
				// Test will fail if date flips btn PrepOutData and todayStr
				// calculation... should be extremely rare.
				todayStr := time.Now().Format("[2006/Jan/02:")
				strContents := toString(outData)
				c.Expect(strContents, ts.StringContains, msg.Payload)
				c.Expect(strContents, ts.StringStartsWith, todayStr)
			})
		})

		c.Specify("correctly formats JSON output", func() {
			config.Format = "json"
			err := fileWriter.Init(config)
			defer stopAndDelete()
			c.Assume(err, gs.IsNil)
			outData := fileWriter.MakeOutData()

			c.Specify("when specified", func() {
				fileWriter.PrepOutData(pipelinePack, outData, nil)
				msgJson, err := json.Marshal(pipelinePack.Message)
				c.Assume(err, gs.IsNil)
				c.Expect(toString(outData), gs.Equals, string(msgJson)+"\n")
			})

			c.Specify("and with a timestamp", func() {
				fileWriter.prefix_ts = true
				fileWriter.PrepOutData(pipelinePack, outData, nil)
				// Test will fail if date flips btn PrepOutData and todayStr
				// calculation... should be extremely rare.
				todayStr := time.Now().Format("[2006/Jan/02:")
				strContents := toString(outData)
				msgJson, err := json.Marshal(pipelinePack.Message)
				c.Assume(err, gs.IsNil)
				c.Expect(strContents, ts.StringContains, string(msgJson)+"\n")
				c.Expect(strContents, ts.StringStartsWith, todayStr)
			})
		})

		c.Specify("writes out to a file", func() {
			outData := fileWriter.MakeOutData()
			outBytes := outData.(*[]byte)
			outStr := "Write me out to the log file"
			*outBytes = append(*outBytes, []byte(outStr)...)

			c.Specify("with default settings", func() {
				err := fileWriter.Init(config)
				defer stopAndDelete()
				c.Assume(err, gs.IsNil)
				err = fileWriter.Write(outData)
				c.Expect(err, gs.IsNil)

				tmpFile, err := os.Open(tmpFilePath)
				defer tmpFile.Close()
				c.Assume(err, gs.IsNil)
				contents, err := ioutil.ReadAll(tmpFile)
				c.Assume(err, gs.IsNil)
				c.Expect(string(contents), gs.Equals, outStr)
			})

			c.Specify("honors different Perm settings", func() {
				config.Perm = 0600
				err := fileWriter.Init(config)
				defer stopAndDelete()
				c.Assume(err, gs.IsNil)
				err = fileWriter.Write(outData)
				c.Expect(err, gs.IsNil)
				tmpFile, err := os.Open(tmpFilePath)
				defer tmpFile.Close()
				c.Assume(err, gs.IsNil)
				fileInfo, err := tmpFile.Stat()
				c.Assume(err, gs.IsNil)
				fileMode := fileInfo.Mode()
				// 7 consecutive dashes implies no perms for group or other
				c.Expect(fileMode.String(), ts.StringContains, "-------")
			})
		})
	})
}
