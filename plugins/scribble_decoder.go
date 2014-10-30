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

package plugins

import (
	. "github.com/mozilla-services/heka/pipeline"
)

type ScribbleDecoderConfig struct {
	// Map of message field names to message string values. Note that all
	// values *must* be strings. Any specified Pid and Severity field values
	// must be parseable as int32. All specified user fields will be created
	// as strings.
	MessageFields MessageTemplate `toml:"message_fields"`
}

type ScribbleDecoder struct {
	messageFields MessageTemplate
}

func (sd *ScribbleDecoder) ConfigStruct() interface{} {
	return new(ScribbleDecoderConfig)
}

func (sd *ScribbleDecoder) Init(config interface{}) (err error) {
	conf := config.(*ScribbleDecoderConfig)
	sd.messageFields = conf.MessageFields
	return
}

func (sd *ScribbleDecoder) Decode(pack *PipelinePack) (packs []*PipelinePack, err error) {
	if err = sd.messageFields.PopulateMessage(pack.Message, nil); err != nil {
		return
	}
	return []*PipelinePack{pack}, nil
}

func init() {
	RegisterPlugin("ScribbleDecoder", func() interface{} {
		return new(ScribbleDecoder)
	})
}
