/***** BEGIN LICENSE BLOCK *****
# This Source Code Form is subject to the terms of the Mozilla Public
# License, v. 2.0. If a copy of the MPL was not distributed with this file,
# You can obtain one at http://mozilla.org/MPL/2.0/.
#
# The Initial Developer of the Original Code is the Mozilla Foundation.
# Portions created by the Initial Developer are Copyright (C) 2012-2015
# the Initial Developer. All Rights Reserved.
#
# Contributor(s):
#   Rob Miller (rmiller@mozilla.com)
#   Mike Trinkala (trink@mozilla.com)
#
# ***** END LICENSE BLOCK *****/

package file

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	. "github.com/mozilla-services/heka/pipeline"
	pipeline_ts "github.com/mozilla-services/heka/pipeline/testsupport"
	"github.com/mozilla-services/heka/plugins"
	plugins_ts "github.com/mozilla-services/heka/plugins/testsupport"
	"github.com/rafrombrc/gomock/gomock"
	gs "github.com/rafrombrc/gospec/src/gospec"
)

func FileOutputSpec(c gs.Context) {
	t := new(pipeline_ts.SimpleT)
	ctrl := gomock.NewController(t)
	tmpFileName := fmt.Sprintf("fileoutput-test-%d", time.Now().UnixNano())
	tmpFilePath := filepath.Join(os.TempDir(), tmpFileName)

	defer func() {
		ctrl.Finish()
		os.Remove(tmpFilePath)
	}()

	oth := plugins_ts.NewOutputTestHelper(ctrl)
	var wg sync.WaitGroup
	inChan := make(chan *PipelinePack, 1)
	pConfig := NewPipelineConfig(nil)

	c.Specify("A FileOutput", func() {
		fileOutput := new(FileOutput)
		encoder := new(plugins.PayloadEncoder)
		encoder.Init(encoder.ConfigStruct())

		config := fileOutput.ConfigStruct().(*FileOutputConfig)
		config.Path = tmpFilePath

		msg := pipeline_ts.GetTestMessage()
		pack := NewPipelinePack(pConfig.InputRecycleChan())
		pack.Message = msg
		pack.QueueCursor = "queuecursor"

		errChan := make(chan error, 1)

		c.Specify("w/ ProtobufEncoder", func() {
			encoder := new(ProtobufEncoder)
			encoder.SetPipelineConfig(pConfig)
			encoder.Init(nil)
			oth.MockOutputRunner.EXPECT().Encoder().Return(encoder)

			c.Specify("uses framing", func() {
				oth.MockOutputRunner.EXPECT().SetUseFraming(true)
				err := fileOutput.Init(config)
				defer os.Remove(tmpFilePath)
				c.Assume(err, gs.IsNil)
				oth.MockOutputRunner.EXPECT().InChan().Return(inChan)
				wg.Add(1)
				go func() {
					err = fileOutput.Run(oth.MockOutputRunner, oth.MockHelper)
					c.Expect(err, gs.IsNil)
					wg.Done()
				}()
				close(inChan)
				wg.Wait()
			})

			c.Specify("but not if config says not to", func() {
				useFraming := false
				config.UseFraming = &useFraming
				err := fileOutput.Init(config)
				defer os.Remove(tmpFilePath)
				c.Assume(err, gs.IsNil)
				oth.MockOutputRunner.EXPECT().InChan().Return(inChan)
				wg.Add(1)
				go func() {
					err = fileOutput.Run(oth.MockOutputRunner, oth.MockHelper)
					c.Expect(err, gs.IsNil)
					wg.Done()
				}()
				close(inChan)
				wg.Wait()
				// We should fail if SetUseFraming is called since we didn't
				// EXPECT it.

			})
		})

		c.Specify("tests rotation of files", func() {
			config.Path = "%Y-%m-%d"
			rotateChan := make(chan time.Time)
			closingChan := make(chan struct{})

			err := fileOutput.Init(config)
			defer fileOutput.file.Close()

			c.Assume(err, gs.IsNil)

			fileOutput.rotateChan = rotateChan
			fileOutput.closing = closingChan

			fileOutput.startRotateNotifier()

			go fileOutput.committer(oth.MockOutputRunner, errChan)

			c.Assume(fileOutput.path, gs.Equals, time.Now().Format("2006-01-02"))

			futureDuration, _ := time.ParseDuration("24h")
			futureNow := time.Now().Add(futureDuration)

			rotateChan <- futureNow

			c.Assume(fileOutput.path, gs.Equals, futureNow.Format("2006-01-02"))

			close(inChan)
			close(fileOutput.batchChan)
		})

		c.Specify("processes incoming messages", func() {
			err := fileOutput.Init(config)
			c.Assume(err, gs.IsNil)
			fileOutput.file.Close()
			// Save for comparison.
			payload := fmt.Sprintf("%s\n", pack.Message.GetPayload())

			oth.MockOutputRunner.EXPECT().InChan().Return(inChan)
			oth.MockOutputRunner.EXPECT().Encode(pack).Return(encoder.Encode(pack))
			go fileOutput.receiver(oth.MockOutputRunner, errChan)
			inChan <- pack
			close(inChan)
			outBatch := <-fileOutput.batchChan
			c.Expect(string(outBatch.data), gs.Equals, payload)
			c.Expect(outBatch.cursor, gs.Equals, pack.QueueCursor)
		})

		c.Specify("commits to a file", func() {
			outStr := "Write me out to the log file"
			outBytes := []byte(outStr)
			batch := &outBatch{
				data:   outBytes,
				cursor: pack.QueueCursor,
			}

			oth.MockOutputRunner.EXPECT().UpdateCursor(pack.QueueCursor)

			c.Specify("with default settings", func() {
				err := fileOutput.Init(config)
				c.Assume(err, gs.IsNil)

				// Start committer loop.
				go fileOutput.committer(oth.MockOutputRunner, errChan)

				// Feed and close the batchChan.
				go func() {
					fileOutput.batchChan <- batch
					_ = <-fileOutput.backChan // clear backChan to prevent blocking.
					close(fileOutput.batchChan)
				}()

				// Wait until we know processing has finished.
				<-fileOutput.closing

				tmpFile, err := os.Open(tmpFilePath)
				defer tmpFile.Close()
				c.Assume(err, gs.IsNil)
				contents, err := ioutil.ReadAll(tmpFile)
				c.Assume(err, gs.IsNil)
				c.Expect(string(contents), gs.Equals, outStr)
			})

			c.Specify("with different Perm settings", func() {
				config.Perm = "600"
				err := fileOutput.Init(config)
				c.Assume(err, gs.IsNil)

				// Start committer loop.
				go fileOutput.committer(oth.MockOutputRunner, errChan)

				// Feed and close the batchChan.
				go func() {
					fileOutput.batchChan <- batch
					_ = <-fileOutput.backChan // clear backChan to prevent blocking.
					close(fileOutput.batchChan)
				}()

				// Wait until we know processing has finished.
				<-fileOutput.closing

				tmpFile, err := os.Open(tmpFilePath)
				defer tmpFile.Close()
				c.Assume(err, gs.IsNil)
				fileInfo, err := tmpFile.Stat()
				c.Assume(err, gs.IsNil)
				fileMode := fileInfo.Mode()
				if runtime.GOOS == "windows" {
					c.Expect(fileMode.String(), pipeline_ts.StringContains, "-rw-rw-rw-")
				} else {
					// 7 consecutive dashes implies no perms for group or other.
					c.Expect(fileMode.String(), pipeline_ts.StringContains, "-------")
				}
			})
		})

		if runtime.GOOS != "windows" {
			if u, err := user.Current(); err != nil && u.Uid != "0" {
				c.Specify("Init halts if basedirectory is not writable", func() {
					tmpdir := filepath.Join(os.TempDir(), "tmpdir")
					err := os.MkdirAll(tmpdir, 0400)
					c.Assume(err, gs.IsNil)
					config.Path = filepath.Join(tmpdir, "out.txt")
					err = fileOutput.Init(config)
					c.Assume(err, gs.Not(gs.IsNil))
					os.RemoveAll(tmpdir)
				})
			}

			c.Specify("honors folder_perm setting", func() {
				config.FolderPerm = "750"
				subdir := filepath.Join(os.TempDir(), "subdir")
				config.Path = filepath.Join(subdir, "out.txt")
				err := fileOutput.Init(config)
				defer os.RemoveAll(subdir)
				c.Assume(err, gs.IsNil)

				fi, err := os.Stat(subdir)
				c.Expect(fi.IsDir(), gs.IsTrue)
				c.Expect(fi.Mode().Perm(), gs.Equals, os.FileMode(0750))
			})
		}

		c.Specify("that starts receiving w/ a flush interval", func() {
			config.FlushInterval = 100000000 // We'll trigger the timer manually.
			inChan := make(chan *PipelinePack)
			oth.MockOutputRunner.EXPECT().InChan().Return(inChan)
			timerChan := make(chan time.Time)

			msg2 := pipeline_ts.GetTestMessage()
			pack2 := NewPipelinePack(pConfig.InputRecycleChan())
			pack2.Message = msg2

			recvWithConfig := func(config *FileOutputConfig) {
				err := fileOutput.Init(config)
				c.Assume(err, gs.IsNil)

				fileOutput.timerChan = timerChan
				go fileOutput.receiver(oth.MockOutputRunner, errChan)
				runtime.Gosched() // Yield so receiver will start.
			}

			cleanUp := func() {
				close(inChan)
				fileOutput.file.Close()
			}

			c.Specify("honors flush interval", func() {
				oth.MockOutputRunner.EXPECT().Encode(pack).Return(encoder.Encode(pack))
				recvWithConfig(config)
				defer cleanUp()
				inChan <- pack
				select {
				case _ = <-fileOutput.batchChan:
					c.Expect("", gs.Equals, "fileOutput.batchChan should NOT have fired yet")
				default:
				}
				timerChan <- time.Now()
				select {
				case _ = <-fileOutput.batchChan:
				default:
					c.Expect("", gs.Equals, "fileOutput.batchChan SHOULD have fired by now")
				}
			})

			c.Specify("honors flush interval AND flush count", func() {
				oth.MockOutputRunner.EXPECT().Encode(pack).Return(encoder.Encode(pack))
				oth.MockOutputRunner.EXPECT().Encode(pack2).Return(encoder.Encode(pack2))
				config.FlushCount = 2
				recvWithConfig(config)
				defer cleanUp()
				inChan <- pack

				select {
				case <-fileOutput.batchChan:
					c.Expect("", gs.Equals, "fileOutput.batchChan should NOT have fired yet")
				default:
				}

				timerChan <- time.Now()
				select {
				case <-fileOutput.batchChan:
					c.Expect("", gs.Equals, "fileOutput.batchChan should NOT have fired yet")
				default:
				}

				inChan <- pack2
				runtime.Gosched()
				select {
				case <-fileOutput.batchChan:
				default:
					c.Expect("", gs.Equals, "fileOutput.batchChan SHOULD have fired by now")
				}
			})

			c.Specify("honors flush interval OR flush count", func() {
				oth.MockOutputRunner.EXPECT().Encode(gomock.Any()).Return(encoder.Encode(pack))
				config.FlushCount = 2
				config.FlushOperator = "OR"
				recvWithConfig(config)
				defer cleanUp()
				inChan <- pack

				select {
				case <-fileOutput.batchChan:
					c.Expect("", gs.Equals, "fileOutput.batchChan should NOT have fired yet")
				default:
				}

				c.Specify("when interval triggers first", func() {
					timerChan <- time.Now()
					runtime.Gosched()
					select {
					case <-fileOutput.batchChan:
					default:
						c.Expect("", gs.Equals, "fileOutput.batchChan SHOULD have fired by now")
					}
				})

				c.Specify("when count triggers first", func() {
					out, err := encoder.Encode(pack2)
					oth.MockOutputRunner.EXPECT().Encode(gomock.Any()).Return(out, err)
					inChan <- pack2
					runtime.Gosched()
					select {
					case <-fileOutput.batchChan:
					default:
						c.Expect("", gs.Equals, "fileOutput.batchChan SHOULD have fired by now")
					}
				})
			})
		})
	})
}
