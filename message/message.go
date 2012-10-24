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
package message

import (
	"time"
)

type Message struct {
	Type        string
	Timestamp   time.Time
	Logger      string
	Severity    int
	Payload     string
	Fields      map[string]interface{}
	Env_version string
	Pid         int
	Hostname    string
}

// Copies a message to a newly initialized Message, including a deep
// copy of the Fields
func (self *Message) Copy(dst *Message) {
	*dst = *self
	dst.Fields = make(map[string]interface{})
	for k, v := range self.Fields {
		dst.Fields[k] = v
	}
}
