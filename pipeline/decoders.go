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
#   Mike Trinkala (trink@mozilla.com)
#
# ***** END LICENSE BLOCK *****/

package pipeline

import (
	"code.google.com/p/go-uuid/uuid"
	"code.google.com/p/goprotobuf/proto"
	"encoding/json"
	"fmt"
	"github.com/mozilla-services/heka/message"
	"log"
	"sync"
)

type DecoderSet interface {
	ByName(name string) (decoder DecoderRunner, ok bool)
	ByEncoding(enc message.Header_MessageEncoding) (decoder DecoderRunner, ok bool)
	AllByName() (decoders map[string]DecoderRunner)
}

type decoderSet struct {
	byName     map[string]DecoderRunner
	byEncoding []DecoderRunner
}

func newDecoderSet(wrappers map[string]*PluginWrapper) (ds *decoderSet, err error) {
	length := int32(topHeaderMessageEncoding) + 1
	ds = &decoderSet{
		byName:     make(map[string]DecoderRunner),
		byEncoding: make([]DecoderRunner, length),
	}
	var (
		d       Decoder
		dInt    interface{}
		dRunner DecoderRunner
		enc     message.Header_MessageEncoding
		name    string
		w       *PluginWrapper
		ok      bool
	)
	for name, w = range wrappers {
		if dInt, err = w.CreateWithError(); err != nil {
			return nil, fmt.Errorf("Failed creating decoder %s: %s", name, err)
		}
		if d, ok = dInt.(Decoder); !ok {
			return nil, fmt.Errorf("Not Decoder type: %s", name)
		}
		dRunner = NewDecoderRunner(name, d)
		ds.byName[name] = dRunner
	}
	for enc, name = range DecodersByEncoding {
		if dRunner, ok = ds.byName[name]; !ok {
			return nil, fmt.Errorf("Encoding registered decoder doesn't exist: %s",
				name)
		}
		ds.byEncoding[enc] = dRunner
	}
	return
}

func (ds *decoderSet) ByName(name string) (decoder DecoderRunner, ok bool) {
	decoder, ok = ds.byName[name]
	return
}

func (ds *decoderSet) ByEncoding(enc message.Header_MessageEncoding) (
	decoder DecoderRunner, ok bool) {

	iEnc := int(enc)
	if !(iEnc >= 0 && iEnc < len(ds.byEncoding)) {
		return
	}
	if decoder = ds.byEncoding[enc]; decoder != nil {
		ok = true
	}
	return
}

func (ds *decoderSet) AllByName() (decoders map[string]DecoderRunner) {
	return ds.byName
}

type DecoderRunner interface {
	PluginRunner
	Decoder() Decoder
	Start(h PluginHelper, wg *sync.WaitGroup)
	InChan() chan *PipelinePack
	UUID() string
}

type dRunner struct {
	pRunnerBase
	inChan chan *PipelinePack
	uuid   string
}

func NewDecoderRunner(name string, decoder Decoder) DecoderRunner {
	return &dRunner{
		pRunnerBase: pRunnerBase{name: name, plugin: decoder.(Plugin)},
		uuid:        uuid.NewRandom().String(),
		inChan:      make(chan *PipelinePack, Globals().PluginChanSize),
	}
}

func (dr *dRunner) Decoder() Decoder {
	return dr.plugin.(Decoder)
}

func (dr *dRunner) Start(h PluginHelper, wg *sync.WaitGroup) {
	go func() {
		var pack *PipelinePack

		defer func() {
			if r := recover(); r != nil {
				dr.LogError(fmt.Errorf("PANIC: %s", r))
				if pack != nil {
					pack.Recycle()
				}
				if Globals().Stopping {
					wg.Done()
				} else {
					dr.Start(h, wg)
				}
			}
		}()

		var err error
		for pack = range dr.inChan {
			if err = dr.Decoder().Decode(pack); err != nil {
				dr.LogError(err)
				pack.Recycle()
				continue
			}
			pack.Decoded = true
			h.Router().InChan() <- pack
		}
		dr.LogMessage("stopped")
		wg.Done()
	}()
}

func (dr *dRunner) InChan() chan *PipelinePack {
	return dr.inChan
}

func (dr *dRunner) UUID() string {
	return dr.uuid
}

func (dr *dRunner) LogError(err error) {
	log.Printf("Decoder '%s' error: %s", dr.name, err)
}

func (dr *dRunner) LogMessage(msg string) {
	log.Printf("Decoder '%s': %s", dr.name, msg)
}

type Decoder interface {
	Decode(pack *PipelinePack) error
}

type JsonDecoder struct{}

func (self *JsonDecoder) Init(config interface{}) error {
	return nil
}

func (self *JsonDecoder) Decode(pack *PipelinePack) error {
	return json.Unmarshal(pack.MsgBytes, pack.Message)
}

type ProtobufDecoder struct{}

func (self *ProtobufDecoder) Init(config interface{}) error {
	return nil
}

func (self *ProtobufDecoder) Decode(pack *PipelinePack) error {
	return proto.Unmarshal(pack.MsgBytes, pack.Message)
}
