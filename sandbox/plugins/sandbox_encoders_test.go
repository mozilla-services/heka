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

	c.Specify("cbuf librato encoder", func() {
		encoder := new(SandboxEncoder)
		encoder.SetPipelineConfig(pConfig)
		conf := encoder.ConfigStruct().(*SandboxEncoderConfig)
		supply := make(chan *pipeline.PipelinePack, 1)
		pack := pipeline.NewPipelinePack(supply)
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
		conf.ScriptFilename = "../lua/encoders/cbuf_librato.lua"
		conf.ModuleDirectory = "../../../../../../modules"
		conf.Config = make(map[string]interface{})
		err = encoder.Init(conf)
		c.Assume(err, gs.IsNil)

		c.Specify("encodes cbuf data", func() {
			payload := `{"time":1410823460,"rows":5,"columns":5,"seconds_per_row":5,"column_info":[{"name":"HTTP_200","unit":"count","aggregation":"sum"},{"name":"HTTP_300","unit":"count","aggregation":"sum"},{"name":"HTTP_400","unit":"count","aggregation":"sum"},{"name":"HTTP_500","unit":"count","aggregation":"sum"},{"name":"HTTP_UNKNOWN","unit":"count","aggregation":"sum"}]}
1	2	3	4	5
6	7	8	9	10
11	12	13	14	15
16	17	18	19	20
21	22	23	24	25
`
			pack.Message.SetPayload(payload)
			result, err = encoder.Encode(pack)
			c.Expect(err, gs.IsNil)
			expected := `{"gauges":[{"value":1,"measure_time":1410823460,"name":"HTTP_200","source":"hostname"},{"value":2,"measure_time":1410823460,"name":"HTTP_300","source":"hostname"},{"value":3,"measure_time":1410823460,"name":"HTTP_400","source":"hostname"},{"value":4,"measure_time":1410823460,"name":"HTTP_500","source":"hostname"},{"value":5,"measure_time":1410823460,"name":"HTTP_UNKNOWN","source":"hostname"},{"value":6,"measure_time":1410823465,"name":"HTTP_200","source":"hostname"},{"value":7,"measure_time":1410823465,"name":"HTTP_300","source":"hostname"},{"value":8,"measure_time":1410823465,"name":"HTTP_400","source":"hostname"},{"value":9,"measure_time":1410823465,"name":"HTTP_500","source":"hostname"},{"value":10,"measure_time":1410823465,"name":"HTTP_UNKNOWN","source":"hostname"},{"value":11,"measure_time":1410823470,"name":"HTTP_200","source":"hostname"},{"value":12,"measure_time":1410823470,"name":"HTTP_300","source":"hostname"},{"value":13,"measure_time":1410823470,"name":"HTTP_400","source":"hostname"},{"value":14,"measure_time":1410823470,"name":"HTTP_500","source":"hostname"},{"value":15,"measure_time":1410823470,"name":"HTTP_UNKNOWN","source":"hostname"},{"value":16,"measure_time":1410823475,"name":"HTTP_200","source":"hostname"},{"value":17,"measure_time":1410823475,"name":"HTTP_300","source":"hostname"},{"value":18,"measure_time":1410823475,"name":"HTTP_400","source":"hostname"},{"value":19,"measure_time":1410823475,"name":"HTTP_500","source":"hostname"},{"value":20,"measure_time":1410823475,"name":"HTTP_UNKNOWN","source":"hostname"}]}`
			c.Expect(string(result), gs.Equals, expected)

			c.Specify("and correctly advances", func() {
				payload := `{"time":1410823475,"rows":5,"columns":5,"seconds_per_row":5,"column_info":[{"name":"HTTP_200","unit":"count","aggregation":"sum"},{"name":"HTTP_300","unit":"count","aggregation":"sum"},{"name":"HTTP_400","unit":"count","aggregation":"sum"},{"name":"HTTP_500","unit":"count","aggregation":"sum"},{"name":"HTTP_UNKNOWN","unit":"count","aggregation":"sum"}]}
16	17	18	19	20
21	22	23	24	25
1	2	3	4	5
6	nan	8	nan	10
5	4	3	2	1
`
				pack.Message.SetPayload(payload)
				result, err = encoder.Encode(pack)
				c.Expect(err, gs.IsNil)
				expected := `{"gauges":[{"value":21,"measure_time":1410823480,"name":"HTTP_200","source":"hostname"},{"value":22,"measure_time":1410823480,"name":"HTTP_300","source":"hostname"},{"value":23,"measure_time":1410823480,"name":"HTTP_400","source":"hostname"},{"value":24,"measure_time":1410823480,"name":"HTTP_500","source":"hostname"},{"value":25,"measure_time":1410823480,"name":"HTTP_UNKNOWN","source":"hostname"},{"value":1,"measure_time":1410823485,"name":"HTTP_200","source":"hostname"},{"value":2,"measure_time":1410823485,"name":"HTTP_300","source":"hostname"},{"value":3,"measure_time":1410823485,"name":"HTTP_400","source":"hostname"},{"value":4,"measure_time":1410823485,"name":"HTTP_500","source":"hostname"},{"value":5,"measure_time":1410823485,"name":"HTTP_UNKNOWN","source":"hostname"},{"value":6,"measure_time":1410823490,"name":"HTTP_200","source":"hostname"},{"value":8,"measure_time":1410823490,"name":"HTTP_400","source":"hostname"},{"value":10,"measure_time":1410823490,"name":"HTTP_UNKNOWN","source":"hostname"}]}`
				c.Expect(string(result), gs.Equals, expected)
			})
		})
	})
}
