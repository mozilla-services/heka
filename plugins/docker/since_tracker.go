/***** BEGIN LICENSE BLOCK *****
# This Source Code Form is subject to the terms of the Mozilla Public
# License, v. 2.0. If a copy of the MPL was not distributed with this file,
# You can obtain one at http://mozilla.org/MPL/2.0/.
#
# The Initial Developer of the Original Code is the Mozilla Foundation.
# Portions created by the Initial Developer are Copyright (C) 2014
# the Initial Developer. All Rights Reserved.
#
# Contributor(s):
#   Rob Miller (rmiller@mozilla.com)
#   Karl Matthias (karl.matthias@gonitro.com)
#
# ***** END LICENSE BLOCK *****/

package docker

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/mozilla-services/heka/pipeline"
)

// Tracks the timestamp of the last time we logged
// data from a particular container.
type SinceTracker struct {
	sync.Mutex
	Path       string        `json:"-"`
	Interval   time.Duration `json:"-"`
	ExpiryDays int           `json:"-"`
	Since      int64
	Containers map[string]int64
	ir         pipeline.InputRunner
}

// Return a fully configured SinceTracker.
func NewSinceTracker(expiryDays int, path string, interval time.Duration) (*SinceTracker, error) {
	sinceTracker := &SinceTracker{
		Path:       path,
		Interval:   interval,
		ExpiryDays: expiryDays,
	}

	if err := sinceTracker.Load(); err != nil {
		return nil, err
	}

	return sinceTracker, nil
}

// Load state from the since file into the SinceTracker.
func (s *SinceTracker) Load() error {
	// Initialize the sinces from the JSON since file.
	sinceFile, err := os.Open(s.Path)
	if err != nil {
		return fmt.Errorf("Can't open \"since\" file '%s': %s", s.Path, err.Error())
	}

	jsonDecoder := json.NewDecoder(sinceFile)

	s.Lock()
	err = jsonDecoder.Decode(s)
	s.Unlock()

	if err != nil {
		return fmt.Errorf("Can't decode \"since\" file '%s': %s", s.Path, err.Error())
	}

	return nil
}

// Write out the current data in the SinceTracker to the sinces file.
func (s *SinceTracker) Write(t time.Time) {
	sinceFile, err := os.Create(s.Path)
	if err != nil {
		s.ir.LogError(fmt.Errorf("Can't create \"since\" file '%s': %s", s.Path,
			err.Error()))
		return
	}
	jsonEncoder := json.NewEncoder(sinceFile)
	s.Lock()
	s.Since = t.Unix()

	// Eject since containers that are too old to keep so we don't build up
	// a list forever.
	for container, lastSeen := range s.Containers {
		if lastSeen == 0 {
			continue
		}

		if lastSeen < time.Now().Unix()-int64(s.ExpiryDays)*3600*24 {
			delete(s.Containers, container)
		}
	}

	// Whitelist the parts of the struct we save to disk
	outStruct := struct {
		Since      *int64
		Containers *map[string]int64
	}{&s.Since, &s.Containers}

	if err = jsonEncoder.Encode(outStruct); err != nil {
		s.ir.LogError(fmt.Errorf("Can't write to \"since\" file '%s': %s", s.Path,
			err.Error()))
	}

	s.Unlock()
	if err = sinceFile.Close(); err != nil {
		s.ir.LogError(fmt.Errorf("Can't close \"since\" file '%s': %s", s.Path,
			err.Error()))
	}
}

// Make sure the file exists and is writable
func EnsureSincesFile(conf *DockerLogInputConfig, sincePath string) error {
	// Make sure the since file exists.
	_, err := os.Stat(sincePath)
	if os.IsNotExist(err) {
		sinceDir := filepath.Dir(sincePath)
		if err = os.MkdirAll(sinceDir, 0700); err != nil {
			return fmt.Errorf("Can't create storage directory '%s': %s", sinceDir,
				err.Error())
		}

		sinceFile, err := os.Create(sincePath)
		if err != nil {
			return fmt.Errorf("Can't create \"since\" file '%s': %s", sincePath,
				err.Error())
		}

		jsonEncoder := json.NewEncoder(sinceFile)
		if err = jsonEncoder.Encode(&SinceTracker{Containers: make(map[string]int64)}); err != nil {
			return fmt.Errorf("Can't write to \"since\" file '%s': %s", sincePath,
				err.Error())
		}
		if err = sinceFile.Close(); err != nil {
			return fmt.Errorf("Can't close \"since\" file '%s': %s", sincePath,
				err.Error())
		}
	} else if err != nil {
		return fmt.Errorf("Can't open \"since\" file '%s': %s", sincePath, err.Error())
	}

	return nil
}
