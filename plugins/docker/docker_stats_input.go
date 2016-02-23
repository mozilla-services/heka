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
#   Anton Lindström (carlantonlindstrom@gmail.com)
#   Rob Miller (rmiller@mozilla.com)
#   Karl Matthias (karl.matthias@gonitro.com)
#   Guy Templeton (guy.templeton@skyscanner.net)
#
# ***** END LICENSE BLOCK *****/

package docker

import (
	"errors"
	"fmt"
	"reflect"
	"github.com/mozilla-services/heka/pipeline"
	"github.com/mozilla-services/heka/message"
	"github.com/pborman/uuid"
)

type DockerStatsInputConfig struct {
	// A Docker endpoint.
	Endpoint      		string   `toml:"endpoint"`
	CertPath      		string   `toml:"cert_path"`
	NameFromEnv   		string   `toml:"name_from_env_var"`
	FieldsFromEnv 		[]string `toml:"fields_from_env"`
	FieldsFromLabels 	[]string `toml:"fields_from_labels"`
}

type DockerStatsInput struct {
	conf         *DockerStatsInputConfig
	stopChan     chan error
	closer       chan struct{}
	statsstream  chan *DockerStat
	attachErrors chan error
	statsMgr     *StatsManager
	pack	     *pipeline.PipelinePack
}

func (di *DockerStatsInput) ConfigStruct() interface{} {
	return &DockerStatsInputConfig{
		Endpoint: "unix:///var/run/docker.sock",
		CertPath: "",
	}
}

func (di *DockerStatsInput) Init(config interface{}) error {

	di.conf = config.(*DockerStatsInputConfig)
	di.stopChan = make(chan error)
	di.closer = make(chan struct{})
	di.statsstream = make(chan *DockerStat)
	di.attachErrors = make(chan error)

	m, err := NewStatsManager(di.conf.Endpoint, di.conf.CertPath, di.attachErrors, di.conf.NameFromEnv, di.conf.FieldsFromEnv, di.conf.FieldsFromLabels)
	if err != nil {
		return fmt.Errorf("DockerStatsInput: failed to attach: %s", err.Error())
	}

	di.statsMgr = m
	return nil
}

func (di *DockerStatsInput) Run(ir pipeline.InputRunner, h pipeline.PluginHelper) error {
	var ok   bool
	di.statsMgr.ir = ir
	hostname := h.Hostname()

	go di.statsMgr.Run(di.statsstream, di.closer, di.stopChan)

	err := withRetries(func() error { return di.statsMgr.client.AddEventListener(di.statsMgr.events) })
	// Get the InputRunner's chan to receive empty PipelinePacks
	packSupply := ir.InChan()

	ok = true
	for ok {
		select {
		case statsline := <-di.statsstream:
			di.pack = <-packSupply

			di.pack.Message.SetType("DockerStats")
			di.pack.Message.SetLogger(statsline.Container)
			di.pack.Message.SetHostname(hostname)   // Use the host's hosntame
			di.pack.Message.SetPayload(statsline.StatsString)
			di.pack.Message.SetTimestamp(statsline.Time.UnixNano())
			di.pack.Message.SetUuid(uuid.NewRandom())
			di.MarshalStatsFields("stat", reflect.ValueOf(statsline.Stat), ir)

			for name, value := range statsline.Fields {
				field, err := message.NewField(name, value, "")
				if err != nil {
					ir.LogError(
						fmt.Errorf("can't add '%s' field: %s", name, err.Error()),
					)
					continue
				}

				di.pack.Message.AddField(field)
			}
			ir.Deliver(di.pack)


		case err, ok = <-di.attachErrors:
			if !ok {
				err = errors.New("Docker event channel closed")
				break
			}
			ir.LogError(fmt.Errorf("Attacher error: %s", err))

		case err = <-di.stopChan:
			ok = false
		}
	}

	di.closer <- struct{}{}
	close(di.statsstream)
	if err != nil {
		ir.LogError(err)
		return errors.New(
			"Failed to attach to Docker containers after retrying. Plugin giving up.")
	}
	if err != nil {
		ir.LogError(err)
		return errors.New(
			"Failed to add Docker event listener after retrying. Plugin giving up.")
	}
	return nil
}

func (di *DockerStatsInput) CleanupForRestart() {
	// Intentionally left empty. Cleanup happens in Run()
}

func (di *DockerStatsInput) Stop() {
	close(di.stopChan)
}

func init() {
	pipeline.RegisterPlugin("DockerStatsInput", func() interface{} {
		return new(DockerStatsInput)
	})
}


// This method marshals the fields of a Stats struct into a map[string]string fields object
// Core logic for handling reflect.value kinds based on
// https://github.com/golang/go/blob/dbaf5010b33e5819050ebb0a387eb0bff2cfb8bf/src/fmt/scan.go#L994
func (di *DockerStatsInput) MarshalStatsFields(path string, v reflect.Value, ir pipeline.InputRunner) {
	switch v.Kind() {
	case reflect.Ptr:
		di.MarshalStatsFields(path, v.Elem(), ir)
	case reflect.Invalid:
		field, _ := message.NewField(path, "invalid", "")
		di.pack.Message.AddField(field)
	case reflect.Slice, reflect.Array:
		for i := 0; i < v.Len(); i++ {
			di.MarshalStatsFields(fmt.Sprintf("%s[%d]", path, i), v.Index(i), ir)
		}
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			fieldPath := fmt.Sprintf("%s-%s", path, v.Type().Field(i).Name)
			di.MarshalStatsFields(fieldPath, v.Field(i), ir)
		}
	// Handle each item within a map
	case reflect.Map:
		for _, key := range v.MapKeys() {
			di.MarshalStatsFields(fmt.Sprintf("%s[%s]", path,
				v.String()), v.MapIndex(key), ir)
		}
	case reflect.Interface:
		if v.IsNil() {
			field, _ := message.NewField(path, "nil", "")
			di.pack.Message.AddField(field)
		} else {
			field, _ := message.NewField(path, v.Elem().Type().String(), "")
			di.pack.Message.AddField(field)
			di.MarshalStatsFields(path + ".value", v.Elem(), ir)
		}
	default: //Pack the values of basic types in
		di.packValue(path, v, ir)
	}
}

func (di *DockerStatsInput) packValue(path string, v reflect.Value, ir pipeline.InputRunner) {
	var (
		err error
		field *message.Field
	)
	//Switch on the kind of value passed in
	switch v.Kind() {
	case reflect.Invalid:
		field, err = message.NewField(path, "invalid", "")
		di.pack.Message.AddField(field)
	case reflect.Int, reflect.Int8, reflect.Int16,
		reflect.Int32, reflect.Int64:
		field, err = message.NewField(path, v.Int(), "")
	case reflect.Uint, reflect.Uint8, reflect.Uint16,
		reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		// Uints are currently unsupported, so just return out
		return
	case reflect.String:
		field, err = message.NewField(path, v.String(), "")
	case reflect.Bool: //Currently no Bools in Stats but handle them anyway
		field, err = message.NewField(path, v.Bool(), "")
	default: // Arrays, slices (structs and interfaces will error)
		field, err = message.NewField(path, v.Type().String() + " value", "")
	}
	// Handle errors on packing - probably an unsupported type
	if err != nil {
			ir.LogError(
				fmt.Errorf("can't add '%s' field: %s", path, err.Error()),
			)
			return
	}
	di.pack.Message.AddField(field)
}