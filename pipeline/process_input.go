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
	"bytes"
	"code.google.com/p/go-uuid/uuid"
	"fmt"
	"github.com/mozilla-services/heka/message"
	"io"
	"os"
	"strconv"
	"strings"
	"time"
)

type cmd_config struct {
	Bin string

	// Command arguments
	Args []string

	// Enviroment variables
	Env []string

	// Dir specifies the working directory of Command.  Defaults to
	// the directory where the program resides.
	Directory string
}

type ProcessInputConfig struct {
	// Some name to tag this commandchain
	Name string

	// Command(s) to run.  If multiple commands are specified they will
	// be run in the order specified, and the standard output stream
	// will be piped to the standard input of the next command.
	Command map[string]cmd_config

	// TickerInterval is the number of seconds to wait between
	// runnning Command.
	TickerInterval uint `toml:"ticker_interval"`

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

	// Trim newline characters from the right side
	Trim bool `toml: trim`

	// Timeout in seconds
	TimeoutSeconds uint `toml:"timeout"`

	ParseStdout bool `toml:"stdout"`
	ParseStderr bool `toml:"stderr"`
}

// Heka Input plugin that runs external programs and processes their
// output as a stream into Message objects to be passed into
// the Router for delivery to matching Filter or Output plugins.
type ProcessInput struct {
	ProcessName string
	cc          *CommandChain
	ir          InputRunner
	decoderName string

	stdout_r io.ReadCloser
	stderr_r io.ReadCloser

	parseStdout bool
	parseStderr bool

	stdoutChan chan string
	stderrChan chan string

	stopChan chan bool
	parser   StreamParser

	hostname     string
	heka_pid     int32
	tickInterval uint

	trim bool
}

// ConfigStruct implements the HasConfigStruct interface and sets
// defaults.
func (pi *ProcessInput) ConfigStruct() interface{} {
	return &ProcessInputConfig{
		Name:           "UnnamedProcessInput",
		TickerInterval: uint(15),
		ParserType:     "token",
		ParseStdout:    true,
		ParseStderr:    false,
		Trim:           true,
	}
}

// Init implements the Plugin interface.
func (pi *ProcessInput) Init(config interface{}) (err error) {
	conf := config.(*ProcessInputConfig)

	pi.stdoutChan = make(chan string)
	pi.stderrChan = make(chan string)
	pi.stopChan = make(chan bool)

	pi.trim = conf.Trim

	if conf.Name == "" {
		return fmt.Errorf("Name field is required for ProcessInput plugin")
	}
	pi.ProcessName = conf.Name

	pi.tickInterval = conf.TickerInterval
	pi.parseStdout = conf.ParseStdout
	pi.parseStderr = conf.ParseStderr

	if len(conf.Command) < 1 {
		return fmt.Errorf("No Command Configured")
	}

	pi.cc = &CommandChain{timeout_duration: time.Duration(conf.TimeoutSeconds) * time.Second}

	// We need to mangle the indexes to be integers
	for idx := 0; idx < len(conf.Command); idx++ {
		str_idx := strconv.Itoa(idx)
		cmd_cfg, ok := conf.Command[str_idx]
		if !ok {
			return fmt.Errorf("Expected to find a command at index [%s][%d]", conf.Name, idx)
		}

		cmd, err := pi.cc.AddStep(cmd_cfg.Bin, cmd_cfg.Args...)
		if err != nil {
			return err
		}

		if cmd_cfg.Directory != "" {
			cmd.Dir = cmd_cfg.Directory
		}
		if cmd_cfg.Env != nil {
			cmd.Env = cmd_cfg.Env
		}
	}

	pi.decoderName = conf.Decoder

	switch conf.ParserType {
	case "token":
		tp := NewTokenParser()
		pi.parser = tp

		switch len(conf.Delimiter) {
		case 0: // no value was set, the default provided by the StreamParser will be used
		case 1:
			tp.SetDelimiter(conf.Delimiter[0])
		default:
			return fmt.Errorf("invalid delimiter: %s", conf.Delimiter)
		}

	case "regexp":
		rp := NewRegexpParser()
		pi.parser = rp
		if err = rp.SetDelimiter(conf.Delimiter); err != nil {
			return err
		}
		if err = rp.SetDelimiterLocation(conf.DelimiterLocation); err != nil {
			return nil
		}
	default:
		return fmt.Errorf("unknown parser type: %s", conf.ParserType)
	}

	hostname, err := os.Hostname()
	if err != nil {
		return err
	}
	pi.hostname = hostname

	pi.heka_pid = int32(os.Getpid())

	return nil
}

func (pi *ProcessInput) Run(ir InputRunner, h PluginHelper) error {
	var (
		pack                *PipelinePack
		dRunner             DecoderRunner
		ok                  bool
		router_shortcircuit bool
	)

	// So we can access our InputRunner outside of the Run function.
	pi.ir = ir
	pConfig := h.PipelineConfig()

	// Try to get the configured decoder.

	if pi.decoderName == "" {
		router_shortcircuit = true
	} else if dRunner, ok = h.DecoderSet().ByName(pi.decoderName); !ok {
		return fmt.Errorf("Decoder not found: %s", pi.decoderName)
	}

	// Start the output parser and start running commands.
	go pi.RunCmd()

	packSupply := ir.InChan()
	// Wait for and route populated PipelinePacks.
	for {
		select {
		case data := <-pi.stdoutChan:
			pack = <-packSupply
			pi.writetopack(data, pack, "stdout")

			if router_shortcircuit {
				pConfig.router.InChan() <- pack
			} else {
				dRunner.InChan() <- pack
			}

		case data := <-pi.stderrChan:
			pack = <-packSupply
			pi.writetopack(data, pack, "stderr")

			if router_shortcircuit {
				pConfig.router.InChan() <- pack
			} else {
				dRunner.InChan() <- pack
			}

		case <-pi.stopChan:
			return nil
		}
	}

	return nil
}

func (pi *ProcessInput) writetopack(data string, pack *PipelinePack, stream_name string) {
	pack.Message.SetUuid(uuid.NewRandom())
	pack.Message.SetTimestamp(time.Now().UnixNano())
	pack.Message.SetType("ProcessInput")
	pack.Message.SetSeverity(int32(0))
	pack.Message.SetEnvVersion("0.8")
	pack.Message.SetPid(pi.heka_pid)
	pack.Message.SetHostname(pi.hostname)
	pack.Message.SetLogger(pi.ir.Name())
	pack.Message.SetPayload(data)
	if fPInputName, err := message.NewField("ProcessInputName",
		fmt.Sprintf("%s.%s", pi.ProcessName, stream_name),
		""); err == nil {
		pack.Message.AddField(fPInputName)
	} else {
		pi.ir.LogError(err)
	}
}

func (pi *ProcessInput) Stop() {
	// This will shutdown the ProcessInput::RunCmd goroutine
	close(pi.stopChan)
}

// RunCmd pipes multiple commands together, runs them
// per the configured msInterval, and passes the output to
// the provided stdout.
func (pi *ProcessInput) RunCmd() {
	if pi.tickInterval == 0 {
		pi.runOnce()
	} else {
		tickChan := pi.ir.Ticker()
		for {
			select {
			case <-tickChan:
				// No need to spin up a new goroutine as we've already
				// detached from the main thread
				pi.cc.reset()
				pi.runOnce()
			case <-pi.stopChan:
				return
			}
		}
	}
}

func (pi *ProcessInput) runOnce() {
	// Stdout of the last command in the pipe gets sent to provided stdout.
	stdout_reader, err := pi.cc.StdoutPipe()
	if err != nil {
		pi.ir.LogError(fmt.Errorf("Error grabbing stdout pipe: %s", err.Error()))
	}
	pi.stdout_r = stdout_reader

	if pi.parseStdout {
		go pi.ParseOutput(pi.stdout_r, pi.stdoutChan)
	}

	stderr_reader, err := pi.cc.StderrPipe()
	if err != nil {
		pi.ir.LogError(fmt.Errorf("Error grabbing stderr pipe: %s", err.Error()))
	}
	pi.stderr_r = stderr_reader
	if pi.parseStderr {
		go pi.ParseOutput(pi.stderr_r, pi.stderrChan)
	}

	err = pi.cc.Start()
	if err != nil {
		pi.ir.LogError(err)
	}

	err = pi.cc.Wait()
	if err != nil {
		pi.ir.LogError(err)
	}
}

func (pi *ProcessInput) ParseOutput(r io.Reader, outputChannel chan string) {
	var (
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
			}
		}

		if pi.trim && record != nil {
			record = bytes.TrimRight(record, "\n")
		}

		if len(record) > 0 {
			// Setup and send the Message
			outputChannel <- string(record)
		}

		if err != nil {
			// golang doesn't seem to have a good solution to
			// streaming output between subprocesses.  It seems like
			// you have to read *all* the content in a goroutine
			// instead of just streaming the content.
			//
			// See: http://code.google.com/p/go/issues/detail?id=2266
			// and http://golang.org/pkg/os/exec/#Cmd.StdoutPipe
			if !strings.Contains(err.Error(), "read |0: bad file descriptor") &&
				(err != io.EOF) {
				pi.ir.LogError(err)
			}
			return
		}
	}
}

// CleanupForRestart implements the Restarting interface.
func (pi *ProcessInput) CleanupForRestart() {
	pi.Stop()
}