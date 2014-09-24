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
#   Klaus Post (klauspost@gmail.com)
#   Rob Miller (rmiller@mozilla.com)
#
# ***** END LICENSE BLOCK *****/

package template

import (
	"bytes"
	"errors"
	"fmt"
	"github.com/mozilla-services/heka/message"
	"github.com/mozilla-services/heka/pipeline"
	"log"
	"os"
	"strconv"
	"text/template"
	"time"
)

type MsgSubs struct {
	Uuid       string
	Timestamp  string
	Type       string
	Logger     string
	Severity   string
	Payload    string
	EnvVersion string
	Pid        string
	Hostname   string
	Fields     map[string]string
}

type TextTemplateEncoderConfig struct {
	TemplateFiles   []string `toml:"template_files"`
	ReloadTemplates bool     `toml:"reload_templates"`
	ReloadInterval  int      `toml:"reload_interval"`
	TimestampFormat string   `toml:"timestamp_format"`
}

type TextTemplateEncoder struct {
	*TextTemplateEncoderConfig
	template     *template.Template
	lastFileTime []time.Time
	nextCheck    time.Time
	pConfig      *pipeline.PipelineConfig
}

func (e *TextTemplateEncoder) ConfigStruct() interface{} {
	config := &TextTemplateEncoderConfig{
		ReloadTemplates: true,
		ReloadInterval:  60,
		TimestampFormat: "2006-01-02T15:04:05Z07:00",
	}
	return config
}

func (e *TextTemplateEncoder) SetPipelineConfig(pConfig *pipeline.PipelineConfig) {
	e.pConfig = pConfig
}

func (e *TextTemplateEncoder) Init(config interface{}) error {
	e.TextTemplateEncoderConfig = config.(*TextTemplateEncoderConfig)
	if len(e.TemplateFiles) == 0 {
		return errors.New("Must specify one or more template_files.")
	}
	for i, path := range e.TemplateFiles {
		e.TemplateFiles[i] = e.pConfig.Globals.PrependShareDir(path)
	}
	e.lastFileTime = make([]time.Time, len(e.TemplateFiles))
	e.nextCheck = time.Now()
	return e.UpdateTemplate()
}

func (e *TextTemplateEncoder) Encode(pack *pipeline.PipelinePack) (output []byte, err error) {
	now := time.Now()
	if e.ReloadTemplates && now.After(e.nextCheck) {
		if err = e.UpdateTemplate(); err != nil {
			return
		}
		e.nextCheck = now.Add(time.Second * time.Duration(e.ReloadInterval))
	}

	m := pack.Message
	subs := MsgSubs{
		Uuid:       m.GetUuidString(),
		Timestamp:  time.Unix(0, m.GetTimestamp()).Format(e.TimestampFormat),
		Type:       m.GetType(),
		Logger:     m.GetLogger(),
		Severity:   strconv.FormatInt(int64(m.GetSeverity()), 10),
		Payload:    m.GetPayload(),
		EnvVersion: m.GetEnvVersion(),
		Pid:        strconv.FormatInt(int64(m.GetPid()), 10),
		Hostname:   m.GetHostname(),
	}
	if len(m.Fields) > 0 {
		subs.Fields = make(map[string]string)
		for _, field := range m.Fields {
			switch field.GetValueType() {
			case message.Field_STRING:
				value := field.GetValueString()
				if len(value) > 0 {
					subs.Fields[field.GetName()] = value[0]
				}
			case message.Field_BYTES:
				value := field.GetValueBytes()
				if len(value) > 0 {
					subs.Fields[field.GetName()] = fmt.Sprintf("%s", value[0])
				}
			case message.Field_INTEGER:
				value := field.GetValueInteger()
				if len(value) > 0 {
					subs.Fields[field.GetName()] = strconv.FormatInt(value[0], 10)
				}
			case message.Field_DOUBLE:
				value := field.GetValueDouble()
				if len(value) > 0 {
					subs.Fields[field.GetName()] = strconv.FormatFloat(value[0], 'g', -1, 64)
				}
			case message.Field_BOOL:
				value := field.GetValueBool()
				if len(value) > 0 {
					subs.Fields[field.GetName()] = strconv.FormatBool(value[0])
				}
			}
		}
	}
	buf := bytes.Buffer{}
	e.template.Execute(&buf, subs)
	return buf.Bytes(), err
}

func (e *TextTemplateEncoder) UpdateTemplate() error {
	var err error
	changed := false
	for i, filename := range e.TemplateFiles {
		fi, err := os.Stat(filename)
		if err != nil {
			return err
		}
		if !fi.ModTime().Equal(e.lastFileTime[i]) {
			changed = true
			e.lastFileTime[i] = fi.ModTime()
		}
	}
	if changed {
		log.Println("Reloaded templates")
		e.template, err = template.ParseFiles(e.TemplateFiles...)
		return err
	}
	return nil
}

func init() {
	pipeline.RegisterPlugin("TextTemplateEncoder", func() interface{} {
		return new(TextTemplateEncoder)
	})
}
