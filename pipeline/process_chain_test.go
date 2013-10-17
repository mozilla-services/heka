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
#   Victor Ng (vng@mozilla.com)
#
# ***** END LICENSE BLOCK *****/

package pipeline

import (
	"bytes"
	gs "github.com/rafrombrc/gospec/src/gospec"
	"io"
	"strings"
	"time"
)

func Readoutput(r io.Reader) string {
	buf := new(bytes.Buffer)
	buf.ReadFrom(r)
	s := buf.String()
	return s
}

func ProcessChainSpec(c gs.Context) {

	PIPE_TEST_FILE := "../testsupport/process_input_pipes_test.txt"
	PIPE_TEST_OUTPUT := "this|is|a|test|\nignore this line\nand this line\n"

	c.Specify("A ManagedCommand", func() {
		c.Specify("can run a single command", func() {
			Path := "tail"
			cmd := &ManagedCmd{
				Path: Path,
				Args: []string{PIPE_TEST_FILE},
			}
			cmd.Init()
			r, err := cmd.StdoutPipe()
			c.Expect(err, gs.IsNil)
			output := make(chan string, 1)
			go func() {
				output <- Readoutput(r)
			}()
			cmd.Start()
			cmd.Wait()
			c.Expect(<-output, gs.Equals, PIPE_TEST_OUTPUT)
		})

		c.Specify("honors nonzero timeouts", func() {
			Path := "tail"
			timeout := time.Second * 1
			cmd := &ManagedCmd{
				Path:             Path,
				Args:             []string{"-f", PIPE_TEST_FILE},
				timeout_duration: timeout,
			}
			cmd.Init()
			r, err := cmd.StdoutPipe()
			c.Expect(err, gs.IsNil)
			output := make(chan string, 1)
			go func() {
				output <- Readoutput(r)
			}()
			cmd.Start()
			start := time.Now()
			cmd.Wait()
			end := time.Now()
			actual_duration := end.Sub(start)

			c.Expect(<-output, gs.Equals, PIPE_TEST_OUTPUT)
			c.Expect(actual_duration >= timeout, gs.Equals, true)
		})

		c.Specify("reads process stderr properly", func() {
			Path := "tail"
			cmd := &ManagedCmd{
				Path: Path,
				Args: []string{"not_a_file.txt"},
			}
			cmd.Init()

			stderr_r, err := cmd.StderrPipe()
			c.Expect(err, gs.IsNil)

			stdout_r, err := cmd.StdoutPipe()
			c.Expect(err, gs.IsNil)

			stderr_output := make(chan string, 1)
			stdout_output := make(chan string, 1)
			go func() {
				stderr_output <- Readoutput(stderr_r)
			}()

			go func() {
				stdout_output <- Readoutput(stdout_r)
			}()
			cmd.Start()
			cmd.Wait()
			expected_output := "tail: not_a_file.txt: No such file or directory"
			c.Expect(strings.Contains(<-stderr_output, expected_output), gs.Equals, true)
			c.Expect(<-stdout_output, gs.Equals, "")
		})

		c.Specify("can be terminated before timeout occurs", func() {
			Path := "tail"
			timeout := time.Second * 3
			cmd := &ManagedCmd{
				Path:             Path,
				Args:             []string{"-f", PIPE_TEST_FILE},
				timeout_duration: timeout,
			}
			cmd.Init()
			r, err := cmd.StdoutPipe()
			c.Expect(err, gs.IsNil)
			output := make(chan string, 1)
			go func() {
				output <- Readoutput(r)
			}()
			cmd.Start()
			start := time.Now()
			time.Sleep(time.Second * 1)
			cmd.Stopchan <- true
			cmd.Wait()
			end := time.Now()
			actual_duration := end.Sub(start)
			c.Expect(<-output, gs.Equals, PIPE_TEST_OUTPUT)
			c.Expect(actual_duration < timeout, gs.Equals, true)
		})

		c.Specify("can reset commands to run again", func() {
			Path := "tail"
			cmd := &ManagedCmd{
				Path: Path,
				Args: []string{PIPE_TEST_FILE},
			}
			cmd.Init()

			r, err := cmd.StdoutPipe()
			c.Expect(err, gs.IsNil)
			output := make(chan string, 1)
			go func() {
				output <- Readoutput(r)
			}()
			cmd.Start()
			cmd.Wait()
			c.Expect(<-output, gs.Equals, PIPE_TEST_OUTPUT)

			// Reset and rerun it
			cmd.reset()

			r, err = cmd.StdoutPipe()
			c.Expect(err, gs.IsNil)
			go func() {
				output <- Readoutput(r)
			}()
			cmd.Start()
			cmd.Wait()
			c.Expect(<-output, gs.Equals, PIPE_TEST_OUTPUT)
		})
	})

	c.Specify("A ProcessChain", func() {
		c.Specify("can pipe cat and grep", func() {
			// This test assumes tail and grep
			var err error

			chain := &CommandChain{timeout_duration: 0}
			chain.Init()
			// tail -f will never terminate
			chain.AddStep("cat", PIPE_TEST_FILE)
			chain.AddStep("grep", "-i", "TEST")

			stdout_reader, err := chain.StdoutPipe()
			c.Expect(err, gs.IsNil)

			stderr_reader, err := chain.StderrPipe()
			c.Expect(err, gs.IsNil)

			outChan := make(chan string, 1)
			errChan := make(chan string, 1)

			go func() {
				outChan <- Readoutput(stdout_reader)
			}()
			go func() {
				errChan <- Readoutput(stderr_reader)
			}()

			err = chain.Start()
			c.Expect(err, gs.IsNil)
			err = chain.Wait()
			c.Expect(err, gs.IsNil)

			c.Expect(<-errChan, gs.Equals, "")
			c.Expect(<-outChan, gs.Equals, "this|is|a|test|\n")

		})

		c.Specify("will honor timeouts", func() {
			// This test assumes tail and grep
			var err error

			timeout := time.Second * 1
			chain := &CommandChain{timeout_duration: timeout}
			chain.Init()
			// tail -f will never terminate
			chain.AddStep("tail", "-f", PIPE_TEST_FILE)
			chain.AddStep("grep", "-i", "TEST")

			stdout_reader, err := chain.StdoutPipe()
			c.Expect(err, gs.IsNil)

			stderr_reader, err := chain.StderrPipe()
			c.Expect(err, gs.IsNil)

			go Readoutput(stdout_reader)
			go Readoutput(stderr_reader)

			err = chain.Start()
			start := time.Now()
			c.Expect(err, gs.IsNil)
			err = chain.Wait()
			c.Expect(err, gs.Not(gs.IsNil))

			end := time.Now()
			actual_duration := end.Sub(start)
			c.Expect(strings.Contains(err.Error(), "timeout error:"), gs.Equals, true)
			c.Expect(actual_duration >= timeout, gs.Equals, true)
		})

		c.Specify("will stop chains before timeout has completed", func() {
			// This test assumes tail and grep
			var err error

			timeout := time.Second * 5
			chain := &CommandChain{timeout_duration: timeout}
			chain.Init()
			// tail -f will never terminate
			chain.AddStep("tail", "-f", PIPE_TEST_FILE)
			chain.AddStep("grep", "-i", "TEST")

			stdout_reader, err := chain.StdoutPipe()
			c.Expect(err, gs.IsNil)

			stderr_reader, err := chain.StderrPipe()
			c.Expect(err, gs.IsNil)

			go Readoutput(stdout_reader)
			go Readoutput(stderr_reader)

			err = chain.Start()
			start := time.Now()
			c.Expect(err, gs.IsNil)
			time.Sleep(time.Second * 1)

			chain.Stopchan <- true
			err = chain.Wait()
			end := time.Now()
			actual_duration := end.Sub(start)
			c.Expect(err, gs.Not(gs.IsNil))
			c.Expect(actual_duration < timeout, gs.Equals, true)
		})

		c.Specify("can reset chains to run again", func() {
			// This test assumes tail and grep
			var err error

			chain := &CommandChain{timeout_duration: 0}
			chain.Init()
			chain.AddStep("cat", PIPE_TEST_FILE)
			chain.AddStep("grep", "-i", "TEST")

			stdout_reader, err := chain.StdoutPipe()
			c.Expect(err, gs.IsNil)

			stderr_reader, err := chain.StderrPipe()
			c.Expect(err, gs.IsNil)

			outChan := make(chan string, 1)
			errChan := make(chan string, 1)

			go func() {
				outChan <- Readoutput(stdout_reader)
			}()
			go func() {
				errChan <- Readoutput(stderr_reader)
			}()

			err = chain.Start()
			c.Expect(err, gs.IsNil)
			err = chain.Wait()
			c.Expect(err, gs.IsNil)

			c.Expect(<-errChan, gs.Equals, "")
			c.Expect(<-outChan, gs.Equals, "this|is|a|test|\n")

			// Reset the chain for a second run
			chain.reset()

			stdout_reader, err = chain.StdoutPipe()
			c.Expect(err, gs.IsNil)
			stderr_reader, err = chain.StderrPipe()
			c.Expect(err, gs.IsNil)

			go func() {
				outChan <- Readoutput(stdout_reader)
			}()
			go func() {
				errChan <- Readoutput(stderr_reader)
			}()

			err = chain.Start()
			c.Expect(err, gs.IsNil)
			err = chain.Wait()
			c.Expect(err, gs.IsNil)

			c.Expect(<-errChan, gs.Equals, "")
			c.Expect(<-outChan, gs.Equals, "this|is|a|test|\n")
		})

	})
}
