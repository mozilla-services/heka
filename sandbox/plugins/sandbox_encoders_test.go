package plugins

import (
	"code.google.com/p/go-uuid/uuid"
	"code.google.com/p/gogoprotobuf/proto"
	"encoding/json"
	"github.com/mozilla-services/heka/message"
	"github.com/mozilla-services/heka/pipeline"
	ts "github.com/mozilla-services/heka/pipeline/testsupport"
	"github.com/rafrombrc/gomock/gomock"
	gs "github.com/rafrombrc/gospec/src/gospec"
)

func EncoderSpec(c gs.Context) {
	t := new(ts.SimpleT)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// NewPipelineConfig sets up Globals which is needed for the
	// pipeline.Prepend*Dir functions to not die during plugin Init().
	pConfig := pipeline.NewPipelineConfig(nil)

	c.Specify("A SandboxEncoder", func() {

		encoder := new(SandboxEncoder)
		encoder.SetPipelineConfig(pConfig)
		conf := encoder.ConfigStruct().(*SandboxEncoderConfig)
		supply := make(chan *pipeline.PipelinePack, 1)
		pack := pipeline.NewPipelinePack(supply)
		pack.Message.SetPayload("original")
		pack.Message.SetType("my_type")
		pack.Message.SetPid(12345)
		pack.Message.SetSeverity(4)
		pack.Message.SetHostname("hostname")
		pack.Message.SetTimestamp(54321)
		pack.Message.SetUuid(uuid.NewRandom())
		var (
			result []byte
			err    error
		)

		c.Specify("emits JSON correctly", func() {
			conf.ScriptFilename = "../lua/testsupport/encoder_json.lua"
			err = encoder.Init(conf)
			c.Expect(err, gs.IsNil)

			result, err = encoder.Encode(pack)
			c.Expect(err, gs.IsNil)
			msg := new(message.Message)
			err = json.Unmarshal(result, msg)
			c.Expect(err, gs.IsNil)
			c.Expect(msg.GetTimestamp(), gs.Equals, int64(54321))
			c.Expect(msg.GetPid(), gs.Equals, int32(12345))
			c.Expect(msg.GetSeverity(), gs.Equals, int32(4))
			c.Expect(msg.GetHostname(), gs.Equals, "hostname")
			c.Expect(msg.GetPayload(), gs.Equals, "original")
			c.Expect(msg.GetType(), gs.Equals, "my_type")
		})

		c.Specify("emits text correctly", func() {
			conf.ScriptFilename = "../lua/testsupport/encoder_text.lua"
			err = encoder.Init(conf)
			c.Expect(err, gs.IsNil)

			result, err = encoder.Encode(pack)
			c.Expect(err, gs.IsNil)
			c.Expect(string(result), gs.Equals, "Prefixed original")
		})

		c.Specify("emits protobuf correctly", func() {

			c.Specify("when inject_message is used", func() {
				conf.ScriptFilename = "../lua/testsupport/encoder_protobuf.lua"
				err = encoder.Init(conf)
				c.Expect(err, gs.IsNil)

				result, err = encoder.Encode(pack)
				c.Expect(err, gs.IsNil)

				msg := new(message.Message)
				err = proto.Unmarshal(result, msg)
				c.Expect(err, gs.IsNil)
				c.Expect(msg.GetTimestamp(), gs.Equals, int64(54321))
				c.Expect(msg.GetPid(), gs.Equals, int32(12345))
				c.Expect(msg.GetSeverity(), gs.Equals, int32(4))
				c.Expect(msg.GetHostname(), gs.Equals, "hostname")
				c.Expect(msg.GetPayload(), gs.Equals, "mutated")
				c.Expect(msg.GetType(), gs.Equals, "after")
			})

			c.Specify("when `write_message` is used", func() {
				conf.ScriptFilename = "../lua/testsupport/encoder_writemessage.lua"
				err = encoder.Init(conf)
				c.Expect(err, gs.IsNil)

				result, err = encoder.Encode(pack)
				c.Expect(err, gs.IsNil)

				msg := new(message.Message)
				err = proto.Unmarshal(result, msg)
				c.Expect(err, gs.IsNil)
				c.Expect(msg.GetPayload(), gs.Equals, "mutated payload")
				c.Expect(pack.Message.GetPayload(), gs.Equals, "original")
			})
		})

	})
}
