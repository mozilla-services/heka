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
#
# ***** END LICENSE BLOCK *****/

package pipeline

// WantsName indicates a plug-in needs its name before it has access to the
// runner interface.
type WantsName interface {
	// Passes the toml section name into the plugin at configuration time.
	SetName(name string)
}

// WantsPipelineConfig indicates that a plugin wants access to the
// PipelineConfig before any methods on the plugin are called. This (and the
// other `Wants*` interfaces) is a bit kludgey, but it will have to do until
// we overhaul the config loading code to make all of the right components
// automatically available to each plugin earlier in the life cycle, which
// will involve small breaking changes to the core plugin APIs.
type WantsPipelineConfig interface {
	SetPipelineConfig(pConfig *PipelineConfig)
}

// Restarting indicates a plug-in can handle being restart should it exit
// before heka is shut-down.
type Restarting interface {
	// Is called anytime the plug-in returns during the main Run loop to
	// clean up the plug-in state and determine whether the plugin should
	// be restarted or not.
	CleanupForRestart()
}

// Stoppable indicates a plug-in can stop without causing a heka shut-down
// Callers should first check IsStoppable, and if it returns true, Unregister
// should be called to remove it from Heka while running.
type Stoppable interface {
	IsStoppable() bool
	Unregister(pConfig *PipelineConfig) error
}

type notStoppable struct{}

func (s *notStoppable) IsStoppable() bool {
	return false
}

func (s *notStoppable) Unregister(pConfig *PipelineConfig) error {
	return nil
}
