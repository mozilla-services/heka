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
#
# ***** END LICENSE BLOCK *****/

package graphite

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	. "github.com/mozilla-services/heka/pipeline"
	pipeline_ts "github.com/mozilla-services/heka/pipeline/testsupport"
	"github.com/mozilla-services/heka/pipelinemock"
	plugins_ts "github.com/mozilla-services/heka/plugins/testsupport"
	"github.com/rafrombrc/gomock/gomock"
	"github.com/rafrombrc/gospec/src/gospec"
	gs "github.com/rafrombrc/gospec/src/gospec"
	"github.com/rafrombrc/whisper-go/whisper"
)

func WhisperRunnerSpec(c gospec.Context) {
	tmpDir := os.TempDir()
	tmpFileName := fmt.Sprintf("heka-%d.wsp", time.Now().UTC().UnixNano())
	tmpFileName = filepath.Join(tmpDir, tmpFileName)

	interval := uint32(10)
	archiveInfo := []whisper.ArchiveInfo{
		whisper.ArchiveInfo{0, interval, 60},
		whisper.ArchiveInfo{0, 60, 8},
	}

	c.Specify("A WhisperRunner", func() {
		var wg sync.WaitGroup
		wg.Add(1)
		folderPerm := os.FileMode(0755)
		wr, err := NewWhisperRunner(tmpFileName, archiveInfo, whisper.AggregationSum,
			folderPerm, &wg)
		c.Assume(err, gs.IsNil)
		defer func() {
			os.Remove(tmpFileName)
		}()

		c.Specify("creates a whisper file of the correct size", func() {
			fi, err := os.Stat(tmpFileName)
			c.Expect(err, gs.IsNil)
			c.Expect(fi.Size(), gs.Equals, int64(856))
			close(wr.InChan())
			wg.Wait()
		})

		c.Specify("writes a data point to the whisper file", func() {
			// Send a data point through and close.
			when := time.Now().UTC()
			val := float64(6)
			pt := whisper.NewPoint(when, val)
			wr.InChan() <- &pt
			close(wr.InChan())
			wg.Wait()

			// Open db file and fetch interval including our data point.
			from := when.Add(-1 * time.Second).Unix()
			until := when.Add(1 * time.Second).Unix()
			db, err := whisper.Open(tmpFileName)
			c.Expect(err, gs.IsNil)
			_, fetched, _ := db.FetchUntil(uint32(from), uint32(until))

			// Verify that our value is stored in the most recent interval and
			// that the diff btn our timestamp and the stored time value is
			// less than the interval duration.
			fpt := fetched[len(fetched)-1]
			diff := when.Sub(fpt.Time().UTC())
			c.Expect(diff.Seconds() < float64(interval), gs.IsTrue)
			c.Expect(fpt.Value, gs.Equals, val)
		})
	})
}

func WhisperOutputSpec(c gospec.Context) {
	t := &pipeline_ts.SimpleT{}
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	oth := new(plugins_ts.OutputTestHelper)
	oth.MockHelper = pipelinemock.NewMockPluginHelper(ctrl)
	oth.MockOutputRunner = pipelinemock.NewMockOutputRunner(ctrl)
	inChan := make(chan *PipelinePack, 1)
	pConfig := NewPipelineConfig(nil)

	c.Specify("A WhisperOutput", func() {
		o := &WhisperOutput{pConfig: pConfig}
		config := o.ConfigStruct().(*WhisperOutputConfig)
		config.BasePath = filepath.Join(os.TempDir(), config.BasePath)
		o.Init(config)

		const count = 5
		lines := make([]string, count)
		baseTime := time.Now().UTC().Add(-10 * time.Second)
		nameTmpl := "stats.name.%d"

		wChan := make(chan *whisper.Point, count)
		mockWr := NewMockWhisperRunner(ctrl)

		for i := 0; i < count; i++ {
			statName := fmt.Sprintf(nameTmpl, i)
			statTime := baseTime.Add(time.Duration(i) * time.Second)
			lines[i] = fmt.Sprintf("%s %d %d", statName, i*2, statTime.Unix())
			o.dbs[statName] = mockWr
		}

		pack := NewPipelinePack(pConfig.InputRecycleChan())
		pack.Message.SetPayload(strings.Join(lines, "\n"))

		c.Specify("turns statmetric lines into points", func() {
			oth.MockOutputRunner.EXPECT().InChan().Return(inChan)
			oth.MockOutputRunner.EXPECT().UpdateCursor("")
			wChanCall := mockWr.EXPECT().InChan().Times(count)
			wChanCall.Return(wChan)

			go o.Run(oth.MockOutputRunner, oth.MockHelper)
			inChan <- pack

			// Usually each wChan will be unique instead of shared across
			// multiple whisper runners. This weird dance here prevents our
			// mock wChan from being closed multiple times.
			bogusChan := make(chan *whisper.Point)
			wChanCall = mockWr.EXPECT().InChan().Times(count)
			wChanCall.Return(bogusChan)
			wChanCall.Do(func() {
				wChanCall.Return(make(chan *whisper.Point))
			})

			close(inChan)
			<-bogusChan // wait for inChan to flush
			close(wChan)

			i := 0
			for pt := range wChan {
				statTime := baseTime.Add(time.Duration(i) * time.Second)
				c.Expect(pt.Value, gs.Equals, float64(i*2))
				c.Expect(pt.Time().UTC().Unix(), gs.Equals, statTime.Unix())
				i++
			}
		})
	})
}
