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
	"crypto/rand"
	"errors"
	"math/big"
	"time"
)

var ErrMaxRetriesExceeded = errors.New("Max retries exceeded")

// This struct provides a structure for the available retry options for a
// RetryHelper.
type RetryOptions struct {
	// Maximum time in seconds between restart attempts. Defaults to 30s.
	MaxDelay string `toml:"max_delay"`
	// Starting delay in milliseconds between restart attempts. Defaults to
	// 250ms.
	Delay string
	// Maximum jitter added to every retry attempt. Defaults to 500ms.
	MaxJitter string `toml:"max_jitter"`
	// How many times to attempt starting the plugin before failing. Defaults
	// to -1 (retry forever).
	MaxRetries int `toml:"max_retries"`
}

func getDefaultRetryOptions() RetryOptions {
	return RetryOptions{
		MaxDelay:   "30s",
		Delay:      "250ms",
		MaxRetries: -1,
	}
}

// Retry helper, created with a RetryOptions struct
//
// Everytime Wait is called, the times this has been used is incremented.
// Calling Reset will reset the time counter indicating the operation that
// was being retried succeeded.
type RetryHelper struct {
	maxDelay  time.Duration
	delay     time.Duration
	curDelay  time.Duration
	maxJitter time.Duration
	retries   int
	times     int
}

// Creates and returns a RetryHelper pointer to be used when retrying
// plugin restarts or other parts that require exponential backoff
func NewRetryHelper(opts RetryOptions) (helper *RetryHelper, err error) {
	if opts.Delay == "" {
		opts.Delay = "250ms"
	}
	if opts.MaxDelay == "" {
		opts.MaxDelay = "30s"
	}
	if opts.MaxJitter == "" {
		opts.MaxJitter = "500ms"
	}
	delay, err := time.ParseDuration(opts.Delay)
	if err != nil {
		return
	}
	maxDelay, err := time.ParseDuration(opts.MaxDelay)
	if err != nil {
		return
	}
	maxJitter, err := time.ParseDuration(opts.MaxJitter)
	if err != nil {
		return
	}
	helper = &RetryHelper{
		maxDelay:  maxDelay,
		delay:     delay,
		curDelay:  delay,
		retries:   opts.MaxRetries,
		maxJitter: maxJitter,
		times:     0,
	}
	return
}

// Wait for a retry
//
// If the max retries has been exceeded, an error will be returned
func (r *RetryHelper) Wait() error {
	if r.retries != -1 && r.times >= r.retries {
		return ErrMaxRetriesExceeded
	}
	jitter, _ := rand.Int(rand.Reader, big.NewInt(r.maxJitter.Nanoseconds()))
	jitterWait := time.Duration(jitter.Int64()) * time.Nanosecond
	timer := time.NewTimer(r.curDelay + jitterWait)
	select {
	case <-timer.C:
		break
	}
	r.curDelay *= 2
	r.times += 1
	if r.curDelay > r.maxDelay {
		r.curDelay = r.maxDelay
	}
	return nil
}

// Reset the retry counter
func (r *RetryHelper) Reset() {
	r.times = 0
	r.curDelay = r.delay
}
