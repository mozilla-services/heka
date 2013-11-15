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
	"fmt"
	gs "github.com/rafrombrc/gospec/src/gospec"
	"io"
	"strings"
	"time"
)

func readCommandOutput(output_chan chan string, result_chan chan string) {
	txt := ""
	for {
		select {
		case data := <-output_chan:
			if len(data) > 0 {
				txt += data
			} else {
				result_chan <- txt
				return
			}
		}
	}
}

func ProcessChainSpec(c gs.Context) {

	c.Specify("A ManagedCommand", func() {
		c.Specify("can run a single command", func() {
			Path := SINGLE_CMD
			cmd := NewManagedCmd(Path, SINGLE_CMD_ARGS, 0)

			output_chan := cmd.StdoutChan()
			output := make(chan string)
			go readCommandOutput(output_chan, output)
			cmd.Start(true)
			cmd.Wait()
			output_str := <-output
			c.Expect(fmt.Sprintf("%x", output_str), gs.Equals, fmt.Sprintf("%x", SINGLE_CMD_OUTPUT))
		})

		c.Specify("honors nonzero timeouts", func() {
			// Timeout should always occur inside of 10 seconds.
			Path := NONZERO_TIMEOUT_CMD

			timeout := NONZERO_TIMEOUT
			cmd := NewManagedCmd(Path, NONZERO_TIMEOUT_ARGS, timeout)

			output_chan := cmd.StdoutChan()
			output := make(chan string)

			go readCommandOutput(output_chan, output)

			cmd.Start(true)
			start := time.Now()
			cmd.Wait()
			end := time.Now()
			<-output

			actual_duration := end.Sub(start)
			c.Expect(actual_duration < time.Second*10, gs.Equals, true)
		})

		c.Specify("reads process stderr properly", func() {
			Path := STDERR_CMD
			cmd := NewManagedCmd(Path, STDERR_CMD_ARGS, 0)

			stdout_chan := cmd.StdoutChan()
			stdout_results := make(chan string, 1)

			stderr_chan := cmd.StderrChan()
			stderr_results := make(chan string, 1)

			go readCommandOutput(stdout_chan, stdout_results)
			go readCommandOutput(stderr_chan, stderr_results)

			cmd.Start(true)
			cmd.Wait()
			// stderr messages will vary platform to platform, just check that there is some
			// message which will be about "No such file found"
			c.Expect(len(<-stderr_results) > 0, gs.Equals, true)
			c.Expect(<-stdout_results, gs.Equals, "")
		})

		c.Specify("can be terminated before timeout occurs", func() {
			Path := NONZERO_TIMEOUT_CMD
			timeout := time.Second * 30
			cmd := NewManagedCmd(Path, NONZERO_TIMEOUT_ARGS, timeout)

			stdout_chan := cmd.StdoutChan()
			stdout_results := make(chan string, 1)

			go readCommandOutput(stdout_chan, stdout_results)

			cmd.Start(true)
			start := time.Now()
			time.Sleep(NONZERO_TIMEOUT)
			cmd.Stopchan <- true
			cmd.Wait()
			end := time.Now()
			actual_duration := end.Sub(start)
			c.Expect(actual_duration < timeout, gs.Equals, true)
		})

		c.Specify("can reset commands to run again", func() {
			Path := SINGLE_CMD
			cmd := NewManagedCmd(Path, SINGLE_CMD_ARGS, 0)

			stdout_chan := cmd.StdoutChan()
			stdout_results := make(chan string, 1)

			go readCommandOutput(stdout_chan, stdout_results)

			cmd.Start(true)
			cmd.Wait()
			c.Expect(<-stdout_results, gs.Equals, SINGLE_CMD_OUTPUT)

			// Reset and rerun it
			cmd = cmd.clone()

			stdout_chan = cmd.StdoutChan()
			stdout_results = make(chan string, 1)

			go readCommandOutput(stdout_chan, stdout_results)

			cmd.Start(true)
			cmd.Wait()
			c.Expect(<-stdout_results, gs.Equals, SINGLE_CMD_OUTPUT)
		})
	})

	c.Specify("A New ProcessChain", func() {
		c.Specify("can pipe cat and grep", func() {
			// This test assumes cat and grep
			var err error

			chain := NewCommandChain(0)
			chain.AddStep(PIPE_CMD1, PIPE_CMD1_ARGS...)
			chain.AddStep(PIPE_CMD2, PIPE_CMD2_ARGS...)

			stdout_chan, err := chain.StdoutChan()
			c.Expect(err, gs.IsNil)
			stdout_result := make(chan string, 1)

			stderr_chan, err := chain.StderrChan()
			c.Expect(err, gs.IsNil)
			stderr_result := make(chan string, 1)

			go readCommandOutput(stdout_chan, stdout_result)
			go readCommandOutput(stderr_chan, stderr_result)

			err = chain.Start()
			c.Expect(err, gs.IsNil)
			err = chain.Wait()
			c.Expect(err, gs.IsNil)

			c.Expect(<-stderr_result, gs.Equals, "")
			c.Expect(<-stdout_result, gs.Equals, PIPE_CMD_OUTPUT)

		})

		c.Specify("will honor timeouts", func() {
			// This test must timeout within 10 seconds
			var err error

			timeout := NONZERO_TIMEOUT
			chain := NewCommandChain(timeout)
			chain.AddStep(TIMEOUT_PIPE_CMD1, TIMEOUT_PIPE_CMD1_ARGS...)
			chain.AddStep(TIMEOUT_PIPE_CMD2, TIMEOUT_PIPE_CMD2_ARGS...)

			err = chain.Start()
			start := time.Now()
			c.Expect(err, gs.IsNil)
			err = chain.Wait()
			end := time.Now()
			actual_duration := end.Sub(start)
			c.Expect(err, gs.Not(gs.IsNil))
			c.Expect(strings.Contains(err.Error(), "was killed"), gs.Equals, true)
			c.Expect(actual_duration < time.Second*10, gs.Equals, true)
		})

		c.Specify("will stop chains before timeout has completed", func() {
			var err error

			timeout := time.Second * 30
			chain := NewCommandChain(timeout)
			chain.AddStep(TIMEOUT_PIPE_CMD1, TIMEOUT_PIPE_CMD1_ARGS...)
			chain.AddStep(TIMEOUT_PIPE_CMD2, TIMEOUT_PIPE_CMD2_ARGS...)

			err = chain.Start()
			start := time.Now()
			c.Expect(err, gs.IsNil)
			time.Sleep(NONZERO_TIMEOUT)

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

			chain := NewCommandChain(0)
			chain.AddStep(PIPE_CMD1, PIPE_CMD1_ARGS...)
			chain.AddStep(PIPE_CMD2, PIPE_CMD2_ARGS...)

			stdout_chan, err := chain.StdoutChan()
			c.Expect(err, gs.IsNil)
			stdout_result := make(chan string, 1)

			stderr_chan, err := chain.StderrChan()
			c.Expect(err, gs.IsNil)
			stderr_result := make(chan string, 1)

			go readCommandOutput(stdout_chan, stdout_result)
			go readCommandOutput(stderr_chan, stderr_result)

			err = chain.Start()
			c.Expect(err, gs.IsNil)
			err = chain.Wait()
			c.Expect(err, gs.IsNil)

			c.Expect(<-stderr_result, gs.Equals, "")
			c.Expect(<-stdout_result, gs.Equals, PIPE_CMD_OUTPUT)

			// Reset the chain for a second run
			chain = chain.clone()

			stdout_chan, err = chain.StdoutChan()
			c.Expect(err, gs.IsNil)
			stdout_result = make(chan string, 1)

			stderr_chan, err = chain.StderrChan()
			c.Expect(err, gs.IsNil)
			stderr_result = make(chan string, 1)

			go readCommandOutput(stdout_chan, stdout_result)
			go readCommandOutput(stderr_chan, stderr_result)

			err = chain.Start()
			c.Expect(err, gs.IsNil)
			err = chain.Wait()
			c.Expect(err, gs.IsNil)

			c.Expect(<-stderr_result, gs.Equals, "")
			c.Expect(<-stdout_result, gs.Equals, PIPE_CMD_OUTPUT)
		})

		c.Specify("can be used as a reader", func() {

			// This test assumes cat and grep
			var err error

			chain := NewCommandChain(0)
			chain.AddStep(PIPE_CMD1, PIPE_CMD1_ARGS...)
			chain.AddStep(PIPE_CMD2, PIPE_CMD2_ARGS...)

			stdout_chan, err := chain.StdoutChan()
			c.Expect(err, gs.IsNil)
			reader := &StringChannelReader{input: stdout_chan}

			read_channel := make(chan string)
			go func() {
				read_channel <- readoutput(reader)
			}()

			err = chain.Start()
			c.Expect(err, gs.IsNil)
			err = chain.Wait()
			c.Expect(err, gs.IsNil)

			c.Expect(<-read_channel, gs.Equals, PIPE_CMD_OUTPUT)
		})
	})

}

func readoutput(r io.Reader) string {
	buf := new(bytes.Buffer)
	buf.ReadFrom(r)
	s := buf.String()
	return s
}
