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
	"github.com/kisielk/whisper-go/whisper"
	"os"
)

type WhisperOutput struct {
	db *whisper.Whisper
}

type WhisperOutputConfig struct {
	// Full file path to Whisper database file.
	Path              string
	AggregationMethod whisper.AggregationMethod
}

func (o *WhisperOutput) ConfigStruct() interface{} {
	return new(WhisperOutputConfig)
}

func (o *WhisperOutput) Init(config interface{}) (err error) {
	conf := config.(*WhisperOutputConfig)
	if o.db, err = whisper.Open(conf.Path); err != nil {
		if !os.IsNotExist(err) {
			// A real error, bail out.
			err = fmt.Errorf("Error opening whisper db: %s", err)
			return
		}
		// No db file, create it.
		if o.db, err = whisper.Create(conf.Path, nil, 0.1, conf.AggregationMethod,
			false); err != nil {
			err = fmt.Errorf("Error creating whisper db: %s", err)
			return
		}
	} else {
		if o.db.Header.Metadata.AggregationMethod != conf.AggregationMethod {
			if err = o.db.SetAggregationMethod(conf.AggregationMethod); err != nil {
				err = fmt.Errorf("Error setting whisper db agg method: %s", err)
				return
			}
		}
	}
}

func (o *WhisperOutput) Deliver(pack *PipelinePack) {

}
