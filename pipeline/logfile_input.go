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
#   Ben Bangert (bbangert@mozilla.com)
#
# ***** END LICENSE BLOCK *****/
package pipeline

import (
	"github.com/howeyc/fsnotify"
)

type LogfileInputConfig struct {
	SincedbFlush int
	WatchFiles   []string
}

type LogfileWriter struct {
}

type Logline struct {
	Path []byte
	Line []byte
}

func (lw *LogfileWriter) ConfigStruct() interface{} {
	return &LogfileInputConfig{1}
}

func (lw *LogfileWriter) MakeOutData() interface{} {
	logline := new(Logline)
	logline.Path = make([]byte, 500)
	logline.Line = make([]byte, 1500)
	return logline
}

func (lw *LogfileWriter) ZeroOutData(outData interface{}) {
	outData.(*Logline).Path = outData.(*Logline).Path[:0]
	outData.(*Logline).Line = outData.(*Logline).Line[:0]
}

func (lw *LogfileWriter) Init(config interface{}) error {
	return nil
}

// We don't actually do anything, because PrepOutData is the
// one that copies data into the pipeline pack as an input
//
// The result of this is that the outData is then zeroed and
// returned to the recycle pool.
func (lw *LogfileWriter) Write(outData interface{}) error {
	return nil
}

func (lw *LogfileWriter) PrepOutData(pack *PipelinePack, outData interface{},
	timeout *time.Duration) error {

}
