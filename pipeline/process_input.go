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
    "github.com/mozilla-services/heka/message"
    "code.google.com/p/go-uuid/uuid"
    "os/exec"
    "fmt"
    "io"
    "time"
    "bytes"
    "os"
)

const (
    AlreadyStarted = "exec: already started"
)

type ProcessInputConfig struct {
    // Command to run.
    Command []string

    // TickerInterval is the number of seconds to wait between runnning
    // Command.  In cases where the program is designed to run continuously
    // this is irrelevant, and FailOnTimeout should be false.
    // Default is 15 seconds.
    TickerInterval uint `toml:"ticker_interval"`

    // Captures stdout from the Command.  Defaults to true.
    Stdout bool

    // Captures stderr from the Command.  Defaults to false
    Stderr bool

    // TolerateFailures prevents ProcessInput from stopping due to Command
    // failures.  Defaults to false.
    TolerateFailures bool `toml:"tolerate_failures"`

    // If FailOnTimeout is true the plugin will stop when the command has not
    // exited before the next TickInterval. Defaults to true.
    FailOnTimeout bool `toml:"fail_on_timeout"`

    // EnvVars is used to set environment variables before Command is run.
    // Defaults to nil, which uses the current process's environment.
    Env []string `toml:"environment"`

    // Directory specifies the working directory of Command.  Defaults to
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
    Cmd          *ManagedCmd
    ir           InputRunner
    outChan      chan *PipelinePack
    parser       StreamParser
    StdoutReader io.Reader
    StderrReader io.Reader
    config       *ProcessInputConfig
}

// ConfigStruct implements the HasConfigStruct interface and sets defaults.
func (pi *ProcessInput) ConfigStruct() interface{} {
    return &ProcessInputConfig {
        TickerInterval:   5,
        ParserType:       "token",
        Stdout:           true,
        Stderr:           false,
        TolerateFailures: false,
        FailOnTimeout:    true,
    }
}

// Init implements the Plugin interface.
func (pi *ProcessInput) Init(config interface{}) error {
    pi.Cmd         = new(ManagedCmd)
    pi.outChan     = make(chan *PipelinePack)
    pi.config      = config.(*ProcessInputConfig)
    conf          := pi.config // Just an alias.

    // Setup the Cmd
    cmdL := len(conf.Command)
    switch cmdL {
    case 0:  return fmt.Errorf("No Command Configured")
    case 1:
        pi.Cmd.Cmd = *exec.Command(conf.Command[0])
    default:
        pi.Cmd.Cmd = *exec.Command(conf.Command[0], conf.Command[1:cmdL]...)
    }
    if pi.config.Env != nil { pi.Cmd.Env = pi.config.Env }
    pi.Cmd.TolerateFailures = pi.config.TolerateFailures
    pi.Cmd.FailOnTimeout    = pi.config.FailOnTimeout
    pi.Cmd.RunChan          = make(<-chan time.Time)
    pi.Cmd.StopChan         = make(chan bool)

    // Setup the appropriate parser per the configuration.
    switch pi.config.ParserType {
    case "token":
        tp := NewTokenParser()
        if pi.config.Delimiter != "" {
            tp.SetDelimiter([]byte(pi.config.Delimiter)[0])
        }
        pi.parser = tp
    case "regex":
        rp := NewRegexpParser()
        rp.SetDelimiter(pi.config.Delimiter)
        rp.SetDelimiterLocation(pi.config.DelimiterLocation)
        pi.parser = rp
    }

    // These are pipes where command output is written to for parsing
    // into heka messages.
    if conf.Stdout {
        pi.StdoutReader, pi.Cmd.Stdout = io.Pipe()
        go pi.ParseCmdOutput(pi.StdoutReader)
    }
    if conf.Stderr {
        pi.StderrReader, pi.Cmd.Stderr = io.Pipe()
        go pi.ParseCmdOutput(pi.StderrReader)
    }

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
    if pi.config.Decoder != "" {
        if dRunner, ok = h.DecoderSet().ByName(pi.config.Decoder); !ok {
            return fmt.Errorf("Decoder not found: %s", pi.config.Decoder)
        }
    }

    // Route populated PipelinePacks.
    go func() {
        for pack = range pi.outChan {
            if dRunner == nil {
                pi.ir.Inject(pack)
            } else {
                dRunner.InChan() <- pack
            }
        }
    }()

    // Set the Run ticker for Cmd and start running.  Log fatal errors.
    pi.Cmd.RunChan = ir.Ticker()
    err := pi.Cmd.Run()
    if err != nil { return err }

    return nil
}

func (pi *ProcessInput) Stop() {
    close(pi.Cmd.StopChan)
    close(pi.outChan)
}

func (pi *ProcessInput) CleanupForRestart() {
    pi.Stop()
}

// ManagedCmd extends exec.Cmd to support running on a ticker interval. A
// timeout condition occurs when the Cmd has not exited by the next RunChan
// tick.
type ManagedCmd struct {
    exec.Cmd
    RunChan        <-chan time.Time
    StopChan         chan bool
    errChan          chan error
    failChan         chan error
    TolerateFailures bool
    FailOnTimeout    bool
}

// Run manages Cmd run intervals and failures.
func (mc *ManagedCmd) Run() (err error) {
    // Setup error channel and start the error handler.
    mc.errChan  = make(chan error)
    mc.failChan = make(chan error)
    go func() { mc.failChan <-mc.handleErrors() }()

    // Here's where we continue running commands on an interval or stop.
    for {
        select {
        case <-mc.StopChan:
            return err
        case <-mc.RunChan:
            go func() { mc.errChan <-mc.Cmd.Run() }()
        case err = <-mc.failChan:
            return err
        }
    }

    return err
}

// handleErrors manages failure situations.
func (mc *ManagedCmd) handleErrors() error {
    var err error
    for {
        select {
        case <-mc.StopChan:
            return nil
        case err = <-mc.errChan:
            // No error, so we create a new Cmd to run.
            if  mc.TolerateFailures || err == nil {
                mc.refreshCmd()
                continue
            }

            if err.Error() == "exec: already started" && mc.FailOnTimeout {
                return fmt.Errorf("Command Timed Out")
            }
            
            return fmt.Errorf("Command Returned Error: %v", err)
        }
    }
    return nil
}

func (mc *ManagedCmd) refreshCmd() {
    newCmd := &exec.Cmd{
        Path: mc.Path,
        Args: mc.Args,
        Env:  mc.Env,
        Dir:  mc.Dir,
        Stdin: mc.Stdin,
        Stdout: mc.Stdout,
        Stderr: mc.Stderr,
        ExtraFiles: mc.ExtraFiles,
        SysProcAttr: mc.SysProcAttr,
    }
    mc.Cmd = *newCmd
}

func (pi *ProcessInput) ParseCmdOutput(r io.Reader) {
    var (
        pack     *PipelinePack
        record   []byte
        err      error
        pid      int = os.Getpid()
        hostname string
    )

    hostname, err = os.Hostname()
    if err != nil { hostname = "unknown-hostname" }

    for {
        // Use configured StreamParser to split output from commands.
        _, record, err = pi.parser.Parse(r)
        if err != nil {
            if err == io.EOF {
                record = pi.parser.GetRemainingData()
            } else if err == io.ErrShortBuffer {
                pi.ir.LogError(fmt.Errorf("record exceeded MAX_RECORD_SIZE %d",
                               message.MAX_RECORD_SIZE))
                err = nil // non-fatal, keep going
            } else {
                panic(err)
            }
        }

        if len(record) > 0 {
            // Remove trailing newline
            record = bytes.TrimRight(record, "\n")
            // Setup and send the Message
            pack = <-pi.ir.InChan()
            pack.Message.SetUuid(uuid.NewRandom())
            pack.Message.SetTimestamp(time.Now().UnixNano())
            pack.Message.SetType("ProcessInput")
            // pack.Message.SetSeverity(int32(0))
            // pack.Message.SetEnvVersion("")
            pack.Message.SetPid(int32(pid))
            pack.Message.SetHostname(hostname)
            pack.Message.SetLogger(pi.ir.Name())
            pack.Message.SetPayload(string(record))
            pi.outChan <-pack
        }
    }
}
