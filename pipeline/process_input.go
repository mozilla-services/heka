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
#
#***** END LICENSE BLOCK *****/

package pipeline

import (
	"bytes"
	"code.google.com/p/go-uuid/uuid"
	"fmt"
	"github.com/mozilla-services/heka/message"
	"io"
	"os/exec"
	"time"
)

type ProcessInputConfig struct {
	// Command(s) to run.  If multiple commands are specified they will
	// be run in the order specified, and the standard output stream
	// will be piped to the standard input of the next command.
	Command [][]string

	// RunInterval is the number of milliseconds to wait between runnning
	// Command.  In cases where the program is designed to run continuously
	// RunInterval is essentially irrelevant. Default is 5000 (5 seconds).
	RunInterval int `toml:"run_interval"`

	// EnvVars is used to set environment variables before Command is run.
	// Defaults to nil, which uses the current process's environment.
	Env []string `toml:"environment"`

	// Dir specifies the working directory of Command.  Defaults to
	// the directory where the program resides.
	Directory string

	// Name of configured decoder instance.
	Decoder string

	// ParserType is the parser used to split program output into
	// heka messages. Defaults to "token".
	ParserType string `toml:"parser_type"`

	// Delimiter used to split the output stream into heka messages.
	// Defaults to newline.
	Delimiter string

	// String indicating if the delimiter is at the start or end of the line.
	// Only used for regexp delimiters
	DelimiterLocation string `toml:"delimiter_location"`
}

// Heka Input plugin that runs external programs and processes their
// output as a stream into Message objects to be passed into
// the Router for delivery to matching Filter or Output plugins.
type ProcessInput struct {
	cmds        []exec.Cmd
	runInterval int
	ir          InputRunner
	decoderName string
	w           io.Writer
	r           io.Reader
	outChan     chan *PipelinePack
	stopChan    chan bool
	parser      StreamParser
}

// ConfigStruct implements the HasConfigStruct interface and sets
// defaults.
func (pi *ProcessInput) ConfigStruct() interface{} {
	return &ProcessInputConfig{
		RunInterval: 5000,
		ParserType:  "token",
	}
}

// Init implements the Plugin interface.
func (pi *ProcessInput) Init(config interface{}) error {
	pi.outChan = make(chan *PipelinePack)
	pi.stopChan = make(chan bool)
	conf := config.(*ProcessInputConfig)
	if conf.RunInterval < 0 {
		return fmt.Errorf("Negative run_interval Configured")
	}
	if len(conf.Command) < 1 {
		return fmt.Errorf("No Command Configured")
	}
	pi.cmds = make([]exec.Cmd, len(conf.Command))
	for i, v := range conf.Command {
		switch len(v) {
		case 0:
			return fmt.Errorf("Empty Command Configured")
		case 1:
			pi.cmds[i] = *exec.Command(v[0])
		default:
			pi.cmds[i] = *exec.Command(v[0], v[1:len(v)]...)
		}
		if conf.Env != nil {
			pi.cmds[i].Env = conf.Env
		}
	}

	pi.runInterval = conf.RunInterval
	pi.decoderName = conf.Decoder

	switch conf.ParserType {
	case "token":
		tp := NewTokenParser()
		pi.parser = tp
		if conf.Delimiter == "" {
		} else {
			tp.SetDelimiter([]byte(conf.Delimiter)[0])
		}
	case "regex":
		rp := NewRegexpParser()
		pi.parser = rp
		rp.SetDelimiter(conf.Delimiter)
		rp.SetDelimiterLocation(conf.DelimiterLocation)
	}

	// This is the main pipe where command output is
	// written to for parsing into messages.
	r, w := io.Pipe()
	pi.r = r
	pi.w = w

	return nil
}

func (pi *ProcessInput) Run(ir InputRunner, h PluginHelper) error {
	var (
		pack    *PipelinePack
		dRunner DecoderRunner
		ok      bool
	)

	// So we can access our InputRunner outside of the Run function.
	pi.ir = ir

	// Try to get the configured decoder.
	if pi.decoderName != "" {
		if dRunner, ok = h.DecoderSet().ByName(pi.decoderName); !ok {
			return fmt.Errorf("Decoder not found: %s", pi.decoderName)
		}
	}

	// Start the output parser and start running commands.
	go pi.ParseOutput(pi.r)
	go RunCmd(pi.cmds, pi.runInterval, pi.w, pi.stopChan)

	// Wait for and route populated PipelinePacks.
	for pack = range pi.outChan {
		if dRunner == nil {
			pi.ir.Inject(pack)
		} else {
			dRunner.InChan() <- pack
		}
	}

	return nil
}

func (pi *ProcessInput) Stop() {
	close(pi.stopChan)
	close(pi.outChan)
}

// RunCmd pipes multiple commands together, runs them
// per the configured msInterval, and passes the output to
// the provided stdout.
func RunCmd(cmds []exec.Cmd, msInterval int, stdout io.Writer, stopChan chan bool) (err error) {
	var (
		last int
		run  <-chan time.Time
	)

	last = len(cmds) - 1

	if msInterval == 0 {
		run = time.Tick(time.Millisecond * time.Duration(1000))
	} else {
		run = time.Tick(time.Millisecond * time.Duration(msInterval))
	}

	// Pipe the commands together by stdin/stdout.
	for i, _ := range cmds {
		if i != 0 {
			cmds[i].Stdin = nil
			cmds[i-1].Stdout = nil
			prevStdout, err := cmds[i-1].StdoutPipe()
			if err != nil {
				return err
			}
			cmds[i].Stdin = prevStdout
		}
	}

	// Stdout of the last command in the pipe gets sent to provided stdout.
	cmds[last].Stdout = stdout

	// Here's where we continue running commands on an interval or stop.
	for {
		select {
		case <-stopChan:
			return nil
		case <-run:
			// Start running commands.
			for _, v := range cmds {
				err = v.Run()
				if err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func (pi *ProcessInput) ParseOutput(r io.Reader) {
	var (
		pack   *PipelinePack
		record []byte
		err    error
	)

	for {
		// Use configured StreamParser to split output from commands.
		_, record, err = pi.parser.Parse(r)
		if err != nil {
			if err == io.EOF {
				record = pi.parser.GetRemainingData()
			} else if err == io.ErrShortBuffer {
				pi.ir.LogError(fmt.Errorf("record exceeded MAX_RECORD_SIZE %d", message.MAX_RECORD_SIZE))
				err = nil // non-fatal, keep going
			} else {
				panic(err)
			}
		}

		if len(record) > 0 {
			// Setup and send the Message
			pack = <-pi.ir.InChan()
			pack.Message.SetUuid(uuid.NewRandom())
			pack.Message.SetTimestamp(time.Now().UnixNano())
			pack.Message.SetType("ProcessInput")
			pack.Message.SetSeverity(int32(0))
			pack.Message.SetEnvVersion("0.8")
			pack.Message.SetPid(0)               // TODO: PID of commands?
			pack.Message.SetHostname("testhost") // TODO: Get OS hostname
			pack.Message.SetLogger(pi.ir.Name())
			record = bytes.TrimRight(record, "\n")
			pack.Message.SetPayload(string(record))
			pi.outChan <- pack
		}
	}
}

// CleanupForRestart implements the Restarting interface.
func (pi *ProcessInput) CleanupForRestart() {
	close(pi.stopChan)
	close(pi.outChan)
}
