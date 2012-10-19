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
package hekagrater

import (
	"github.com/orfjackal/gospec/src/gospec"
	"heka/client"
	"reflect"
	"testing"
)

func TestAllSpecs(t *testing.T) {
	r := gospec.NewRunner()
	r.AddSpec(DecodersSpec)
	gospec.MainGoTest(r, t)
}

func (self *Message) Equals(o interface{}) bool {
	other := o.(*hekaclient.Message)
	soFar := self.Type == other.Type && self.Timestamp == other.Timestamp &&
		self.Logger == other.Logger && self.Severity == other.Severity &&
		self.Payload == other.Payload &&
		self.Env_version == other.Env_version && self.Pid == other.Pid &&
		self.Hostname == other.Hostname
	if !soFar {
		return false
	}
	return reflect.DeepEqual(self.Fields, other.Fields)
}