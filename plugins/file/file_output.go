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
#   Rob Miller (rmiller@mozilla.com)
#   Mike Trinkala (trink@mozilla.com)
#   Bruno Binet (bruno.binet@gmail.com)
#
# ***** END LICENSE BLOCK *****/

package file

import (
	"errors"
	"fmt"
	. "github.com/mozilla-services/heka/pipeline"
	"github.com/mozilla-services/heka/plugins"
	"github.com/rafrombrc/go-notify"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

// Output plugin that writes message contents to a file on the file system.
type FileOutput struct {
	*FileOutputConfig
	path       string
	perm       os.FileMode
	flushOpAnd bool
	file       *os.File
	batchChan  chan []byte
	backChan   chan []byte
	folderPerm os.FileMode
	timerChan  <-chan time.Time
	rotateChan chan time.Time
	closing    chan struct{}
}

// ConfigStruct for FileOutput plugin.
type FileOutputConfig struct {
	// Full output file path.
	// If date rotation is in use, then the output file name can support
	// Go's time.Format syntax to embed timestamps in the filename:
	// http://golang.org/pkg/time/#Time.Format
	Path string

	// Output file permissions (default "644").
	Perm string

	// Interval at which the output file should be rotated, in hours.
	// Only the following values are allowed: 0, 1, 4, 12, 24
	// The files will be named relative to midnight of the day.
	// (default 0, i.e. disabled). Set to 0 to disable.
	RotationInterval uint32 `toml:"rotation_interval"`

	// Interval at which accumulated file data should be written to disk, in
	// milliseconds (default 1000, i.e. 1 second). Set to 0 to disable.
	FlushInterval uint32 `toml:"flush_interval"`

	// Number of messages to accumulate until file data should be written to
	// disk (default 1, minimum 1).
	FlushCount uint32 `toml:"flush_count"`

	// Operator describing how the two parameters "flush_interval" and
	// "flush_count" are combined. Allowed values are "AND" or "OR" (default is
	// "AND").
	FlushOperator string `toml:"flush_operator"`

	// Permissions to apply to directories created for FileOutput's parent
	// directory if it doesn't exist.  Must be a string representation of an
	// octal integer. Defaults to "700".
	FolderPerm string `toml:"folder_perm"`

	// Specifies whether or not Heka's stream framing will be applied to the
	// output. We do some magic to default to true if ProtobufEncoder is used,
	// false otherwise.
	UseFraming *bool `toml:"use_framing"`
}

func (o *FileOutput) ConfigStruct() interface{} {
	return &FileOutputConfig{
		Perm:             "644",
		RotationInterval: 0,
		FlushInterval:    1000,
		FlushCount:       1,
		FlushOperator:    "AND",
		FolderPerm:       "700",
	}
}

func (o *FileOutput) Init(config interface{}) (err error) {
	conf := config.(*FileOutputConfig)
	o.FileOutputConfig = conf
	var intPerm int64

	if intPerm, err = strconv.ParseInt(conf.FolderPerm, 8, 32); err != nil {
		err = fmt.Errorf("FileOutput '%s' can't parse `folder_perm`, is it an octal integer string?",
			o.Path)
		return err
	}
	o.folderPerm = os.FileMode(intPerm)

	if intPerm, err = strconv.ParseInt(conf.Perm, 8, 32); err != nil {
		err = fmt.Errorf("FileOutput '%s' can't parse `perm`, is it an octal integer string?",
			o.Path)
		return err
	}
	o.perm = os.FileMode(intPerm)

	if conf.FlushCount < 1 {
		err = fmt.Errorf("Parameter 'flush_count' needs to be greater 1.")
		return err
	}
	switch conf.FlushOperator {
	case "AND":
		o.flushOpAnd = true
	case "OR":
		o.flushOpAnd = false
	default:
		err = fmt.Errorf("Parameter 'flush_operator' needs to be either 'AND' or 'OR', is currently: '%s'",
			conf.FlushOperator)
		return err
	}

	o.closing = make(chan struct{})
	switch conf.RotationInterval {
	case 0:
		// date rotation is disabled
		o.path = o.Path
	case 1, 4, 12, 24:
		// RotationInterval value is allowed
		o.startRotateNotifier()
	default:
		err = fmt.Errorf("Parameter 'rotation_interval' must be one of: 0, 1, 4, 12, 24.")
		return err
	}
	if err = o.openFile(); err != nil {
		err = fmt.Errorf("FileOutput '%s' error opening file: %s", o.path, err)
		close(o.closing)
		return err
	}

	o.batchChan = make(chan []byte)
	o.backChan = make(chan []byte, 2) // Never block on the hand-back
	return nil
}

func (o *FileOutput) startRotateNotifier() {
	now := time.Now()
	interval := time.Duration(o.RotationInterval) * time.Hour
	last := now.Truncate(interval)
	next := last.Add(interval)
	until := next.Sub(now)
	after := time.After(until)

	o.path = last.Format(o.Path)
	o.rotateChan = make(chan time.Time)

	go func() {
		ok := true
		for ok {
			select {
			case _, ok = <-o.closing:
				break
			case <-after:
				last = next
				next = next.Add(interval)
				until = next.Sub(time.Now())
				after = time.After(until)
				o.rotateChan <- last
			}
		}
	}()
}

func (o *FileOutput) openFile() (err error) {
	basePath := filepath.Dir(o.path)
	if err = os.MkdirAll(basePath, o.folderPerm); err != nil {
		return fmt.Errorf("Can't create the basepath for the FileOutput plugin: %s", err.Error())
	}
	if err = plugins.CheckWritePermission(basePath); err != nil {
		return
	}
	o.file, err = os.OpenFile(o.path, os.O_WRONLY|os.O_APPEND|os.O_CREATE, o.perm)
	return
}

func (o *FileOutput) Run(or OutputRunner, h PluginHelper) error {
	enc := or.Encoder()
	if enc == nil {
		return errors.New("Encoder required.")
	}
	if o.UseFraming == nil {
		// Nothing was specified, we'll default to framing IFF ProtobufEncoder
		// is being used.
		if _, ok := enc.(*ProtobufEncoder); ok {
			or.SetUseFraming(true)
		}
	}

	errChan := make(chan error, 1)
	go o.committer(or, errChan)
	return o.receiver(or, errChan)
}

func (o *FileOutput) receiver(or OutputRunner, errChan chan error) (err error) {
	var (
		pack            *PipelinePack
		e               error
		timer           *time.Timer
		timerDuration   time.Duration
		msgCounter      uint32
		intervalElapsed bool
		outBytes        []byte
	)
	ok := true
	outBatch := make([]byte, 0, 10000)
	inChan := or.InChan()

	timerDuration = time.Duration(o.FlushInterval) * time.Millisecond
	if o.FlushInterval > 0 {
		timer = time.NewTimer(timerDuration)
		if o.timerChan == nil { // Tests might have set this already.
			o.timerChan = timer.C
		}
	}

	for ok {
		select {
		case pack, ok = <-inChan:
			if !ok {
				// Closed inChan => we're shutting down, flush data
				if len(outBatch) > 0 {
					o.batchChan <- outBatch
				}
				close(o.batchChan)
				break
			}
			if outBytes, e = or.Encode(pack); e != nil {
				or.LogError(e)
			} else if outBytes != nil {
				outBatch = append(outBatch, outBytes...)
				msgCounter++
			}
			pack.Recycle()

			// Trigger immediately when the message count threshold has been
			// reached if a) the "OR" operator is in effect or b) the
			// flushInterval is 0 or c) the flushInterval has already elapsed.
			// at least once since the last flush.
			if msgCounter >= o.FlushCount {
				if !o.flushOpAnd || o.FlushInterval == 0 || intervalElapsed {
					// This will block until the other side is ready to accept
					// this batch, freeing us to start on the next one.
					o.batchChan <- outBatch
					outBatch = <-o.backChan
					msgCounter = 0
					intervalElapsed = false
					if timer != nil {
						timer.Reset(timerDuration)
					}
				}
			}
		case <-o.timerChan:
			if (o.flushOpAnd && msgCounter >= o.FlushCount) ||
				(!o.flushOpAnd && msgCounter > 0) {

				// This will block until the other side is ready to accept
				// this batch, freeing us to start on the next one.
				o.batchChan <- outBatch
				outBatch = <-o.backChan
				msgCounter = 0
				intervalElapsed = false
			} else {
				intervalElapsed = true
			}
			timer.Reset(timerDuration)
		case err = <-errChan:
			ok = false
			break
		}
	}
	return err
}

// Runs in a separate goroutine, waits for buffered data on the committer
// channel, writes it out to the filesystem, and puts the now empty buffer on
// the return channel for reuse.
func (o *FileOutput) committer(or OutputRunner, errChan chan error) {
	initBatch := make([]byte, 0, 10000)
	o.backChan <- initBatch
	var outBatch []byte
	var err error

	ok := true
	hupChan := make(chan interface{})
	notify.Start(RELOAD, hupChan)

	for ok {
		select {
		case outBatch, ok = <-o.batchChan:
			if !ok {
				// Channel is closed => we're shutting down, exit cleanly.
				o.file.Close()
				close(o.closing)
				break
			}
			n, err := o.file.Write(outBatch)
			if err != nil {
				or.LogError(fmt.Errorf("Can't write to %s: %s", o.path, err))
			} else if n != len(outBatch) {
				or.LogError(fmt.Errorf("Truncated output for %s", o.path))
			} else {
				o.file.Sync()
			}
			outBatch = outBatch[:0]
			o.backChan <- outBatch
		case <-hupChan:
			o.file.Close()
			if err = o.openFile(); err != nil {
				close(o.closing)
				err = fmt.Errorf("unable to reopen file '%s': %s", o.path, err)
				errChan <- err
				ok = false
				break
			}
		case rotateTime := <-o.rotateChan:
			o.file.Close()
			o.path = rotateTime.Format(o.Path)
			if err = o.openFile(); err != nil {
				close(o.closing)
				err = fmt.Errorf("unable to open rotated file '%s': %s", o.path, err)
				errChan <- err
				ok = false
				break
			}
		}
	}
}

func init() {
	RegisterPlugin("FileOutput", func() interface{} {
		return new(FileOutput)
	})
}
