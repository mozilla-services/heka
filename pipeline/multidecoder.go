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
#   Victor Ng (vng@mozilla.com)
#   Rob Miller (rmiller@mozilla.com)
#
# ***** END LICENSE BLOCK *****/

package pipeline

import (
	"errors"
	"fmt"
	"log"
)

// DecoderRunner wrapper that the MultiDecoder will hand to any subs that ask
// for one. Shadows some data and methods, but doesn't spin up any goroutines.
type mDRunner struct {
	*dRunner
	decoder Decoder
	name    string
	subName string
}

func (mdr *mDRunner) Name() string {
	return mdr.name
}

func (mdr *mDRunner) SetName(name string) {
	mdr.name = name
}

func (mdr *mDRunner) Plugin() Plugin {
	return mdr.decoder.(Plugin)
}

func (mdr *mDRunner) Decoder() Decoder {
	return mdr.decoder
}

func (mdr *mDRunner) LogError(err error) {
	log.Printf("SubDecoder '%s' error: %s", mdr.name, err)
}

func (mdr *mDRunner) LogMessage(msg string) {
	log.Printf("SubDecoder '%s': %s", mdr.name, msg)
}

type MultiDecoder struct {
	Config    *MultiDecoderConfig
	Name      string
	Decoders  map[string]Decoder
	Ordered   []Decoder
	dRunner   DecoderRunner
	CascStrat int
}

type MultiDecoderConfig struct {
	Subs            []string
	LogSubErrors    bool   `toml:"log_sub_errors"`
	CascadeStrategy string `toml:"cascade_strategy"`
}

const (
	CASC_FIRST_WINS = iota
	CASC_ALL
)

var mdStrategies = map[string]int{"first-wins": CASC_FIRST_WINS, "all": CASC_ALL}

func (md *MultiDecoder) ConfigStruct() interface{} {
	subs := make([]string, 0)
	return &MultiDecoderConfig{subs, false, "first-wins"}
}

// Heka will call this before calling Init() to set the name of the
// MultiDecoder based on the section name in the TOML config.
func (md *MultiDecoder) SetName(name string) {
	md.Name = name
}

func (md *MultiDecoder) Init(config interface{}) (err error) {
	md.Config = config.(*MultiDecoderConfig)

	if len(md.Config.Subs) == 0 {
		return errors.New("At least one subdecoder must be specified.")
	}

	md.Decoders = make(map[string]Decoder)
	md.Ordered = make([]Decoder, len(md.Config.Subs))
	pConfig := Globals().PipelineConfig

	var (
		ok      bool
		decoder Decoder
	)

	if md.CascStrat, ok = mdStrategies[md.Config.CascadeStrategy]; !ok {
		return fmt.Errorf("Unrecognized cascade strategy: %s", md.Config.CascadeStrategy)
	}

	for i, name := range md.Config.Subs {
		if decoder, ok = pConfig.Decoder(name); !ok {
			return fmt.Errorf("Non-existent subdecoder: %s", name)
		}
		md.Decoders[name] = decoder
		md.Ordered[i] = decoder
	}

	return nil
}

// Heka will call this to give us access to the runner. We'll store it for
// ourselves, but also have to pass on a wrapped version to any subdecoders
// that might need it.
func (md *MultiDecoder) SetDecoderRunner(dr DecoderRunner) {
	md.dRunner = dr
	for subName, decoder := range md.Decoders {
		if wanter, ok := decoder.(WantsDecoderRunner); ok {
			// It wants a DecoderRunner, have to create one. But first we need
			// to get our hands on a *dRunner.
			var realDRunner *dRunner
			if realDRunner, ok = dr.(*dRunner); !ok {
				// It's not a *dRunner, maybe it's an *mDRunner?
				var mdr *mDRunner
				if mdr, ok = dr.(*mDRunner); ok {
					// Bingo, we can grab its *dRunner.
					realDRunner = mdr.dRunner
				}
			}
			if realDRunner == nil {
				// Couldn't get a *dRunner. Just log an error and pass the
				// outer DecoderRunner through.
				dr.LogError(fmt.Errorf("Can't create nested DecoderRunner for '%s'",
					subName))
				wanter.SetDecoderRunner(dr)
				continue
			}
			// We have a *dRunner, use it to create an *mDRunner.
			subRunner := &mDRunner{
				realDRunner,
				decoder,
				fmt.Sprintf("%s-%s", realDRunner.name, subName),
				subName,
			}
			wanter.SetDecoderRunner(subRunner)
		}
	}
}

// Heka will call this at DecoderRunner shutdown time, we might need to pass
// this along to subdecoders.
func (md *MultiDecoder) Shutdown() {
	for _, decoder := range md.Decoders {
		if wanter, ok := decoder.(WantsDecoderRunnerShutdown); ok {
			wanter.Shutdown()
		}
	}
}

// Recurses through a decoder chain, decoding the original pack and returning
// it and any generated extra packs.
func (md *MultiDecoder) getDecodedPacks(chain []Decoder, inPacks []*PipelinePack) (
	packs []*PipelinePack, anyMatch bool) {

	decoder := chain[0]
	for _, p := range inPacks {
		if ps, err := decoder.Decode(p); ps != nil {
			anyMatch = true
			packs = append(packs, ps...)
		} else {
			if err != nil && md.Config.LogSubErrors {
				idx := len(md.Ordered) - len(chain)
				err = fmt.Errorf("Subdecoder '%s' decode error: %s",
					md.Config.Subs[idx], err)
				md.dRunner.LogError(err)
			}
			packs = append(packs, p)
		}
	}

	if len(chain) > 1 {
		var otherMatch bool
		packs, otherMatch = md.getDecodedPacks(chain[1:], packs)
		anyMatch = anyMatch || otherMatch
	}

	return
}

// Runs the message payload against each of the decoders.
func (md *MultiDecoder) Decode(pack *PipelinePack) (packs []*PipelinePack, err error) {

	if md.CascStrat == CASC_FIRST_WINS {
		for i, d := range md.Ordered {
			if packs, err = d.Decode(pack); packs != nil {
				return
			}
			if err != nil && md.Config.LogSubErrors {
				err = fmt.Errorf("Subdecoder '%s' decode error: %s", md.Config.Subs[i],
					err)
				md.dRunner.LogError(err)
			}
		}
		// If we got this far none of the decoders succeeded.
		err = errors.New("All subdecoders failed.")
		packs = nil
	} else {
		// If we get here we know cascade_strategy == "all.
		var anyMatch bool
		packs, anyMatch = md.getDecodedPacks(md.Ordered, []*PipelinePack{pack})
		if !anyMatch {
			err = errors.New("All subdecoders failed.")
			packs = nil
		}
	}
	return
}

func init() {
	RegisterPlugin("MultiDecoder", func() interface{} {
		return new(MultiDecoder)
	})
}
