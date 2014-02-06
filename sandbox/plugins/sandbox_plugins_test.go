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
#   Mike Trinkala (trink@mozilla.com)
#
# ***** END LICENSE BLOCK *****/

package plugins

import (
	"code.google.com/p/go-uuid/uuid"
	"code.google.com/p/gomock/gomock"
	"fmt"
	"github.com/mozilla-services/heka/message"
	"github.com/mozilla-services/heka/pipeline"
	ts "github.com/mozilla-services/heka/pipeline/testsupport"
	pm "github.com/mozilla-services/heka/pipelinemock"
	"github.com/mozilla-services/heka/sandbox"
	gs "github.com/rafrombrc/gospec/src/gospec"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestAllSpecs(t *testing.T) {
	r := gs.NewRunner()
	r.Parallel = false

	r.AddSpec(FilterSpec)
	r.AddSpec(DecoderSpec)

	gs.MainGoTest(r, t)
}

func getTestMessage() *message.Message {
	hostname := "my.host.name"
	field, _ := message.NewField("foo", "bar", "")
	msg := &message.Message{}
	msg.SetType("TEST")
	msg.SetTimestamp(time.Now().UnixNano())
	msg.SetUuid(uuid.NewRandom())
	msg.SetLogger("GoSpec")
	msg.SetSeverity(int32(6))
	msg.SetPayload("Test Payload")
	msg.SetEnvVersion("0.8")
	msg.SetPid(int32(os.Getpid()))
	msg.SetHostname(hostname)
	msg.AddField(field)
	return msg
}

type FilterTestHelper struct {
	MockHelper       *pm.MockPluginHelper
	MockFilterRunner *pm.MockFilterRunner
}

func NewFilterTestHelper(ctrl *gomock.Controller) (fth *FilterTestHelper) {
	fth = new(FilterTestHelper)
	fth.MockHelper = pm.NewMockPluginHelper(ctrl)
	fth.MockFilterRunner = pm.NewMockFilterRunner(ctrl)
	return
}

func FilterSpec(c gs.Context) {
	t := new(ts.SimpleT)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	fth := NewFilterTestHelper(ctrl)
	inChan := make(chan *pipeline.PipelinePack, 1)
	pConfig := pipeline.NewPipelineConfig(nil)

	c.Specify("A SandboxFilter", func() {
		sbFilter := new(SandboxFilter)
		config := sbFilter.ConfigStruct().(*sandbox.SandboxConfig)
		config.ScriptType = "lua"
		config.MemoryLimit = 32000
		config.InstructionLimit = 1000
		config.OutputLimit = 1024

		msg := getTestMessage()
		pack := pipeline.NewPipelinePack(pConfig.InjectRecycleChan())
		pack.Message = msg
		pack.Decoded = true

		c.Specify("Over inject messages from ProcessMessage", func() {
			var timer <-chan time.Time
			fth.MockFilterRunner.EXPECT().Ticker().Return(timer)
			fth.MockFilterRunner.EXPECT().InChan().Return(inChan)
			fth.MockFilterRunner.EXPECT().Name().Return("processinject").Times(3)
			fth.MockFilterRunner.EXPECT().Inject(pack).Return(true).Times(2)
			fth.MockHelper.EXPECT().PipelineConfig().Return(pConfig)
			fth.MockHelper.EXPECT().PipelinePack(uint(0)).Return(pack).Times(2)
			fth.MockFilterRunner.EXPECT().LogError(fmt.Errorf("exceeded InjectMessage count"))

			config.ScriptFilename = "../lua/testsupport/processinject.lua"
			err := sbFilter.Init(config)
			c.Assume(err, gs.IsNil)
			inChan <- pack
			close(inChan)
			sbFilter.Run(fth.MockFilterRunner, fth.MockHelper)
		})

		c.Specify("Over inject messages from TimerEvent", func() {
			var timer <-chan time.Time
			timer = time.Tick(time.Duration(1) * time.Millisecond)
			fth.MockFilterRunner.EXPECT().Ticker().Return(timer)
			fth.MockFilterRunner.EXPECT().InChan().Return(inChan)
			fth.MockFilterRunner.EXPECT().Name().Return("timerinject").Times(12)
			fth.MockFilterRunner.EXPECT().Inject(pack).Return(true).Times(11)
			fth.MockHelper.EXPECT().PipelineConfig().Return(pConfig)
			fth.MockHelper.EXPECT().PipelinePack(uint(0)).Return(pack).Times(11)
			fth.MockFilterRunner.EXPECT().LogError(fmt.Errorf("exceeded InjectMessage count"))

			config.ScriptFilename = "../lua/testsupport/timerinject.lua"
			err := sbFilter.Init(config)
			c.Assume(err, gs.IsNil)
			go func() {
				time.Sleep(time.Duration(250) * time.Millisecond)
				close(inChan)
			}()
			sbFilter.Run(fth.MockFilterRunner, fth.MockHelper)
		})
	})

	c.Specify("A SandboxManagerFilter", func() {
		sbmFilter := new(SandboxManagerFilter)
		config := sbmFilter.ConfigStruct().(*SandboxManagerFilterConfig)
		config.MaxFilters = 1

		origBaseDir := pipeline.Globals().BaseDir
		pipeline.Globals().BaseDir = os.TempDir()
		sbxMgrsDir := filepath.Join(pipeline.Globals().BaseDir, "sbxmgrs")
		defer func() {
			pipeline.Globals().BaseDir = origBaseDir
			tmpErr := os.RemoveAll(sbxMgrsDir)
			c.Expect(tmpErr, gs.IsNil)
		}()

		msg := getTestMessage()
		pack := pipeline.NewPipelinePack(pConfig.InputRecycleChan())
		pack.Message = msg
		pack.Decoded = true

		c.Specify("Control message in the past", func() {
			sbmFilter.Init(config)
			pack.Message.SetTimestamp(time.Now().UnixNano() - 5e9)
			fth.MockFilterRunner.EXPECT().InChan().Return(inChan)
			fth.MockFilterRunner.EXPECT().Name().Return("SandboxManagerFilter")
			fth.MockFilterRunner.EXPECT().LogError(fmt.Errorf("Discarded control message: 5 seconds skew"))
			inChan <- pack
			close(inChan)
			sbmFilter.Run(fth.MockFilterRunner, fth.MockHelper)
		})

		c.Specify("Control message in the future", func() {
			sbmFilter.Init(config)
			pack.Message.SetTimestamp(time.Now().UnixNano() + 5.9e9)
			fth.MockFilterRunner.EXPECT().InChan().Return(inChan)
			fth.MockFilterRunner.EXPECT().Name().Return("SandboxManagerFilter")
			fth.MockFilterRunner.EXPECT().LogError(fmt.Errorf("Discarded control message: -5 seconds skew"))
			inChan <- pack
			close(inChan)
			sbmFilter.Run(fth.MockFilterRunner, fth.MockHelper)
		})

		c.Specify("Generates the right default working directory", func() {
			sbmFilter.Init(config)
			fth.MockFilterRunner.EXPECT().InChan().Return(inChan)
			name := "SandboxManagerFilter"
			fth.MockFilterRunner.EXPECT().Name().Return(name)
			close(inChan)
			sbmFilter.Run(fth.MockFilterRunner, fth.MockHelper)
			c.Expect(sbmFilter.workingDirectory, gs.Equals, sbxMgrsDir)
			_, err := os.Stat(sbxMgrsDir)
			c.Expect(err, gs.IsNil)
		})

	})
}

func DecoderSpec(c gs.Context) {
	t := new(ts.SimpleT)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// NewPipelineConfig sets up Globals which is needed for the
	// pipeline.GetHekaConfigDir() to not die during plugin Init()
	_ = pipeline.NewPipelineConfig(nil)

	c.Specify("A SandboxDecoder", func() {

		decoder := new(SandboxDecoder)
		conf := decoder.ConfigStruct().(*sandbox.SandboxConfig)
		conf.ScriptFilename = "../lua/testsupport/decoder.lua"
		conf.ScriptType = "lua"
		supply := make(chan *pipeline.PipelinePack, 1)
		pack := pipeline.NewPipelinePack(supply)

		c.Specify("decodes simple messages", func() {
			data := "1376389920 debug id=2321 url=example.com item=1"
			err := decoder.Init(conf)
			c.Assume(err, gs.IsNil)
			dRunner := pm.NewMockDecoderRunner(ctrl)
			decoder.SetDecoderRunner(dRunner)
			pack.Message.SetPayload(data)
			_, err = decoder.Decode(pack)
			c.Assume(err, gs.IsNil)

			c.Expect(pack.Message.GetTimestamp(),
				gs.Equals,
				int64(1376389920000000000))

			c.Expect(pack.Message.GetSeverity(), gs.Equals, int32(7))

			var ok bool
			var value interface{}
			value, ok = pack.Message.GetFieldValue("id")
			c.Expect(ok, gs.Equals, true)
			c.Expect(value, gs.Equals, "2321")

			value, ok = pack.Message.GetFieldValue("url")
			c.Expect(ok, gs.Equals, true)
			c.Expect(value, gs.Equals, "example.com")

			value, ok = pack.Message.GetFieldValue("item")
			c.Expect(ok, gs.Equals, true)
			c.Expect(value, gs.Equals, "1")
			decoder.Shutdown()
		})

		c.Specify("decodes an invalid messages", func() {
			data := "1376389920 bogus id=2321 url=example.com item=1"
			err := decoder.Init(conf)
			c.Assume(err, gs.IsNil)
			dRunner := pm.NewMockDecoderRunner(ctrl)
			decoder.SetDecoderRunner(dRunner)
			pack.Message.SetPayload(data)
			packs, err := decoder.Decode(pack)
			c.Expect(len(packs), gs.Equals, 0)
			c.Expect(err.Error(), gs.Equals, "Failed parsing: "+data)
			c.Expect(decoder.processMessageFailures, gs.Equals, int64(1))
			decoder.Shutdown()
		})
	})

	c.Specify("A Multipack SandboxDecoder", func() {
		decoder := new(SandboxDecoder)
		conf := decoder.ConfigStruct().(*sandbox.SandboxConfig)
		conf.ScriptFilename = "../lua/testsupport/multipack_decoder.lua"
		conf.ScriptType = "lua"
		supply := make(chan *pipeline.PipelinePack, 3)
		pack := pipeline.NewPipelinePack(supply)
		pack.Message = getTestMessage()

		pack1 := pipeline.NewPipelinePack(supply)
		pack2 := pipeline.NewPipelinePack(supply)

		c.Specify("decodes into multiple packs", func() {
			err := decoder.Init(conf)
			c.Assume(err, gs.IsNil)
			dRunner := pm.NewMockDecoderRunner(ctrl)
			decoder.SetDecoderRunner(dRunner)
			gomock.InOrder(
				dRunner.EXPECT().NewPack().Return(pack1),
				dRunner.EXPECT().NewPack().Return(pack2),
			)
			packs, err := decoder.Decode(pack)
			c.Expect(len(packs), gs.Equals, 3)
			c.Expect(packs[0].Message.GetPayload(), gs.Equals, "message one")
			c.Expect(packs[1].Message.GetPayload(), gs.Equals, "message two")
			c.Expect(packs[2].Message.GetPayload(), gs.Equals, "message three")

			for i := 0; i < 1; i++ {
				c.Expect(packs[i].Message.GetType(), gs.Equals, "TEST")
				c.Expect(packs[i].Message.GetHostname(), gs.Equals, "my.host.name")
				c.Expect(packs[i].Message.GetLogger(), gs.Equals, "GoSpec")
				c.Expect(packs[i].Message.GetSeverity(), gs.Equals, int32(6))

			}
			decoder.Shutdown()
		})
	})

	c.Specify("A syslog SandboxDecoder", func() {
		decoder := new(SandboxDecoder)
		conf := decoder.ConfigStruct().(*sandbox.SandboxConfig)
		conf.ModuleDirectory = "/Users/victorng/dev/heka/build/heka/modules"
		conf.ScriptFilename = "../lua/decoders/rfc5424_syslog.lua"
		conf.ScriptType = "lua"

		supply := make(chan *pipeline.PipelinePack, 1)
		pack := pipeline.NewPipelinePack(supply)

		rfc5424_with_structured_data := "<165>1 2003-10-11T22:14:15.003Z mymachine.example.com evntslog - ID47 [exampleSDID@32473 iut=\"3\" eventSource= \"Application\" eventID=\"1011\"] An application event log entry..."

		rfc5424_no_structured_data := "<34>1 2003-10-11T22:14:15.003Z mymachine.example.com su - ID47 - BOM'su root' failed for lonvick on /dev/pts/8"
		rfc5424_structured_data_only := "<165>1 2003-10-11T22:14:15.003Z mymachine.example.com evntslog - ID47 [exampleSDID@32473 iut=\"3\" eventSource= \"Application\" eventID=\"1011\"][examplePriority@32473 class=\"high\"]"

		c.Specify("decodes without structured_data only", func() {
			// As per section 6.5 of RFC5424:
			// This example shows a message with only STRUCTURED-DATA
			// and no MSG part.  This is a valid message.

			err := decoder.Init(conf)
			c.Assume(err, gs.IsNil)
			dRunner := pm.NewMockDecoderRunner(ctrl)
			decoder.SetDecoderRunner(dRunner)
			pack.Message.SetPayload(rfc5424_structured_data_only)
			_, err = decoder.Decode(pack)
			c.Assume(err, gs.IsNil)
			value, ok := pack.Message.GetFieldValue("pri")
			c.Expect(ok, gs.IsTrue)
			c.Expect(value, gs.Equals, "165")

			value, ok = pack.Message.GetFieldValue("hostname")
			c.Expect(ok, gs.IsTrue)
			c.Expect(value, gs.Equals, "mymachine.example.com")

			value, ok = pack.Message.GetFieldValue("app_name")
			c.Expect(ok, gs.IsTrue)
			c.Expect(value, gs.Equals, "evntslog")

			value, ok = pack.Message.GetFieldValue("procid")
			c.Expect(ok, gs.IsTrue)
			c.Expect(value, gs.Equals, "-")

			value, ok = pack.Message.GetFieldValue("msgid")
			c.Expect(ok, gs.IsTrue)
			c.Expect(value, gs.Equals, "ID47")

			value, ok = pack.Message.GetFieldValue("structured_data")
			c.Expect(ok, gs.IsTrue)
			c.Expect(value, gs.Equals, "[exampleSDID@32473 iut=\"3\" eventSource= \"Application\" eventID=\"1011\"][examplePriority@32473 class=\"high\"]")

		})

		c.Specify("decodes without structured_data", func() {
			err := decoder.Init(conf)
			c.Assume(err, gs.IsNil)
			dRunner := pm.NewMockDecoderRunner(ctrl)
			decoder.SetDecoderRunner(dRunner)
			pack.Message.SetPayload(rfc5424_no_structured_data)
			_, err = decoder.Decode(pack)
			c.Assume(err, gs.IsNil)
			value, ok := pack.Message.GetFieldValue("pri")
			c.Expect(ok, gs.IsTrue)
			c.Expect(value, gs.Equals, "34")
			value, ok = pack.Message.GetFieldValue("hostname")
			c.Expect(ok, gs.IsTrue)
			c.Expect(value, gs.Equals, "mymachine.example.com")

			value, ok = pack.Message.GetFieldValue("app_name")
			c.Expect(ok, gs.IsTrue)
			c.Expect(value, gs.Equals, "su")

			value, ok = pack.Message.GetFieldValue("procid")
			c.Expect(ok, gs.IsTrue)
			c.Expect(value, gs.Equals, "-")

			value, ok = pack.Message.GetFieldValue("msgid")
			c.Expect(ok, gs.IsTrue)
			c.Expect(value, gs.Equals, "ID47")

			value, ok = pack.Message.GetFieldValue("msg")
			c.Expect(ok, gs.IsTrue)
			c.Expect(value, gs.Equals, "BOM'su root' failed for lonvick on /dev/pts/8")

		})

		c.Specify("uses alternate msg_type", func() {
			conf.Config = make(map[string]interface{})
			conf.Config["msg_type"] = "alternate"
			err := decoder.Init(conf)
			c.Assume(err, gs.IsNil)
			dRunner := pm.NewMockDecoderRunner(ctrl)
			decoder.SetDecoderRunner(dRunner)
			pack.Message.SetPayload(rfc5424_with_structured_data)
			_, err = decoder.Decode(pack)
			c.Assume(err, gs.IsNil)
			c.Expect(pack.Message.GetType(), gs.Equals, "alternate")
		})

		c.Specify("decodes structured_data", func() {
			err := decoder.Init(conf)
			c.Assume(err, gs.IsNil)
			dRunner := pm.NewMockDecoderRunner(ctrl)
			decoder.SetDecoderRunner(dRunner)
			pack.Message.SetPayload(rfc5424_with_structured_data)
			_, err = decoder.Decode(pack)
			c.Assume(err, gs.IsNil)
			value, ok := pack.Message.GetFieldValue("pri")
			c.Expect(ok, gs.IsTrue)
			c.Expect(value, gs.Equals, "165")
			value, ok = pack.Message.GetFieldValue("hostname")
			c.Expect(ok, gs.IsTrue)
			c.Expect(value, gs.Equals, "mymachine.example.com")

			value, ok = pack.Message.GetFieldValue("app_name")
			c.Expect(ok, gs.IsTrue)
			c.Expect(value, gs.Equals, "evntslog")

			value, ok = pack.Message.GetFieldValue("procid")
			c.Expect(ok, gs.IsTrue)
			c.Expect(value, gs.Equals, "-")

			value, ok = pack.Message.GetFieldValue("msgid")
			c.Expect(ok, gs.IsTrue)
			c.Expect(value, gs.Equals, "ID47")

			value, ok = pack.Message.GetFieldValue("structured_data")
			c.Expect(ok, gs.IsTrue)
			c.Expect(value, gs.Equals, "[exampleSDID@32473 iut=\"3\" eventSource= \"Application\" eventID=\"1011\"]")

			value, ok = pack.Message.GetFieldValue("msg")
			c.Expect(ok, gs.IsTrue)
			c.Expect(value, gs.Equals, "An application event log entry...")
		})

	})
}
