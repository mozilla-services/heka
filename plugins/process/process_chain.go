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
#   Bryan Zubrod (bzubrod@gmail.com)
#   Victor Ng (vng@mozilla.com)
#   Rob Miller (rmiller@mozilla.com)
#
#***** END LICENSE BLOCK *****/

package process

import (
	"fmt"
	"io"
	"os/exec"
	"strings"
	"time"
)

// ManagedCmd extends exec.Cmd to support killing of a subprocess if a timeout
// has been exceeded. A timeout duration value of 0 indicates that no timeout
// is enforced.
type ManagedCmd struct {
	*exec.Cmd

	done     chan error
	Stopchan chan bool

	// Note that the timeout duration is only used when Wait() is called. If
	// you put this command on a run interval where the interval time is very
	// close to the timeout interval, it is possible that the timeout may only
	// occur *after* the command has been restarted.
	timeout_duration time.Duration

	Stdout_r *io.PipeReader
	Stderr_r *io.PipeReader
}

func NewManagedCmd(path string, args []string, timeout time.Duration) (mc *ManagedCmd) {
	mc = &ManagedCmd{timeout_duration: timeout}
	mc.done = make(chan error)
	mc.Stopchan = make(chan bool, 1)
	mc.Cmd = exec.Command(path, args...)
	return mc
}

func (mc *ManagedCmd) Start(pipeOutput bool) (err error) {
	if pipeOutput {
		mc.Stdout_r, mc.Stdout = io.Pipe()
		mc.Stderr_r, mc.Stderr = io.Pipe()
	}
	return mc.Cmd.Start()
}

// We overload the Wait() method to enable subprocess termination if a
// timeout has been exceeded.
func (mc *ManagedCmd) Wait() (err error) {
	go func() {
		mc.done <- mc.Cmd.Wait()
	}()

	done := false
	if mc.timeout_duration != 0 {
		for !done {
			select {
			case <-mc.Stopchan:
				err = fmt.Errorf("ManagedCmd was stopped with error: [%s]", mc.kill())
				done = true
			case <-time.After(mc.timeout_duration):
				mc.Stopchan <- true
				err = fmt.Errorf("ManagedCmd timedout")
			case err = <-mc.done:
				done = true
			}
		}
	} else {
		select {
		case <-mc.Stopchan:
			err = fmt.Errorf("ManagedCmd was stopped with error: [%s]", mc.kill())
		case err = <-mc.done:
		}
	}

	var writer *io.PipeWriter
	var ok bool

	writer, ok = mc.Stdout.(*io.PipeWriter)
	if ok {
		writer.Close()
	}
	writer, ok = mc.Stderr.(*io.PipeWriter)
	if ok {
		writer.Close()
	}

	return err
}

// Kill the current process. This will always return an error code.
func (mc *ManagedCmd) kill() (err error) {
	if err := mc.Process.Kill(); err != nil {
		return fmt.Errorf("failed to kill subprocess: %s", err.Error())
	}
	// killing process will make Wait() return
	<-mc.done
	return fmt.Errorf("subprocess was killed: [%s]", strings.Join(mc.Args, " "))
}

// This resets a command so that we can run the command again. Usually so that
// a chain can be restarted.
func (mc *ManagedCmd) clone() (clone *ManagedCmd) {
	// mc.Args[0] should always be == mc.Path, so mc.Args[1:] should be safe
	// to use here.
	clone = NewManagedCmd(mc.Path, mc.Args[1:], mc.timeout_duration)
	clone.Env = mc.Env
	clone.Dir = mc.Dir
	return clone
}

// A CommandChain lets you execute an ordered set of subprocesses and pipe
// stdout to stdin for each stage.
type CommandChain struct {
	Cmds []*ManagedCmd

	// The timeout duration is the maximum time that each stage of the
	// pipeline should run for before the Wait() returns a timeout error.
	timeout_duration time.Duration

	done     chan CommandChainStatus
	Stopchan chan bool
}

// A CommandChainStatus records the return execution result of a command chain.
// ReturnStatus stores the return status of the command chain, which is the
// return status of the last successfully executed command.
// SubcmdErrors stores the errors of each subcommand.
type CommandChainStatus struct {
	ExitStatus   error
	SubcmdErrors error
}

func NewCommandChain(timeout time.Duration) (cc *CommandChain) {
	cc = &CommandChain{timeout_duration: timeout}
	cc.done = make(chan CommandChainStatus)
	cc.Stopchan = make(chan bool, 1)
	return cc
}

// Add A single command to our command chain, piping stdout to stdin for each
// stage.
func (cc *CommandChain) AddStep(Path string, Args ...string) (cmd *ManagedCmd) {
	cmd = NewManagedCmd(Path, Args, cc.timeout_duration)

	cc.Cmds = append(cc.Cmds, cmd)
	if len(cc.Cmds) > 1 {
		r, w := io.Pipe()
		cc.Cmds[len(cc.Cmds)-2].Stdout = w
		cc.Cmds[len(cc.Cmds)-1].Stdin = r
	}
	return cmd
}

func (cc *CommandChain) Stdout_r() (stdout io.Reader, err error) {
	if len(cc.Cmds) == 0 {
		return nil, fmt.Errorf("No commands are in this chain")
	}
	return cc.Cmds[len(cc.Cmds)-1].Stdout_r, nil
}

func (cc *CommandChain) Stderr_r() (stderr io.Reader, err error) {
	if len(cc.Cmds) == 0 {
		return nil, fmt.Errorf("No commands are in this chain")
	}
	return cc.Cmds[len(cc.Cmds)-1].Stderr_r, nil
}

func (cc *CommandChain) Start() (err error) {
	/* This is a bit subtle.  You want to spin up all the commands in
	   order by calling Start().  */

	for idx, cmd := range cc.Cmds {
		if idx == (len(cc.Cmds) - 1) {
			err = cmd.Start(true)
		} else {
			err = cmd.Start(false)
		}

		if err != nil {
			return fmt.Errorf("Command [%s] triggered an error: [%s]",
				strings.Join(cmd.Args, " "),
				err.Error())
		}
	}
	return nil
}

func (cc *CommandChain) Wait() (cc_status CommandChainStatus) {
	/* You need to Wait and close the stdout for each
	   stage in order, except that you do *not* want to close the last
	   output pipe as we need to use that to get the final results.  */
	go func() {
		var subcmd_err error
		var cc_status CommandChainStatus
		subcmd_errors := make([]string, 0)

		for i, cmd := range cc.Cmds {
			subcmd_err = cmd.Wait()
			cc_status.ExitStatus = subcmd_err

			if subcmd_err != nil {
				subcmd_errors = append(subcmd_errors,
					fmt.Sprintf("Subcommand[%d] returned an error: [%s]", i, subcmd_err.Error()))
			}
			if i < (len(cc.Cmds) - 1) {
				subcmd_err = cmd.Stdout.(*io.PipeWriter).Close()
				if subcmd_err != nil {
					subcmd_errors = append(subcmd_errors,
						fmt.Sprintf("Pipewriter close error: [%s]\n", subcmd_err.Error()))
				}
			}
		}
		if len(subcmd_errors) > 0 {
			cc_status.SubcmdErrors = fmt.Errorf(strings.Join(subcmd_errors, "\n"))
			cc.done <- cc_status
		} else {
			cc.done <- cc_status
		}
	}()

	done := false
	for !done {
		select {
		case cc_status = <-cc.done:
			done = true
		case <-cc.Stopchan:
			for i := 0; i < len(cc.Cmds); i++ {
				cmd := cc.Cmds[i]
				cmd.Stopchan <- true
			}
		}
	}
	return cc_status
}

// This resets a command so that we can run the command again.
// Usually so that a chain can be restarted.
func (cc *CommandChain) clone() (clone *CommandChain) {
	clone = NewCommandChain(cc.timeout_duration)
	for _, orig := range cc.Cmds {
		// mc.Args[0] should always be == mc.Path, so mc.Args[1:] should be
		// safe to use here.
		cmd := clone.AddStep(orig.Path, orig.Args[1:]...)
		cmd.Env = orig.Env
		cmd.Dir = orig.Dir
	}
	return clone
}
