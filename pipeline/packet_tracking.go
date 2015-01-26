/***** BEGIN LICENSE BLOCK *****
# This Source Code Form is subject to the terms of the Mozilla Public
# License, v. 2.0. If a copy of the MPL was not distributed with this file,
# You can obtain one at http://mozilla.org/MPL/2.0/.
#
# The Initial Developer of the Original Code is the Mozilla Foundation.
# Portions created by the Initial Developer are Copyright (C) 2012-2014
# the Initial Developer. All Rights Reserved.
#
# Contributor(s):
#   Ben Bangert (bbangert@mozilla.com)
#   Rob Miller (rmiller@mozilla.com)
#
# ***** END LICENSE BLOCK *****/

package pipeline

import (
	"fmt"
	"sync"
	"time"
)

// Diagnostic object for packet tracking.
type PacketTracking struct {
	// Last accessed time.
	LastAccess time.Time

	// Plugins the packet has been handed to.
	lastPlugins []PluginRunner

	// RWMutex to gate attribute access.
	rwmutex sync.RWMutex
}

// NewPacketTracking creates a new blank PacketTracking.
func NewPacketTracking() *PacketTracking {
	return &PacketTracking{
		LastAccess:  time.Now(),
		lastPlugins: make([]PluginRunner, 0, 8),
	}
}

// Stamp stamps a packet with the tracking data for the plugin its handed to,
// clearing any existing stamps.
func (p *PacketTracking) Stamp(pluginRunner PluginRunner) {
	p.rwmutex.Lock()
	p.lastPlugins = p.lastPlugins[:0]
	p.lastPlugins = append(p.lastPlugins, pluginRunner)
	p.LastAccess = time.Now()
	p.rwmutex.Unlock()
}

// AddStamp adds a stamp to the packet as having been handed to the specified
// plugin.
func (p *PacketTracking) AddStamp(pluginRunner PluginRunner) {
	p.rwmutex.Lock()
	p.lastPlugins = append(p.lastPlugins, pluginRunner)
	p.LastAccess = time.Now()
	p.rwmutex.Unlock()
}

// Reset resets the packet stamping metadata.
func (p *PacketTracking) Reset() {
	p.rwmutex.Lock()
	p.lastPlugins = p.lastPlugins[:0]
	p.LastAccess = time.Now()
	p.rwmutex.Unlock()
}

// PluginNames returns the names of the plugins that have last accessed the
// packet.
func (p *PacketTracking) PluginNames() (names []string) {
	names = make([]string, 0, 4)
	p.rwmutex.RLock()
	for _, pr := range p.lastPlugins {
		names = append(names, pr.Name())
	}
	p.rwmutex.RUnlock()
	return
}

// Runners returns the names of the plugin runners that last accessed the
// packet. The returned slice object is owned by the caller, but the
// underlying array and the contained PluginRunners are not thread safe and
// should NOT be mutated.
func (p *PacketTracking) Runners() (runners []PluginRunner) {
	p.rwmutex.RLock()
	runners = p.lastPlugins
	p.rwmutex.RUnlock()
	return runners
}

// A diagnostic tracker that can track pipeline packs and do accounting
// to determine possible leaks
type DiagnosticTracker struct {
	// Track all the packs that have been created.
	packs []*PipelinePack

	// Identify the name of the recycle channel it monitors packs for.
	ChannelName string

	// Pipeline configuration globals.
	globals *GlobalConfigStruct
}

// Create and return a new diagnostic tracker
func NewDiagnosticTracker(channelName string, globals *GlobalConfigStruct) *DiagnosticTracker {
	return &DiagnosticTracker{
		packs:       make([]*PipelinePack, 0, 50),
		ChannelName: channelName,
		globals:     globals,
	}
}

// Add a pipeline pack for monitoring
func (d *DiagnosticTracker) AddPack(pack *PipelinePack) {
	d.packs = append(d.packs, pack)
}

// Run the monitoring routine, this should be spun up in a new goroutine
func (d *DiagnosticTracker) Run() {
	var (
		pack           *PipelinePack
		earliestAccess time.Time
		pluginCounts   map[PluginRunner]int
		count          int
		runner         PluginRunner
	)
	g := d.globals
	idleMax := g.MaxPackIdle
	idleMaxSecs := int(idleMax.Seconds())
	probablePacks := make([]*PipelinePack, 0, len(d.packs))
	ticker := time.NewTicker(time.Duration(30) * time.Second)
	for {
		<-ticker.C
		probablePacks = probablePacks[:0]
		pluginCounts = make(map[PluginRunner]int)

		// Locate all the packs that have not been touched in idleMax duration
		// that are not recycled.
		earliestAccess = time.Now().Add(-idleMax)
		for _, pack = range d.packs {
			if len(pack.diagnostics.lastPlugins) == 0 {
				continue
			}
			if pack.diagnostics.LastAccess.Before(earliestAccess) {
				probablePacks = append(probablePacks, pack)
				for _, runner = range pack.diagnostics.Runners() {
					pluginCounts[runner] += 1
				}
			}
		}

		// Drop a warning about how many packs have been idle.
		if len(probablePacks) > 0 {
			g.LogMessage("Diagnostics",
				fmt.Sprintf("%d packs have been idle more than %d seconds.",
					len(probablePacks), idleMaxSecs))
			g.LogMessage("Diagnostics",
				fmt.Sprintf("(%s) Plugin names and quantities found on idle packs:",
					d.ChannelName))
			for runner, count = range pluginCounts {
				runner.SetLeakCount(count)
				g.LogMessage("Diagnostics", fmt.Sprintf("\t%s: %d", runner.Name(), count))
			}
			LogInfo.Println("")
		}
	}
}
