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
	"code.google.com/p/gomock/gomock"
	"code.google.com/p/goprotobuf/proto"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/mozilla-services/heka/message"
	ts "github.com/mozilla-services/heka/testsupport"
	gs "github.com/rafrombrc/gospec/src/gospec"
	"github.com/rafrombrc/gospec/src/gospec"
	"io/ioutil"
	"strings"
	"sync"
	"testing"
	"time"
)

type PanicDecoder struct{}

func (p *PanicDecoder) Init(config interface{}) (err error) {
	return
}

func (p *PanicDecoder) Decode(pack *PipelinePack) (err error) {
	panic("PANICDECODER")
	return
}

// Attach an `Init` method to MockDecoders so they'll work w/ PluginWrappers
func (d *MockDecoder) Init(config interface{}) (err error) {
	return
}

func DecodersSpec(c gospec.Context) {
	t := &ts.SimpleT{}
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	msg := getTestMessage()
	config := NewPipelineConfig(nil)

	c.Specify("A JsonDecoder", func() {
		encoded, err := json.Marshal(msg)
		c.Assume(err, gs.IsNil)
		pack := NewPipelinePack(config.inputRecycleChan)
		decoder := new(JsonDecoder)

		c.Specify("decodes a json message", func() {
			pack.MsgBytes = encoded
			err := decoder.Decode(pack)
			c.Expect(err, gs.IsNil)
			c.Expect(pack.Message, gs.Equals, msg)
			v, ok := pack.Message.GetFieldValue("foo")
			c.Expect(ok, gs.IsTrue)
			c.Expect(v, gs.Equals, "bar")
		})

		c.Specify("returns an error for bunk encoding", func() {
			bunk := append([]byte{'}'}, encoded...)
			pack.MsgBytes = bunk
			err := decoder.Decode(pack)
			c.Expect(err, gs.Not(gs.IsNil))
		})
	})

	c.Specify("A ProtobufDecoder", func() {
		encoded, err := proto.Marshal(msg)
		c.Assume(err, gs.IsNil)
		pack := NewPipelinePack(config.inputRecycleChan)
		decoder := new(ProtobufDecoder)

		c.Specify("decodes a protobuf message", func() {
			pack.MsgBytes = encoded
			err := decoder.Decode(pack)
			c.Expect(err, gs.IsNil)
			c.Expect(pack.Message, gs.Equals, msg)
			v, ok := pack.Message.GetFieldValue("foo")
			c.Expect(ok, gs.IsTrue)
			c.Expect(v, gs.Equals, "bar")
		})

		c.Specify("returns an error for bunk encoding", func() {
			bunk := append([]byte{0, 0, 0}, encoded...)
			pack.MsgBytes = bunk
			err := decoder.Decode(pack)
			c.Expect(err, gs.Not(gs.IsNil))
		})
	})

	c.Specify("Recovers from a panic in `Decode()`", func() {
		decoder := new(PanicDecoder)
		dRunner := NewDecoderRunner("panic", decoder, nil)
		pack := NewPipelinePack(config.inputRecycleChan)
		var wg sync.WaitGroup
		wg.Add(1)
		Globals().Stopping = true
		dRunner.Start(config, &wg)
		dRunner.InChan() <- pack // No panic ==> success
		wg.Wait()
		Globals().Stopping = false
	})

	c.Specify("A LoglineDecoder", func() {
		decoder := new(LoglineDecoder)
		conf := decoder.ConfigStruct().(*LoglineDecoderConfig)
		supply := make(chan *PipelinePack, 1)
		pack := NewPipelinePack(supply)
		conf.TimestampLayout = "02/Jan/2006:15:04:05 -0700"

		c.Specify("non capture regex", func() {
			conf.MatchRegex = `\d+`
			err := decoder.Init(conf)
			c.Expect(err, gs.Not(gs.IsNil))
			c.Expect(err.Error(), gs.Equals, "LoglineDecoder regex must contain capture groups")
		})

		c.Specify("invalid regex", func() {
			conf.MatchRegex = `\mtest`
			err := decoder.Init(conf)
			c.Expect(err, gs.Not(gs.IsNil))
			c.Expect(err.Error(), gs.Equals, "LoglineDecoder: error parsing regexp: invalid escape sequence: `\\m`")
		})

		c.Specify("reading an apache timestamp", func() {
			conf.MatchRegex = `\[(?P<Timestamp>[^\]]+)\]`
			err := decoder.Init(conf)
			c.Assume(err, gs.IsNil)
			dRunner := NewMockDecoderRunner(ctrl)
			decoder.SetDecoderRunner(dRunner)
			pack.Message.SetPayload("[18/Apr/2013:14:00:28 -0700]")
			err = decoder.Decode(pack)
			c.Expect(pack.Message.GetTimestamp(), gs.Equals, int64(1366318828000000000))
			pack.Zero()
		})

		c.Specify("uses kitchen timestamp", func() {
			conf.MatchRegex = `\[(?P<Timestamp>[^\]]+)\]`
			err := decoder.Init(conf)
			c.Assume(err, gs.IsNil)
			dRunner := NewMockDecoderRunner(ctrl)
			decoder.SetDecoderRunner(dRunner)
			pack.Message.SetPayload("[5:16PM]")
			now := time.Now()
			cur_date := time.Date(now.Year(), now.Month(), now.Day(), 17, 16, 0, 0,
				time.UTC)
			err = decoder.Decode(pack)
			c.Expect(pack.Message.GetTimestamp(), gs.Equals, cur_date.UnixNano())
			pack.Zero()
		})

		c.Specify("adjusts timestamps as specified", func() {
			conf.MatchRegex = `\[(?P<Timestamp>[^\]]+)\]`
			conf.TimestampLayout = "02/Jan/2006:15:04:05"
			conf.TimestampLocation = "America/Los_Angeles"
			timeStr := "18/Apr/2013:14:00:28"
			loc, err := time.LoadLocation(conf.TimestampLocation)
			c.Assume(err, gs.IsNil)
			expectedLocal, err := time.ParseInLocation(conf.TimestampLayout, timeStr, loc)
			c.Assume(err, gs.IsNil)
			err = decoder.Init(conf)
			c.Assume(err, gs.IsNil)
			dRunner := NewMockDecoderRunner(ctrl)
			decoder.SetDecoderRunner(dRunner)
			pack.Message.SetPayload("[" + timeStr + "]")
			err = decoder.Decode(pack)
			c.Expect(pack.Message.GetTimestamp(), gs.Equals, expectedLocal.UnixNano())
			pack.Zero()
		})

		c.Specify("apply representation metadata to a captured field", func() {
			value := "0.23"
			payload := "header"
			conf.MatchRegex = `(?P<ResponseTime>\d+\.\d+)`
			conf.MessageFields = MessageTemplate{
				"ResponseTime|s": "%ResponseTime%",
				"Payload|s":      "%ResponseTime%",
				"Payload":        payload,
			}
			err := decoder.Init(conf)
			c.Assume(err, gs.IsNil)
			dRunner := NewMockDecoderRunner(ctrl)
			decoder.SetDecoderRunner(dRunner)
			pack.Message.SetPayload(value)
			err = decoder.Decode(pack)

			f := pack.Message.FindFirstField("ResponseTime")
			c.Expect(f, gs.Not(gs.IsNil))
			c.Expect(f.GetValue(), gs.Equals, value)
			c.Expect(f.GetRepresentation(), gs.Equals, "s")

			f = pack.Message.FindFirstField("Payload")
			c.Expect(f, gs.Not(gs.IsNil))
			c.Expect(f.GetValue(), gs.Equals, value)
			c.Expect(f.GetRepresentation(), gs.Equals, "s")

			c.Expect(pack.Message.GetPayload(), gs.Equals, payload)

			pack.Zero()
		})

		c.Specify("reading test-zeus.log", func() {
			conf.MatchRegex = `(?P<Ip>([0-9]{1,3}\.){3}[0-9]{1,3}) (?P<Hostname>(([a-zA-Z0-9]|[a-zA-Z0-9][a-zA-Z0-9\-]*[a-zA-Z0-9])\.)*([A-Za-z0-9]|[A-Za-z0-9][A-Za-z0-9\-]*[A-Za-z0-9])) (?P<User>\w+) \[(?P<Timestamp>[^\]]+)\] \"(?P<Verb>[A-X]+) (?P<Request>\/\S*) HTTP\/(?P<Httpversion>\d\.\d)\" (?P<Response>\d{3}) (?P<Bytes>\d+)`
			conf.MessageFields = MessageTemplate{
				"hostname": "%Hostname%",
				"ip":       "%Ip%",
				"response": "%Response%",
			}
			err := decoder.Init(conf)
			c.Assume(err, gs.IsNil)
			filePath := "../testsupport/test-zeus.log"
			fileBytes, err := ioutil.ReadFile(filePath)
			c.Assume(err, gs.IsNil)
			fileStr := string(fileBytes)
			lines := strings.Split(fileStr, "\n")

			containsFieldValue := func(str, fieldName string, msg *message.Message) bool {
				raw, ok := msg.GetFieldValue(fieldName)
				if !ok {
					return false
				}
				value := raw.(string)
				return strings.Contains(str, value)
			}

			c.Specify("extracts capture data and puts it in the message fields", func() {
				var misses int
				for _, line := range lines {
					if strings.TrimSpace(line) == "" {
						continue
					}
					pack.Message.SetPayload(line)
					err = decoder.Decode(pack)
					if err != nil {
						misses++
						continue
					}
					c.Expect(containsFieldValue(line, "hostname", pack.Message), gs.IsTrue)
					c.Expect(containsFieldValue(line, "ip", pack.Message), gs.IsTrue)
					c.Expect(containsFieldValue(line, "response", pack.Message), gs.IsTrue)
					pack.Zero()
				}
				c.Expect(misses, gs.Equals, 3)
			})
		})

		c.Specify("reading test-severity.log", func() {
			conf.MatchRegex = `severity: (?P<Severity>[a-zA-Z]+)`
			conf.SeverityMap = map[string]int32{
				"emergency": 0,
				"alert":     1,
				"critical":  2,
				"error":     3,
				"warning":   4,
				"notice":    5,
				"info":      6,
				"debug":     7,
			}
			reverseMap := make(map[int32]string)
			for str, i := range conf.SeverityMap {
				reverseMap[i] = str
			}
			err := decoder.Init(conf)
			c.Assume(err, gs.IsNil)
			dRunner := NewMockDecoderRunner(ctrl)
			decoder.SetDecoderRunner(dRunner)

			filePath := "../testsupport/test-severity.log"
			fileBytes, err := ioutil.ReadFile(filePath)
			c.Assume(err, gs.IsNil)
			fileStr := string(fileBytes)
			lines := strings.Split(fileStr, "\n")

			c.Specify("sets message severity based on SeverityMap", func() {
				err := errors.New("Don't recognize severity: 'BOGUS'")
				dRunner.EXPECT().LogError(err)
				for _, line := range lines {
					if strings.TrimSpace(line) == "" {
						continue
					}
					pack.Message.SetPayload(line)
					err = decoder.Decode(pack)
					if err != nil {
						fmt.Println(line)
					}
					c.Expect(err, gs.IsNil)
					if strings.Contains(line, "BOGUS") {
						continue
					}
					strVal := reverseMap[pack.Message.GetSeverity()]
					c.Expect(strings.Contains(line, strVal), gs.IsTrue)
				}
			})
		})
	})
}

func BenchmarkDecodeJSON(b *testing.B) {
	b.StopTimer()
	msg := getTestMessage()
	var fmtString = `{"uuid":"%s","type":"%s","timestamp":%s,"logger":"%s","severity":%d,"payload":"%s","fields":%s,"env_version":"%s","metlog_pid":%d,"metlog_hostname":"%s"}`
	timestampJson, _ := json.Marshal(time.Unix(*msg.Timestamp/1e9, *msg.Timestamp%1e9))
	fieldsJson := `{"foo":"bar"}`
	uuid := msg.GetUuidString()
	jsonString := fmt.Sprintf(fmtString, uuid, *msg.Type,
		timestampJson, *msg.Logger, *msg.Severity, *msg.Payload,
		fieldsJson, *msg.EnvVersion, *msg.Pid, *msg.Hostname)

	config := NewPipelineConfig(nil)
	pipelinePack := NewPipelinePack(config.inputRecycleChan)
	pipelinePack.MsgBytes = []byte(jsonString)
	jsonDecoder := new(JsonDecoder)
	b.StartTimer()
	for i := 0; i < b.N; i++ {
		jsonDecoder.Decode(pipelinePack)
	}
}
func BenchmarkEncodeProtobuf(b *testing.B) {
	b.StopTimer()
	msg := getTestMessage()
	b.StartTimer()
	for i := 0; i < b.N; i++ {
		proto.Marshal(msg)
	}
}

func BenchmarkDecodeProtobuf(b *testing.B) {
	b.StopTimer()
	msg := getTestMessage()
	encoded, _ := proto.Marshal(msg)
	config := NewPipelineConfig(nil)
	pack := NewPipelinePack(config.inputRecycleChan)
	decoder := new(ProtobufDecoder)
	pack.MsgBytes = encoded
	b.StartTimer()
	for i := 0; i < b.N; i++ {
		decoder.Decode(pack)
	}
}
