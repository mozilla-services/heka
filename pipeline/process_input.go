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
    // "github.com/davecgh/go-spew/spew"
    "github.com/mozilla-services/heka/message"
    "code.google.com/p/go-uuid/uuid"
    "os/exec"
    "fmt"
    "io"
    "time"
)

type ProcessInputConfig struct {
    // Command(s) to run.  If multiple commands are specified they will
    // be run in the order specified, and the standard output stream
    // will be piped to the standard input of the next command.
    Command [][]string

    // EnvVars is used to set environment variables before Command is run.
    // Defaults to nil, which uses the current process's environment.
    Env []string `toml:"environment"`

    // Dir specifies the working directory of Command.  Defaults to
    // the directory where the program resides.
    Directory string

    // RunInterval is the number of milliseconds to wait between runnning
    // Command.  In cases where the program is designed to run continuously
    // RunInterval is essentially irrelevant. Default is 500 (5 seconds).
    RunInterval int `toml:"run_interval"`

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
    runInterval time.Duration
    ir          InputRunner
    decoderName string
    w           io.PipeWriter
    r           io.Reader
    outChan     chan *PipelinePack
    stopChan    chan bool
    parser      StreamParser
}

// ConfigStruct implements the HasConfigStruct interface and sets
// defaults.
func (pi *ProcessInput) ConfigStruct() interface{} {
    return &ProcessInputConfig {
        RunInterval: 5000,
        ParserType:  "token",
    }
}

// Init implements the Plugin interface.
func (pi *ProcessInput) Init(config interface{}) error {
    pi.outChan  = make(chan *PipelinePack)
    pi.stopChan = make(chan bool)
    conf := config.(*ProcessInputConfig)
    if len(conf.Command) < 1 { return fmt.Errorf("No Command Configured") }
    pi.cmds = make([]exec.Cmd, len(conf.Command))
    for i, v := range conf.Command {
        switch len(v) {
            case 0:  return fmt.Errorf("Empty Command Configured")
            case 1:  pi.cmds[i] = *exec.Command(v[0])
            default: pi.cmds[i] = *exec.Command(v[0], v[1:len(v)]...)
        }
        if conf.Env != nil { pi.cmds[i].Env = conf.Env }
    }
    pi.runInterval = time.Millisecond * time.Duration(conf.RunInterval)
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

    // Pipe the commands together by stdin/stdout.
    // last := len(pi.cmds)-1
    for i, _ := range pi.cmds {
        if i != 0 {
            stdout, err := pi.cmds[i-1].StdoutPipe()
            if err != nil { return err }
            pi.cmds[i].Stdin = stdout
        }
    }

    return nil
}

func (pi *ProcessInput) Run(ir InputRunner, h PluginHelper) error {
    var (
        pack    *PipelinePack
        dRunner DecoderRunner
        ok      bool
    )

    pi.ir = ir

    if pi.decoderName != "" {
        if dRunner, ok = h.DecoderSet().ByName(pi.decoderName); !ok {
            return fmt.Errorf("Decoder not found: %s", pi.decoderName)
        }
    }

    // go pi.ParseOutput()
    go pi.RunCmd()

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
// per the configured interval, and passes the output to
// ParseOutput for processing into packs.
func (pi *ProcessInput) RunCmd() (err error) {
    run  := time.Tick(pi.runInterval)

    for {
        select {
        case <-pi.stopChan:
            return nil
        case <-run:
            // Run all commands in the background.
            cmds := make([]exec.Cmd, len(pi.cmds))
            copy(cmds, pi.cmds)
            r, w := io.Pipe()
            cmds[len(cmds)-1].Stdout = w
            for _, v := range cmds {
                err = v.Start()
                if err != nil { panic(err) }
            }
            go pi.ParseOutput(r)
        }
    }
    return nil
}

func (pi *ProcessInput) ParseOutput(r io.Reader) {
    var (
        pack    *PipelinePack
        record  []byte
        err     error
    )

    for {
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
            pack.Message.SetPid(0) // TODO: PID of commands?
            pack.Message.SetHostname("testhost") // TODO: Get OS hostname
            pack.Message.SetLogger(pi.ir.Name())
            pack.Message.SetPayload(string(record))
            pi.outChan <-pack
        }
    }
}

// CleanupForRestart implements the Restarting interface.
func (pi *ProcessInput) CleanupForRestart() {
    close(pi.stopChan)
    close(pi.outChan)
}
