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
#   Victor Ng (vng@mozilla.com)
#
# ***** END LICENSE BLOCK *****/

package pipeline

import (
    "fmt"
)

type MultiDecoder struct {
	decoders map[string]Decoder
	order    []string

	dRunner DecoderRunner
}

type MultiDecoderConfig struct {
	// subs is an ordered dictionary of other decoders
	subs  map[string]interface{}
	order []string
}

func (md *MultiDecoder) ConfigStruct() interface{} {
	subs := make(map[string]interface{})
	order := make([]string, 0)
	return &MultiDecoderConfig{subs: subs, order: order}
}

func (md *MultiDecoder) Init(config interface{}) (err error) {
	conf := config.(*MultiDecoderConfig)
	md.order = conf.order

	// TODO: run PrimitiveDecode against each subsection here and bind
	// it into the md.decoders map

	return nil
}

// Heka will call this to give us access to the runner.
func (md *MultiDecoder) SetDecoderRunner(dr DecoderRunner) {
	md.dRunner = dr
}

// Runs the message payload against each of the decoders
func (md *MultiDecoder) Decode(pack *PipelinePack) (err error) {
	var d Decoder
	for _, decoder_name := range md.order {
		d = md.decoders[decoder_name]
		if err = d.Decode(pack); err == nil {
			return
		}
	}
	return fmt.Errorf("Unable to decode message: [%s]", pack)
}
