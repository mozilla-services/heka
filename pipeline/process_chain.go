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
#   Bryan Zubrod (bzubrod@gmail.com)
#   Victor Ng (vng@mozilla.com)
#
#***** END LICENSE BLOCK *****/

package pipeline

import (
	"fmt"
	"io"
	"os/exec"
	"strings"
	"time"
)

// ManagedCmd extends exec.Cmd to support killing of a subprocess if
// a timeout has been exceeded.  A timeout duration value of 0
// indicates that no timeout is enforced.
type ManagedCmd struct {
	exec.Cmd

	Path string
	Args []string
	// Env specifies the environment of the process.
	// If Env is nil, Run uses the current process's environment.
	Env []string

	// Dir specifies the working directory of the command.
	// If Dir is the empty string, Run runs the command in the
	// calling process's current directory.
	Dir string

	done     chan error
	Stopchan chan bool

	// Note that the timeout duration is only used when Wait() is called.
	// If you put this command on a run interval where the interval time is
	// very close to the timeout interval, it is possible that the
	// timeout may only occur *after* the command has been restarted.
	timeout_duration time.Duration
}

func (mc *ManagedCmd) Init() (err error) {
	mc.done = make(chan error)
	mc.Stopchan = make(chan bool, 1)
	mc.Cmd = *exec.Command(mc.Path, mc.Args...)
	mc.Cmd.Env = mc.Env
	mc.Cmd.Dir = mc.Dir
	return nil
}

// We overload the Wait() method to enable subprocess termination if a
// timeout has been exceeded.
func (mc *ManagedCmd) Wait() (err error) {
	go func() {
		// Note that this will close the pipe on StdoutPipe()
		// as documented in :
		// http://code.google.com/p/go/issues/detail?id=2266
		// http://golang.org/pkg/os/exec/#Cmd.StdoutPipe
		mc.done <- mc.Cmd.Wait()
	}()

	if mc.timeout_duration != 0 {
		select {
		case <-mc.Stopchan:
			return fmt.Errorf("CommandChain was stopped with error: [%s]", mc.kill())
		case <-time.After(mc.timeout_duration):
			return fmt.Errorf("CommandChain timedout with error: [%s]", mc.kill())
		case err = <-mc.done:
			return err
		}
	} else {
		err = <-mc.done
		return err
	}
	return nil
}

// Kill the current process. This will always return an error code.
func (mc *ManagedCmd) kill() (err error) {
	if err := mc.Process.Kill(); err != nil {
		return fmt.Errorf("failed to kill subprocess: %s", err.Error())
	}
	// killing process will make Wait() return
	<-mc.done
	return fmt.Errorf("subprocess was killed: [%s %s]", mc.Path, strings.Join(mc.Args, " "))
}

// This resets a command so that we can run the command again.
// Usually so that a chain can be restarted.
func (mc *ManagedCmd) reset() {
	mc.Cmd = exec.Cmd{
		Path: mc.Cmd.Path,
		Args: mc.Cmd.Args,
		Env:  mc.Cmd.Env,
		Dir:  mc.Cmd.Dir,
	}

	mc.done = make(chan error)
	mc.Stopchan = make(chan bool, 1)
}

// A CommandChain lets you execute an ordered set of subprocesses
// and pipe stdout to stdin for each stage.
type CommandChain struct {
	Cmds []*ManagedCmd

	// The timeout duration is the maximum time that each stage of the
	// pipeline should run for before the Wait() returns a
	// timeout error.
	timeout_duration time.Duration

	done     chan error
	Stopchan chan bool
}

func (cc *CommandChain) Init() {
	cc.done = make(chan error)
	cc.Stopchan = make(chan bool, 1)
}

// Add a single command to our command chain, piping stdout to stdin
// for each stage.
func (cc *CommandChain) AddStep(Path string, Args ...string) (cmd *ManagedCmd) {
	cmd = &ManagedCmd{
		Path:             Path,
		Args:             Args,
		timeout_duration: cc.timeout_duration,
	}
	cmd.Init()

	cc.Cmds = append(cc.Cmds, cmd)
	if len(cc.Cmds) > 1 {
		r, w := io.Pipe()
		cc.Cmds[len(cc.Cmds)-2].Stdout = w
		cc.Cmds[len(cc.Cmds)-1].Stdin = r
	}
	return cmd
}

func (cc *CommandChain) StdoutPipe() (r io.ReadCloser, err error) {
	if len(cc.Cmds) == 0 {
		return nil, fmt.Errorf("No commands are in this chain")
	}
	return cc.Cmds[len(cc.Cmds)-1].StdoutPipe()
}

func (cc *CommandChain) StderrPipe() (r io.ReadCloser, err error) {
	if len(cc.Cmds) == 0 {
		return nil, fmt.Errorf("No commands are in this chain")
	}
	return cc.Cmds[len(cc.Cmds)-1].StderrPipe()
}

func (cc *CommandChain) Start() (err error) {
	/* This is a bit subtle.  You want to spin up all the commands in
	   order by calling Start().  */

	for _, cmd := range cc.Cmds {
		err = cmd.Start()
		if err != nil {
			return fmt.Errorf("Command [%s %s] triggered an error: [%s]", cmd.Path, strings.Join(cmd.Args, " "), err.Error())
		}
	}
	return nil
}

func (cc *CommandChain) Wait() (err error) {
	/* You need to Wait and close the stdout for each
	   stage in order, except that you do *not* want to close the last
	   output pipe as we need to use that to get the final results.  */
	go func() {
		for i, cmd := range cc.Cmds {
			err = cmd.Wait()
			if err != nil {
				cc.done <- err
				return
			}
			if i < (len(cc.Cmds) - 1) {
				err = cmd.Stdout.(*io.PipeWriter).Close()
				if err != nil {
					cc.done <- err
					return
				}
			}
		}

		cc.done <- nil
	}()

	select {
	case err = <-cc.done:
		return err
	case <-cc.Stopchan:
		for _, cmd := range cc.Cmds {
			cmd.Stopchan <- true
		}
		return fmt.Errorf("Chain stopped")
	}
	return nil
}

// This resets a command chain so that we can rereun
func (cc *CommandChain) reset() {
	for i, cmd := range cc.Cmds {
		cmd.reset()

		if i > 0 {
			// Reconnect the previous command's stdout to this
			// command's stdin
			r, w := io.Pipe()
			cc.Cmds[i-1].Stdout = w
			cmd.Stdin = r
		}
	}
	cc.done = make(chan error)
	cc.Stopchan = make(chan bool, 1)
}
