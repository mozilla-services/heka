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
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/mozilla-services/heka/message"
	. "github.com/mozilla-services/heka/pipeline"
)

type cmdConfig struct {
	// Path to executable file.
	Bin string

	// Command arguments.
	Args []string

	// Environment variables.
	Env []string

	// Dir specifies the working directory of Command.  Defaults to the
	// directory where the program resides.
	Directory string
}

// Helper function for manually comparing structs since slice attributes mean
// we can't use `==`.
func (c *cmdConfig) Equals(otherC *cmdConfig) bool {
	if c.Bin != otherC.Bin {
		return false
	}
	if c.Directory != otherC.Directory {
		return false
	}
	if len(c.Args) != len(otherC.Args) {
		return false
	}
	for i, v := range c.Args {
		if otherC.Args[i] != v {
			return false
		}
	}
	if len(c.Env) != len(otherC.Env) {
		return false
	}
	for i, v := range c.Env {
		if otherC.Env[i] != v {
			return false
		}
	}
	return true
}

type ProcessInputConfig struct {
	// Command(s) to run. If multiple commands are specified they will be run
	// in the order specified, and the standard output stream will be piped to
	// the standard input of the next command.
	Command map[string]cmdConfig

	// Number of seconds to wait between runnning command(s).
	TickerInterval uint `toml:"ticker_interval"`

	// Skips wait
	ImmediateStart bool `toml:"immediate_start"`

	// Timeout in seconds.
	TimeoutSeconds uint `toml:"timeout"`

	ParseStdout bool `toml:"stdout"`
	ParseStderr bool `toml:"stderr"`
}

// Helper function for manually comparing structs since a map attribute means
// we can't use `==`.
func (pic *ProcessInputConfig) Equals(otherPic *ProcessInputConfig) bool {
	if pic.TickerInterval != otherPic.TickerInterval {
		return false
	}
	if pic.TimeoutSeconds != otherPic.TimeoutSeconds {
		return false
	}
	if pic.ParseStdout != otherPic.ParseStdout {
		return false
	}
	if pic.ParseStderr != otherPic.ParseStderr {
		return false
	}
	if len(pic.Command) != len(otherPic.Command) {
		return false
	}
	for k, v := range pic.Command {
		cmd := &v
		otherV, ok := otherPic.Command[k]
		if !ok {
			return false
		}
		if !cmd.Equals(&otherV) {
			return false
		}
	}
	return true
}

// Heka Input plugin that runs external programs and processes their
// output as a stream into Message objects to be passed into
// the Router for delivery to matching Filter or Output plugins.
type ProcessInput struct {
	ProcessName string
	cc          *CommandChain
	ir          InputRunner

	parseStdout bool
	parseStderr bool

	stdoutDeliverer Deliverer
	stdoutSRunner   SplitterRunner
	stderrDeliverer Deliverer
	stderrSRunner   SplitterRunner

	stopChan  chan bool
	exitError error
	ccStatus  CommandChainStatus

	hostname       string
	hekaPid        int32
	tickInterval   uint
	immediateStart bool

	once sync.Once
}

// ConfigStruct implements the HasConfigStruct interface and sets
// defaults.
func (pi *ProcessInput) ConfigStruct() interface{} {
	return &ProcessInputConfig{
		TickerInterval: uint(15),
		ImmediateStart: false,
		ParseStdout:    true,
		ParseStderr:    false,
	}
}

// Init implements the Plugin interface.
func (pi *ProcessInput) Init(config interface{}) (err error) {
	conf := config.(*ProcessInputConfig)

	pi.tickInterval = conf.TickerInterval
	pi.immediateStart = conf.ImmediateStart
	pi.parseStdout = conf.ParseStdout
	pi.parseStderr = conf.ParseStderr

	if len(conf.Command) < 1 {
		return fmt.Errorf("No Command Configured")
	}

	pi.cc = NewCommandChain(time.Duration(conf.TimeoutSeconds) * time.Second)

	// We need to mangle the indexes to be integers
	for idx := 0; idx < len(conf.Command); idx++ {
		str_idx := strconv.Itoa(idx)
		cmdCfg, ok := conf.Command[str_idx]
		if !ok {
			return fmt.Errorf("Expected to find a command at index [%s][%d]",
				pi.ProcessName, idx)
		}

		cmd := pi.cc.AddStep(cmdCfg.Bin, cmdCfg.Args...)

		if cmdCfg.Directory != "" {
			cmd.Dir = cmdCfg.Directory
		}
		if cmdCfg.Env != nil {
			cmd.Env = cmdCfg.Env
		}
	}

	pi.hekaPid = int32(os.Getpid())
	return nil
}

func (pi *ProcessInput) SetName(name string) {
	pi.ProcessName = name
}

func (pi *ProcessInput) Run(ir InputRunner, h PluginHelper) error {
	// So we can access our InputRunner outside of the Run function.
	pi.ir = ir
	pi.hostname = h.Hostname()
	pi.stopChan = make(chan bool)
	pi.once = sync.Once{}
	pi.exitError = nil
	if pi.parseStdout {
		pi.stdoutDeliverer, pi.stdoutSRunner = pi.initDelivery("stdout")
		defer func() {
			pi.stdoutDeliverer.Done()
			pi.stdoutSRunner.Done()
		}()
	}

	if pi.parseStderr {
		pi.stderrDeliverer, pi.stderrSRunner = pi.initDelivery("stderr")
		defer func() {
			pi.stderrDeliverer.Done()
			pi.stderrSRunner.Done()
		}()
	}

	// Start the output parser and start running commands.
	go pi.RunCmd()

	// Wait for stop signal.
	<-pi.stopChan

	// If RunCmd exited with an error, and we're not in shutdown, pass back
	// up (to trigger any configured retry behaviour)
	if pi.exitError != nil {
		return pi.exitError
	}

	return nil
}

func (pi *ProcessInput) initDelivery(streamName string) (Deliverer, SplitterRunner) {
	deliverer := pi.ir.NewDeliverer(streamName)
	sRunner := pi.ir.NewSplitterRunner(streamName)
	if !sRunner.UseMsgBytes() {
		packDecorator := func(pack *PipelinePack) {
			pack.Message.SetType("ProcessInput")
			pack.Message.SetPid(pi.hekaPid)
			pack.Message.SetHostname(pi.hostname)
			// Add ProcessInputName
			fPInputName, err := message.NewField("ProcessInputName",
				fmt.Sprintf("%s.%s", pi.ProcessName, streamName), "")
			if err == nil {
				pack.Message.AddField(fPInputName)
			} else {
				pi.ir.LogError(err)
			}
			// Wait for the result for subcommands.
			// Add exit status and subcommand error messages to pack.
			var r int
			if exiterr, ok := pi.ccStatus.ExitStatus.(*exec.ExitError); ok {
				if status, ok := exiterr.Sys().(syscall.WaitStatus); ok {
					r = status.ExitStatus()
				}
			}

			exitStatus, err := message.NewField("ExitStatus", r, "")
			if err == nil {
				pack.Message.AddField(exitStatus)
			} else {
				pi.ir.LogError(err)
			}

			if pi.ccStatus.SubcmdErrors != nil {
				subcmdStatus, err := message.NewField("SubcmdErrors", pi.ccStatus.SubcmdErrors.Error(), "")
				if err == nil {
					pack.Message.AddField(subcmdStatus)
				} else {
					pi.ir.LogError(err)
				}
			}
		}
		sRunner.SetPackDecorator(packDecorator)
	}

	return deliverer, sRunner
}

func (pi *ProcessInput) Stop() {
	// This will also shutdown the ProcessInput::RunCmd goroutine and
	// spawned CmdChain processes
	pi.once.Do(func() {
		pi.cc.Stopchan <- true
		close(pi.stopChan)
	})
}

// RunCmd pipes multiple commands together, runs them per the configured
// msInterval, and passes the output to the appropriate splitter.
func (pi *ProcessInput) RunCmd() {

	if pi.tickInterval == 0 {
		pi.runOnce()
		pi.Stop()
		return
	}

	if pi.immediateStart {
		pi.runOnce()
	}
	tickChan := pi.ir.Ticker()
	for {
		select {
		case <-tickChan:
			// No need to spin up a new goroutine as we've already
			// detached from the main thread.
			pi.cc = pi.cc.clone()
			pi.runOnce()
			if pi.exitError != nil {
				pi.stopChan <- true
				return
			}
		case <-pi.stopChan:
			return
		}
	}
}

func (pi *ProcessInput) runOnce() {
	// Stdout of the last command in the pipe gets sent to provided stdout.
	var err error

	if err = pi.cc.Start(); err != nil {
		pi.exitError = fmt.Errorf("CommandChain::Start() error: [%s]", err)
		return
	}

	// We don't get EOF on the pipe readers unless we drain both the stdout
	// and the stderr pipes.
	throwAway := func(r io.Reader) {
		scratch := make([]byte, 500)
		var e error
		for e == nil {
			_, e = r.Read(scratch)
		}
	}

	var stdoutReader io.Reader
	if stdoutReader, err = pi.cc.Stdout_r(); err != nil {
		pi.exitError = fmt.Errorf("Error getting stdout reader: %s", err)
		return
	} else if pi.parseStdout {
		go pi.ParseOutput(stdoutReader, pi.stdoutDeliverer, pi.stdoutSRunner)
	} else {
		go throwAway(stdoutReader)
	}

	var stderrReader io.Reader
	if stderrReader, err = pi.cc.Stderr_r(); err != nil {
		pi.exitError = fmt.Errorf("Error getting stderr reader: %s", err)
		return
	} else if pi.parseStderr {
		go pi.ParseOutput(stderrReader, pi.stderrDeliverer, pi.stderrSRunner)
	} else {
		go throwAway(stderrReader)
	}
	pi.ccStatus = pi.cc.Wait()
}

func (pi *ProcessInput) ParseOutput(r io.Reader, deliverer Deliverer,
	sRunner SplitterRunner) {
	err := sRunner.SplitStreamNullSplitterToEOF(r, deliverer)
	// Go doesn't seem to have a good solution to streaming output
	// between subprocesses.  It seems like you have to read *all* the
	// content in a goroutine instead of just streaming the content.
	//
	// See: http://code.google.com/p/go/issues/detail?id=2266
	// and http://golang.org/pkg/os/exec/#Cmd.StdoutPipe
	if err != nil && err != io.ErrShortBuffer && err != io.EOF &&
		!strings.Contains(err.Error(), "read |0: bad file descriptor") {
		pi.ir.LogError(fmt.Errorf("Stream Error [%s]", err.Error()))
	}
}

// CleanupForRestart implements the Restarting interface.
func (pi *ProcessInput) CleanupForRestart() {
	// Reset the CommandChain (and therefore os.exec status)
	pi.cc = pi.cc.clone()
	pi.Stop()
}

func init() {
	RegisterPlugin("ProcessInput", func() interface{} {
		return new(ProcessInput)
	})
}
