/***** BEGIN LICENSE BLOCK *****
# This Source Code Form is subject to the terms of the Mozilla Public
# License, v. 2.0. If a copy of the MPL was not distributed with this file,
# You can obtain one at http://mozilla.org/MPL/2.0/.
#
# The Initial Developer of the Original Code is the Mozilla Foundation.
# Portions created by the Initial Developer are Copyright (C) 2012-2014
# the Initial Developer. All Rights Reserved.
#
# Contributor(s):
#   Victor Ng (vng@mozilla.com)
#   Rob Miller (rmiller@mozilla.com)
#
# ***** END LICENSE BLOCK *****/

package process

import (
	"bytes"
	"fmt"
	gs "github.com/rafrombrc/gospec/src/gospec"
	"io"
	"io/ioutil"
	"strings"
	"time"
)

func ProcessChainSpec(c gs.Context) {

	readCommandOutput := func(reader io.Reader, resultChan chan string) {
		data, err := ioutil.ReadAll(reader)
		if err != nil {
			resultChan <- fmt.Sprintf("TESTERROR: %s", err.Error())
		} else {
			resultChan <- string(data)
		}
	}

	c.Specify("A ManagedCommand", func() {

		c.Specify("can run a single command", func() {
			Path := SINGLE_CMD
			cmd := NewManagedCmd(Path, SINGLE_CMD_ARGS, 0)
			output := make(chan string)
			cmd.Start(true)
			go readCommandOutput(cmd.Stdout_r, output)
			cmd.Wait()
			outputStr := <-output
			c.Expect(fmt.Sprintf("%x", outputStr), gs.Equals,
				fmt.Sprintf("%x", SINGLE_CMD_OUTPUT))
		})

		c.Specify("honors nonzero timeouts", func() {
			// Timeout should always occur inside of 10 seconds.
			Path := NONZERO_TIMEOUT_CMD
			timeout := NONZERO_TIMEOUT
			cmd := NewManagedCmd(Path, NONZERO_TIMEOUT_ARGS, timeout)
			output := make(chan string)
			cmd.Start(true)
			start := time.Now()
			go readCommandOutput(cmd.Stdout_r, output)
			cmd.Wait()
			end := time.Now()
			outputStr := <-output
			c.Expect(strings.HasPrefix(outputStr, "TESTERROR"), gs.IsFalse)
			actualDuration := end.Sub(start)
			c.Expect(actualDuration < time.Second*10, gs.Equals, true)
		})

		c.Specify("reads process stderr properly", func() {
			Path := STDERR_CMD
			cmd := NewManagedCmd(Path, STDERR_CMD_ARGS, 0)

			stdoutResults := make(chan string, 1)
			stderrResults := make(chan string, 1)

			cmd.Start(true)
			go readCommandOutput(cmd.Stdout_r, stdoutResults)
			go readCommandOutput(cmd.Stderr_r, stderrResults)
			cmd.Wait()

			// Error messages will vary platform to platform, just check that
			// there is some message which will be about "No such file found".
			errOutput := <-stderrResults
			c.Expect(len(errOutput) > 0, gs.Equals, true)
			c.Expect(strings.HasPrefix(errOutput, "TESTERROR"), gs.IsFalse)
			c.Expect(<-stdoutResults, gs.Equals, "")
		})

		c.Specify("can be terminated before timeout occurs", func() {
			Path := NONZERO_TIMEOUT_CMD
			timeout := time.Second * 30
			cmd := NewManagedCmd(Path, NONZERO_TIMEOUT_ARGS, timeout)

			stdoutResults := make(chan string, 1)

			cmd.Start(true)
			start := time.Now()
			go readCommandOutput(cmd.Stdout_r, stdoutResults)
			time.Sleep(NONZERO_TIMEOUT)
			cmd.Stopchan <- true
			cmd.Wait()
			end := time.Now()

			actualDuration := end.Sub(start)
			c.Expect(actualDuration < timeout, gs.Equals, true)
		})

		c.Specify("can reset commands to run again", func() {
			Path := SINGLE_CMD
			cmd := NewManagedCmd(Path, SINGLE_CMD_ARGS, 0)

			stdoutResults := make(chan string, 1)

			cmd.Start(true)
			go readCommandOutput(cmd.Stdout_r, stdoutResults)
			cmd.Wait()
			c.Expect(<-stdoutResults, gs.Equals, SINGLE_CMD_OUTPUT)

			// Reset and rerun it.
			cmd = cmd.clone()
			cmd.Start(true)
			go readCommandOutput(cmd.Stdout_r, stdoutResults)
			cmd.Wait()
			c.Expect(<-stdoutResults, gs.Equals, SINGLE_CMD_OUTPUT)
		})
	})

	c.Specify("A New ProcessChain", func() {
		c.Specify("pipes multiple commands correctly", func() {
			chain := NewCommandChain(0)
			chain.AddStep(PIPE_CMD1, PIPE_CMD1_ARGS...)
			chain.AddStep(PIPE_CMD2, PIPE_CMD2_ARGS...)

			err := chain.Start()
			c.Expect(err, gs.IsNil)

			stdoutReader, err := chain.Stdout_r()
			c.Expect(err, gs.IsNil)
			stdoutResult := make(chan string, 1)

			stderrReader, err := chain.Stderr_r()
			c.Expect(err, gs.IsNil)
			stderrResult := make(chan string, 1)

			go readCommandOutput(stdoutReader, stdoutResult)
			go readCommandOutput(stderrReader, stderrResult)

			cc := chain.Wait()
			c.Expect(cc.SubcmdErrors, gs.IsNil)

			c.Expect(<-stderrResult, gs.Equals, "")
			c.Expect(<-stdoutResult, gs.Equals, PIPE_CMD_OUTPUT)

		})

		c.Specify("will honor timeouts", func() {
			// This test must timeout within 10 seconds.
			var err error

			timeout := NONZERO_TIMEOUT
			chain := NewCommandChain(timeout)
			chain.AddStep(TIMEOUT_PIPE_CMD1, TIMEOUT_PIPE_CMD1_ARGS...)
			chain.AddStep(TIMEOUT_PIPE_CMD2, TIMEOUT_PIPE_CMD2_ARGS...)

			err = chain.Start()
			start := time.Now()
			c.Expect(err, gs.IsNil)
			cc := chain.Wait()
			end := time.Now()
			actual_duration := end.Sub(start)
			c.Expect(cc.SubcmdErrors, gs.Not(gs.IsNil))
			c.Expect(strings.Contains(cc.SubcmdErrors.Error(), "was killed"), gs.Equals, true)
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
			cc := chain.Wait()
			end := time.Now()
			actual_duration := end.Sub(start)
			c.Expect(cc.SubcmdErrors, gs.Not(gs.IsNil))
			c.Expect(actual_duration < timeout, gs.Equals, true)
		})

		c.Specify("can reset chains to run again", func() {
			// This test assumes tail and grep
			var err error

			chain := NewCommandChain(0)
			chain.AddStep(PIPE_CMD1, PIPE_CMD1_ARGS...)
			chain.AddStep(PIPE_CMD2, PIPE_CMD2_ARGS...)

			err = chain.Start()
			c.Expect(err, gs.IsNil)

			stdoutReader, err := chain.Stdout_r()
			c.Expect(err, gs.IsNil)
			stdoutResult := make(chan string, 1)

			stderrReader, err := chain.Stderr_r()
			c.Expect(err, gs.IsNil)
			stderrResult := make(chan string, 1)

			go readCommandOutput(stdoutReader, stdoutResult)
			go readCommandOutput(stderrReader, stderrResult)

			cc := chain.Wait()
			c.Expect(cc.SubcmdErrors, gs.IsNil)

			c.Expect(<-stderrResult, gs.Equals, "")
			c.Expect(<-stdoutResult, gs.Equals, PIPE_CMD_OUTPUT)

			// Reset the chain for a second run
			chain = chain.clone()

			err = chain.Start()
			c.Expect(err, gs.IsNil)

			stdoutReader, err = chain.Stdout_r()
			c.Expect(err, gs.IsNil)

			stderrReader, err = chain.Stderr_r()
			c.Expect(err, gs.IsNil)

			go readCommandOutput(stdoutReader, stdoutResult)
			go readCommandOutput(stderrReader, stderrResult)

			cc = chain.Wait()
			c.Expect(cc.SubcmdErrors, gs.IsNil)

			c.Expect(<-stderrResult, gs.Equals, "")
			c.Expect(<-stdoutResult, gs.Equals, PIPE_CMD_OUTPUT)
		})
	})
}

func readoutput(r io.Reader) string {
	buf := new(bytes.Buffer)
	buf.ReadFrom(r)
	s := buf.String()
	return s
}
