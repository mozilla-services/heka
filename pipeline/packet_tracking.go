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
	"fmt"
	"log"
	"time"
)

// Diagnostic object for packet tracking
type PacketTracking struct {
	// Records last accessed time
	LastAccess time.Time

	// Records the plugins the packet has been handed to
	lastPlugins []PluginRunner
}

// Create a new blank PacketTracking
func NewPacketTracking() *PacketTracking {
	return &PacketTracking{time.Now(), make([]PluginRunner, 0, 8)}
}

// Stamps a packet with the tracking data for the plugin its handed to,
// clearing any existing stamps
func (p *PacketTracking) Stamp(pluginRunner PluginRunner) {
	p.lastPlugins = p.lastPlugins[:0]
	p.lastPlugins = append(p.lastPlugins, pluginRunner)
	p.LastAccess = time.Now()
}

// Adds a stamp to the packet
func (p *PacketTracking) AddStamp(pluginRunner PluginRunner) {
	p.lastPlugins = append(p.lastPlugins, pluginRunner)
	p.LastAccess = time.Now()
}

// Resets the packet stamping
func (p *PacketTracking) Reset() {
	p.lastPlugins = p.lastPlugins[:0]
	p.LastAccess = time.Now()
}

// Returns the names of the plugins that have last accessed the packet
func (p *PacketTracking) PluginNames() (names []string) {
	names = make([]string, 0, 4)
	for _, pr := range p.lastPlugins {
		names = append(names, pr.Name())
	}
	return
}

// Returns the names of the plugin runners that last access the packet
func (p *PacketTracking) Runners() (runners []PluginRunner) {
	return p.lastPlugins
}

// A diagnostic tracker that can track pipeline packs and do accounting
// to determine possible leaks
type DiagnosticTracker struct {
	// Track all the packs that have been created
	packs []*PipelinePack

	// Identify the name of the recycle channel it monitors packs for
	ChannelName string
}

// Create and return a new diagnostic tracker
func NewDiagnosticTracker(channelName string) *DiagnosticTracker {
	return &DiagnosticTracker{make([]*PipelinePack, 0, 50), channelName}
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
	g := Globals()
	idleMax := g.MaxPackIdle
	probablePacks := make([]*PipelinePack, 0, len(d.packs))
	ticker := time.NewTicker(time.Duration(30) * time.Second)
	for {
		<-ticker.C
		probablePacks = probablePacks[:0]
		pluginCounts = make(map[PluginRunner]int)

		// Locate all the packs that have not been touched in idleMax duration
		// that are not recycled
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

		// Drop a warning about how many packs have been idle
		if len(probablePacks) > 0 {
			g.LogMessage("Diagnostics", fmt.Sprintf("%d packs have been idle more than %d seconds.",
				d.ChannelName, len(probablePacks), idleMax))
			g.LogMessage("Diagnostics", fmt.Sprintf("(%s) Plugin names and quantities found on idle packs:",
				d.ChannelName))
			for runner, count = range pluginCounts {
				runner.SetLeakCount(count)
				g.LogMessage("Diagnostics", fmt.Sprintf("\t%s: %d", runner.Name(), count))
			}
			log.Println("")
		}
	}
}
