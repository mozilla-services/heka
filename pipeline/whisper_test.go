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
#   Rob Miller (rmiller@mozilla.com)
#
# ***** END LICENSE BLOCK *****/

package pipeline

import (
	"code.google.com/p/gomock/gomock"
	"fmt"
	ts "github.com/mozilla-services/heka/testsupport"
	"github.com/rafrombrc/gospec/src/gospec"
	gs "github.com/rafrombrc/gospec/src/gospec"
	"github.com/rafrombrc/whisper-go/whisper"
	"os"
	"path"
	"strings"
	"sync"
	"time"
)

func WhisperRunnerSpec(c gospec.Context) {
	tmpDir := os.TempDir()
	tmpFileName := fmt.Sprintf("heka-%d.wsp", time.Now().UTC().UnixNano())
	tmpFileName = path.Join(tmpDir, tmpFileName)

	interval := uint32(10)
	archiveInfo := []whisper.ArchiveInfo{
		whisper.ArchiveInfo{0, interval, 60},
		whisper.ArchiveInfo{0, 60, 8},
	}

	c.Specify("A WhisperRunner", func() {
		var wg sync.WaitGroup
		wg.Add(1)
		wr, err := NewWhisperRunner(tmpFileName, archiveInfo, whisper.AGGREGATION_SUM, &wg)
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
	t := &ts.SimpleT{}
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	oth := new(OutputTestHelper)
	oth.MockHelper = NewMockPluginHelper(ctrl)
	oth.MockOutputRunner = NewMockOutputRunner(ctrl)
	inChan := make(chan *PipelineCapture, 1)
	pConfig := NewPipelineConfig(1)

	c.Specify("A WhisperOutput", func() {
		o := new(WhisperOutput)
		config := o.ConfigStruct()
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

		pack := NewPipelinePack(pConfig)
		pack.Message.SetPayload(strings.Join(lines, "\n"))
		pack.Config.RecycleChan = make(chan *PipelinePack, 1) // don't block on recycle
		plc := &PipelineCapture{Pack: pack}

		c.Specify("turns statmetric lines into points", func() {
			inChanCall := oth.MockOutputRunner.EXPECT().InChan()
			inChanCall.Return(inChan)
			wChanCall := mockWr.EXPECT().InChan().Times(count)
			wChanCall.Return(wChan)

			go o.Run(oth.MockOutputRunner, oth.MockHelper)
			inChan <- plc

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
