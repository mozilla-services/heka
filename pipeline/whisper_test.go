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
	"fmt"
	"github.com/rafrombrc/gospec/src/gospec"
	gs "github.com/rafrombrc/gospec/src/gospec"
	"github.com/rafrombrc/whisper-go/whisper"
	"os"
	"path"
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
		wr, err := NewWhisperRunner(tmpFileName, archiveInfo, whisper.AGGREGATION_SUM)
		c.Assume(err, gs.IsNil)
		defer func() {
			os.Remove(tmpFileName)
		}()

		c.Specify("creates a whisper file of the correct size", func() {
			fi, err := os.Stat(tmpFileName)
			c.Expect(err, gs.IsNil)
			c.Expect(fi.Size(), gs.Equals, int64(856))
			close(wr.InChan)
		})

		c.Specify("writes a data point to the whisper file", func() {
			// Send a data point through and close.
			when := time.Now().UTC()
			val := float64(6)
			pt := whisper.NewPoint(when, val)
			wr.InChan <- &pt
			close(wr.InChan)

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
