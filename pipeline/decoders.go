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
	"errors"
	"fmt"
	"github.com/bitly/go-simplejson"
	"github.com/mozilla-services/heka/message"
	"log"
	"sync"
	"time"
)

type DecoderSource interface {
	NewDecoder(name string) (decoder DecoderRunner, ok bool)
	NewDecoders() (decoders map[string]DecoderRunner)
	NewDecodersByEncoding() (decoders []DecoderRunner)
	RunningDecoders() (decoders map[string]DecoderRunner)
}

type decoderManager struct {
	config    *PipelineConfig
	decoders  map[string]DecoderRunner
	stopped   map[string]DecoderRunner
	lock      *sync.Mutex
	wg        *sync.WaitGroup
	ownerName string
}

func newDecoderManager(config *PipelineConfig, ownerName string) (
	dm *decoderManager) {

	return &decoderManager{
		config:    config,
		wg:        &config.decodersWg,
		ownerName: ownerName,
		decoders:  make(map[string]DecoderRunner),
		stopped:   make(map[string]DecoderRunner),
		lock:      new(sync.Mutex),
	}
}

// Instantiates a single DecoderRunner of the given name. `ok` value of
// `false` means no decoder of the given name was registered in the config.
func (dm *decoderManager) makeDecoder(name string) (dRunner DecoderRunner, ok bool) {
	if dRunner, ok = dm.fromStopped(name); ok {
		return
	}
	var wrapper *PluginWrapper
	if wrapper, ok = dm.config.DecoderWrappers[name]; ok {
		decoder := wrapper.Create().(Decoder)
		dRunner = NewDecoderRunner(name, decoder, dm)
		dm.wg.Add(1)
		dRunner.Start(dm.config, dm.wg)
	}
	return
}

// Checks to see if we have a decoder of the given name among our stopped
// decoders, start and return if so.
func (dm *decoderManager) fromStopped(name string) (dRunner DecoderRunner, ok bool) {
	dm.lock.Lock()
	for uuid, dr := range dm.stopped {
		if dr.Name() == name {
			dRunner = dr
			ok = true
			delete(dm.stopped, uuid)
			break
		}
	}
	dm.lock.Unlock()
	if ok {
		dm.wg.Add(1)
		dRunner.Start(dm.config, dm.wg)
	}
	return
}

// Thread-safe add to registry of running decoders.
func (dm *decoderManager) regDecoders(decoders []DecoderRunner) {
	if len(decoders) == 0 {
		return
	}
	var name string
	dm.lock.Lock()
	defer dm.lock.Unlock()
	for _, d := range decoders {
		dm.decoders[d.UUID()] = d
		name = fmt.Sprintf("%s-%s-%s", dm.ownerName, d.Name(), d.UUID()[:6])
		d.SetName(name)
	}
}

// Thread safe removal from registry of running decoders.
func (dm *decoderManager) unregDecoder(uuid string) {
	dm.lock.Lock()
	defer dm.lock.Unlock()
	if decoder, ok := dm.decoders[uuid]; ok {
		decoder.SetName(decoder.OrigName())
		dm.stopped[uuid] = decoder
		delete(dm.decoders, uuid)
	}
}

// Instantiates a single DecoderRunner of the given name and registers it in
// this manager's set of running decoders. `ok` value of `false` means no
// decoder of the given name was registered in the config.
func (dm *decoderManager) NewDecoder(name string) (decoder DecoderRunner, ok bool) {
	if decoder, ok = dm.makeDecoder(name); ok {
		decoders := []DecoderRunner{decoder}
		dm.regDecoders(decoders)
	}
	return
}

// Creates and starts one of every decoder type registered in the config and
// adds them to this manager's set of running decoders. Return map is keyed by
// decoder name.
func (dm *decoderManager) NewDecoders() (decoders map[string]DecoderRunner) {
	decoders = make(map[string]DecoderRunner)
	dSlice := make([]DecoderRunner, 0, len(dm.config.DecoderWrappers))
	var (
		runner  DecoderRunner
		decoder Decoder
		ok      bool
	)
	// Nested `for` loops below, might be worth doing more efficiently.
	for name, wrapper := range dm.config.DecoderWrappers {
		if runner, ok = dm.fromStopped(name); !ok {
			decoder = wrapper.Create().(Decoder)
			runner = NewDecoderRunner(name, decoder, dm)
			dm.wg.Add(1)
			runner.Start(dm.config, dm.wg)
		}
		decoders[name] = runner
		dSlice = append(dSlice, runner)
	}
	dm.regDecoders(dSlice)
	return
}

// Creates and starts one of every decoder type which has been registered to
// be used for a specific `message.Header_MessageEncoding` value. Return slice
// is indexed by these `Header_MessageEncoding` values.
func (dm *decoderManager) NewDecodersByEncoding() (decoders []DecoderRunner) {
	decoders = make([]DecoderRunner, topHeaderMessageEncoding+1)
	for encoding, name := range DecodersByEncoding {
		decoder, ok := dm.makeDecoder(name)
		if !ok {
			continue
		}
		decoders[encoding] = decoder
	}
	dm.regDecoders(decoders)
	return
}

func (dm *decoderManager) RunningDecoders() (decoders map[string]DecoderRunner) {
	return dm.decoders
}

type DecoderRunner interface {
	PluginRunner
	Decoder() Decoder
	Start(h PluginHelper, wg *sync.WaitGroup)
	InChan() chan *PipelinePack
	UUID() string
	OrigName() string
}

type dRunner struct {
	pRunnerBase
	origName string
	inChan   chan *PipelinePack
	uuid     string
	mgr      *decoderManager
}

func NewDecoderRunner(name string, decoder Decoder, mgr *decoderManager) DecoderRunner {
	return &dRunner{
		pRunnerBase: pRunnerBase{name: name, plugin: decoder.(Plugin)},
		origName:    name,
		uuid:        uuid.NewRandom().String(),
		mgr:         mgr,
	}
}

func (dr *dRunner) Decoder() Decoder {
	return dr.plugin.(Decoder)
}

func (dr *dRunner) Start(h PluginHelper, wg *sync.WaitGroup) {
	dr.inChan = make(chan *PipelinePack, PIPECHAN_BUFSIZE)
	go func() {
		var pack *PipelinePack

		defer func() {
			if r := recover(); r != nil {
				dr.LogError(fmt.Errorf("PANIC: %s", r))
				if pack != nil {
					pack.Recycle()
				}
				if Stopping {
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
			h.Router().InChan <- pack
		}
		dr.mgr.unregDecoder(dr.uuid)
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

func (dr *dRunner) OrigName() string {
	return dr.origName
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

func flattenValue(v interface{}, msg *message.Message, path string) error {
	switch v.(type) {
	case string, float64, bool:
		f, _ := message.NewField(path, v, message.Field_RAW)
		msg.AddField(f)
	case []interface{}:
		err := flattenArray(v.([]interface{}), msg, path)
		if err != nil {
			return err
		}
	case map[string]interface{}:
		err := flattenMap(v.(map[string]interface{}), msg, path)
		if err != nil {
			return err
		}
	default:
		return fmt.Errorf("Path %s, unsupported value type: %T", path, v)
	}
	return nil
}

func flattenArray(a []interface{}, msg *message.Message, path string) error {
	if len(a) > 0 {
		switch a[0].(type) {
		case string, float64, bool:
			f, _ := message.NewField(path, a[0], message.Field_RAW)
			for _, v := range a[1:] {
				err := f.AddValue(v)
				if err != nil {
					return err
				}
			}
			msg.AddField(f)

		default:
			var childPath string
			for i, v := range a {
				childPath = fmt.Sprintf("%s.%d", path, i)
				err := flattenValue(v, msg, childPath)
				if err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func flattenMap(m map[string]interface{}, msg *message.Message, path string) error {
	var childPath string
	for k, v := range m {
		if len(path) == 0 {
			childPath = k
		} else {
			childPath = fmt.Sprintf("%s.%s", path, k)
		}
		err := flattenValue(v, msg, childPath)
		if err != nil {
			return err
		}
	}
	return nil
}

func (self *JsonDecoder) Decode(pipelinePack *PipelinePack) error {
	msgBytes := pipelinePack.MsgBytes
	msgJson, err := simplejson.NewJson(msgBytes)
	if err != nil {
		return err
	}

	msg := pipelinePack.Message
	uuidString, _ := msgJson.Get("uuid").String()
	u := uuid.Parse(uuidString)
	msg.SetUuid(u)
	msg.SetType(msgJson.Get("type").MustString())
	timeStr := msgJson.Get("timestamp").MustString()
	t, err := time.Parse(time.RFC3339Nano, timeStr)
	if err != nil {
		log.Printf("Timestamp parsing error: %s\n", err.Error())
		return errors.New("invalid Timestamp")
	}
	msg.SetTimestamp(t.UnixNano())
	msg.SetLogger(msgJson.Get("logger").MustString())
	msg.SetSeverity(int32(msgJson.Get("severity").MustInt()))
	msg.SetPayload(msgJson.Get("payload").MustString())
	msg.SetEnvVersion(msgJson.Get("env_version").MustString())
	i, _ := msgJson.Get("metlog_pid").Int()
	msg.SetPid(int32(i))
	msg.SetHostname(msgJson.Get("metlog_hostname").MustString())
	fields, _ := msgJson.Get("fields").Map()
	err = flattenMap(fields, msg, "")
	if err != nil {
		return err
	}
	return nil
}

type ProtobufDecoder struct{}

func (self *ProtobufDecoder) Init(config interface{}) error {
	return nil
}

func (self *ProtobufDecoder) Decode(pack *PipelinePack) error {
	err := proto.Unmarshal(pack.MsgBytes, pack.Message)
	if err != nil {
		return fmt.Errorf("unmarshaling error: ", err)
	}
	return nil
}
