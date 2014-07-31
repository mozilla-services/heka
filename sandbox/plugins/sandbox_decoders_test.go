package plugins

import (
	"github.com/mozilla-services/heka/message"
	"github.com/mozilla-services/heka/pipeline"
	ts "github.com/mozilla-services/heka/pipeline/testsupport"
	pm "github.com/mozilla-services/heka/pipelinemock"
	"github.com/mozilla-services/heka/sandbox"
	"github.com/rafrombrc/gomock/gomock"
	gs "github.com/rafrombrc/gospec/src/gospec"
	"os"
)

func DecoderSpec(c gs.Context) {
	t := new(ts.SimpleT)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	pConfig := pipeline.NewPipelineConfig(nil)

	c.Specify("A SandboxDecoder", func() {

		decoder := new(SandboxDecoder)
		decoder.SetPipelineConfig(pConfig)
		conf := decoder.ConfigStruct().(*sandbox.SandboxConfig)
		supply := make(chan *pipeline.PipelinePack, 1)
		pack := pipeline.NewPipelinePack(supply)
		dRunner := pm.NewMockDecoderRunner(ctrl)

		c.Specify("that uses lpeg and inject_message", func() {
			dRunner.EXPECT().Name().Return("serialize")
			conf.ScriptFilename = "../lua/testsupport/decoder.lua"
			err := decoder.Init(conf)
			c.Assume(err, gs.IsNil)

			c.Specify("decodes simple messages", func() {
				data := "1376389920 debug id=2321 url=example.com item=1"
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
				decoder.SetDecoderRunner(dRunner)
				pack.Message.SetPayload(data)
				packs, err := decoder.Decode(pack)
				c.Expect(len(packs), gs.Equals, 0)
				c.Expect(err.Error(), gs.Equals, "Failed parsing: "+data)
				c.Expect(decoder.processMessageFailures, gs.Equals, int64(1))
				decoder.Shutdown()
			})

			c.Specify("Preserves data", func() {
				conf.ScriptFilename = "../lua/testsupport/serialize.lua"
				conf.PreserveData = true
				err := decoder.Init(conf)
				c.Assume(err, gs.IsNil)
				decoder.SetDecoderRunner(dRunner)
				decoder.Shutdown()
				_, err = os.Stat("sandbox_preservation/serialize.data")
				c.Expect(err, gs.IsNil)
				err = os.Remove("sandbox_preservation/serialize.data")
				c.Expect(err, gs.IsNil)
			})
		})

		c.Specify("that only uses write_message", func() {
			conf.ScriptFilename = "../lua/testsupport/write_message_decoder.lua"
			dRunner.EXPECT().Name().Return("write_message")
			err := decoder.Init(conf)
			decoder.SetDecoderRunner(dRunner)
			c.Assume(err, gs.IsNil)

			c.Specify("adds a string field to the message", func() {
				data := "string field scribble"
				pack.Message.SetPayload(data)
				packs, err := decoder.Decode(pack)
				c.Expect(err, gs.IsNil)
				c.Expect(len(packs), gs.Equals, 1)
				c.Expect(packs[0], gs.Equals, pack)
				value, ok := pack.Message.GetFieldValue("scribble")
				c.Expect(ok, gs.IsTrue)
				c.Expect(value.(string), gs.Equals, "foo")
			})

			c.Specify("adds a numeric field to the message", func() {
				data := "num field scribble"
				pack.Message.SetPayload(data)
				packs, err := decoder.Decode(pack)
				c.Expect(err, gs.IsNil)
				c.Expect(len(packs), gs.Equals, 1)
				c.Expect(packs[0], gs.Equals, pack)
				value, ok := pack.Message.GetFieldValue("scribble")
				c.Expect(ok, gs.IsTrue)
				c.Expect(value.(float64), gs.Equals, float64(1))
			})

			c.Specify("adds a boolean field to the message", func() {
				data := "bool field scribble"
				pack.Message.SetPayload(data)
				packs, err := decoder.Decode(pack)
				c.Expect(err, gs.IsNil)
				c.Expect(len(packs), gs.Equals, 1)
				c.Expect(packs[0], gs.Equals, pack)
				value, ok := pack.Message.GetFieldValue("scribble")
				c.Expect(ok, gs.IsTrue)
				c.Expect(value.(bool), gs.Equals, true)
			})

			c.Specify("sets type and payload", func() {
				data := "set type and payload"
				pack.Message.SetPayload(data)
				packs, err := decoder.Decode(pack)
				c.Expect(err, gs.IsNil)
				c.Expect(len(packs), gs.Equals, 1)
				c.Expect(packs[0], gs.Equals, pack)
				c.Expect(pack.Message.GetType(), gs.Equals, "my_type")
				c.Expect(pack.Message.GetPayload(), gs.Equals, "my_payload")
			})

			c.Specify("sets field value with representation", func() {
				data := "set field value with representation"
				pack.Message.SetPayload(data)
				packs, err := decoder.Decode(pack)
				c.Expect(err, gs.IsNil)
				c.Expect(len(packs), gs.Equals, 1)
				c.Expect(packs[0], gs.Equals, pack)
				fields := pack.Message.FindAllFields("rep")
				c.Expect(len(fields), gs.Equals, 1)
				field := fields[0]
				values := field.GetValueString()
				c.Expect(len(values), gs.Equals, 1)
				c.Expect(values[0], gs.Equals, "foo")
				c.Expect(field.GetRepresentation(), gs.Equals, "representation")
			})

			c.Specify("sets multiple field string values", func() {
				data := "set multiple field string values"
				pack.Message.SetPayload(data)
				packs, err := decoder.Decode(pack)
				c.Expect(err, gs.IsNil)
				c.Expect(len(packs), gs.Equals, 1)
				c.Expect(packs[0], gs.Equals, pack)
				fields := pack.Message.FindAllFields("multi")
				c.Expect(len(fields), gs.Equals, 2)
				values := fields[0].GetValueString()
				c.Expect(len(values), gs.Equals, 1)
				c.Expect(values[0], gs.Equals, "first")
				values = fields[1].GetValueString()
				c.Expect(len(values), gs.Equals, 1)
				c.Expect(values[0], gs.Equals, "second")
			})

			c.Specify("sets field string array value", func() {
				data := "set field string array value"
				pack.Message.SetPayload(data)
				packs, err := decoder.Decode(pack)
				c.Expect(err, gs.IsNil)
				c.Expect(len(packs), gs.Equals, 1)
				c.Expect(packs[0], gs.Equals, pack)
				fields := pack.Message.FindAllFields("array")
				c.Expect(len(fields), gs.Equals, 1)
				values := fields[0].GetValueString()
				c.Expect(len(values), gs.Equals, 2)
				c.Expect(values[0], gs.Equals, "first")
				c.Expect(values[1], gs.Equals, "second")
			})
		})
	})

	c.Specify("A Multipack SandboxDecoder", func() {
		decoder := new(SandboxDecoder)
		decoder.SetPipelineConfig(pConfig)
		conf := decoder.ConfigStruct().(*sandbox.SandboxConfig)
		conf.ScriptFilename = "../lua/testsupport/multipack_decoder.lua"
		supply := make(chan *pipeline.PipelinePack, 3)
		pack := pipeline.NewPipelinePack(supply)
		pack.Message = getTestMessage()

		pack1 := pipeline.NewPipelinePack(supply)
		pack2 := pipeline.NewPipelinePack(supply)
		dRunner := pm.NewMockDecoderRunner(ctrl)
		dRunner.EXPECT().Name().Return("SandboxDecoder")

		c.Specify("decodes into multiple packs", func() {
			err := decoder.Init(conf)
			c.Assume(err, gs.IsNil)
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

	c.Specify("Cpu Stats decoder", func() {
		decoder := new(SandboxDecoder)
		decoder.SetPipelineConfig(pConfig)
		conf := decoder.ConfigStruct().(*sandbox.SandboxConfig)
		conf.ScriptFilename = "../lua/decoders/cpustats.lua"
		conf.ModuleDirectory = "../../../../../../modules"
		conf.MemoryLimit = 8e6
		conf.Config = make(map[string]interface{})
		supply := make(chan *pipeline.PipelinePack, 1)
		pack := pipeline.NewPipelinePack(supply)
		dRunner := pm.NewMockDecoderRunner(ctrl)
		dRunner.EXPECT().Name().Return("SandboxDecoder")
		err := decoder.Init(conf)
		c.Assume(err, gs.IsNil)
		decoder.SetDecoderRunner(dRunner)

		c.Specify("decodes a message", func() {
			payload := "0.00 0.01 0.05 3/153 660\n"
			pack.Message.SetPayload(payload)
			f, err := message.NewField("FilePath", "/proc/loadavg", "")
			c.Assume(err, gs.IsNil)
			pack.Message.AddField(f)

			_, err = decoder.Decode(pack)
			c.Assume(err, gs.IsNil)
			c.Expect(pack.Message.GetSeverity(), gs.Equals, int32(7))

			var ok bool
			var value interface{}
			value, ok = pack.Message.GetFieldValue("1MinAvg")
			c.Expect(ok, gs.IsTrue)
			c.Expect(value, gs.Equals, 0.00)

			value, ok = pack.Message.GetFieldValue("5MinAvg")
			c.Expect(ok, gs.IsTrue)
			c.Expect(value, gs.Equals, 0.01)

			value, ok = pack.Message.GetFieldValue("15MinAvg")
			c.Expect(ok, gs.IsTrue)
			c.Expect(value, gs.Equals, 0.05)

			value, ok = pack.Message.GetFieldValue("NumProcesses")
			c.Expect(ok, gs.IsTrue)
			c.Expect(value, gs.Equals, float64(3))

			value, ok = pack.Message.GetFieldValue("FilePath")
			c.Expect(ok, gs.IsTrue)
			c.Expect(value, gs.Equals, "/proc/loadavg")
		})

		c.Specify("decodes an invalid message", func() {
			data := "bogus message"
			pack.Message.SetPayload(data)
			packs, err := decoder.Decode(pack)
			c.Expect(len(packs), gs.Equals, 0)
			c.Expect(err.Error(), gs.Equals, "Failed parsing: "+data)
			c.Expect(decoder.processMessageFailures, gs.Equals, int64(1))
			decoder.Shutdown()
		})
	})

	c.Specify("Nginx access log decoder", func() {
		decoder := new(SandboxDecoder)
		decoder.SetPipelineConfig(pConfig)
		conf := decoder.ConfigStruct().(*sandbox.SandboxConfig)
		conf.ScriptFilename = "../lua/decoders/nginx_access.lua"
		conf.ModuleDirectory = "../../../../../../modules"
		conf.MemoryLimit = 8e6
		conf.Config = make(map[string]interface{})
		conf.Config["log_format"] = "$remote_addr - $remote_user [$time_local] \"$request\" $status $body_bytes_sent \"$http_referer\" \"$http_user_agent\""
		conf.Config["user_agent_transform"] = true
		supply := make(chan *pipeline.PipelinePack, 1)
		pack := pipeline.NewPipelinePack(supply)
		dRunner := pm.NewMockDecoderRunner(ctrl)
		dRunner.EXPECT().Name().Return("SandboxDecoder")
		err := decoder.Init(conf)
		c.Assume(err, gs.IsNil)
		decoder.SetDecoderRunner(dRunner)

		c.Specify("decodes simple messages", func() {
			data := "127.0.0.1 - - [10/Feb/2014:08:46:41 -0800] \"GET / HTTP/1.1\" 304 0 \"-\" \"Mozilla/5.0 (X11; Ubuntu; Linux x86_64; rv:26.0) Gecko/20100101 Firefox/26.0\""
			pack.Message.SetPayload(data)
			_, err = decoder.Decode(pack)
			c.Assume(err, gs.IsNil)

			c.Expect(pack.Message.GetTimestamp(),
				gs.Equals,
				int64(1392050801000000000))

			c.Expect(pack.Message.GetSeverity(), gs.Equals, int32(7))

			var ok bool
			var value interface{}
			value, ok = pack.Message.GetFieldValue("remote_addr")
			c.Expect(ok, gs.Equals, true)
			c.Expect(value, gs.Equals, "127.0.0.1")

			value, ok = pack.Message.GetFieldValue("user_agent_browser")
			c.Expect(ok, gs.Equals, true)
			c.Expect(value, gs.Equals, "Firefox")
			value, ok = pack.Message.GetFieldValue("user_agent_version")
			c.Expect(ok, gs.Equals, true)
			c.Expect(value, gs.Equals, float64(26))
			value, ok = pack.Message.GetFieldValue("user_agent_os")
			c.Expect(ok, gs.Equals, true)
			c.Expect(value, gs.Equals, "Linux")
			_, ok = pack.Message.GetFieldValue("http_user_agent")
			c.Expect(ok, gs.Equals, false)

			value, ok = pack.Message.GetFieldValue("body_bytes_sent")
			c.Expect(ok, gs.Equals, true)
			c.Expect(value, gs.Equals, float64(0))

			value, ok = pack.Message.GetFieldValue("status")
			c.Expect(ok, gs.Equals, true)
			c.Expect(value, gs.Equals, float64(304))
			decoder.Shutdown()
		})

		c.Specify("decodes an invalid messages", func() {
			data := "bogus message"
			pack.Message.SetPayload(data)
			packs, err := decoder.Decode(pack)
			c.Expect(len(packs), gs.Equals, 0)
			c.Expect(err.Error(), gs.Equals, "Failed parsing: "+data)
			c.Expect(decoder.processMessageFailures, gs.Equals, int64(1))
			decoder.Shutdown()
		})
	})

	c.Specify("Apache access log decoder", func() {
		decoder := new(SandboxDecoder)
		decoder.SetPipelineConfig(pConfig)
		conf := decoder.ConfigStruct().(*sandbox.SandboxConfig)
		conf.ScriptFilename = "../lua/decoders/apache_access.lua"
		conf.ModuleDirectory = "../../../../../../modules"
		conf.MemoryLimit = 8e6
		conf.Config = make(map[string]interface{})
		conf.Config["log_format"] = "%h %l %u %t \"%r\" %>s %b \"%{Referer}i\" \"%{User-Agent}i\""
		conf.Config["user_agent_transform"] = true
		supply := make(chan *pipeline.PipelinePack, 1)
		pack := pipeline.NewPipelinePack(supply)
		dRunner := pm.NewMockDecoderRunner(ctrl)
		dRunner.EXPECT().Name().Return("SandboxDecoder")
		err := decoder.Init(conf)
		c.Assume(err, gs.IsNil)
		decoder.SetDecoderRunner(dRunner)

		c.Specify("decodes simple messages", func() {
			data := "127.0.0.1 - - [10/Feb/2014:08:46:41 -0800] \"GET / HTTP/1.1\" 304 0 \"-\" \"Mozilla/5.0 (X11; Ubuntu; Linux x86_64; rv:26.0) Gecko/20100101 Firefox/26.0\""
			pack.Message.SetPayload(data)
			_, err = decoder.Decode(pack)
			c.Assume(err, gs.IsNil)

			c.Expect(pack.Message.GetTimestamp(),
				gs.Equals,
				int64(1392050801000000000))

			c.Expect(pack.Message.GetSeverity(), gs.Equals, int32(7))

			var ok bool
			var value interface{}
			value, ok = pack.Message.GetFieldValue("remote_addr")
			c.Expect(ok, gs.Equals, true)
			c.Expect(value, gs.Equals, "127.0.0.1")

			value, ok = pack.Message.GetFieldValue("user_agent_browser")
			c.Expect(ok, gs.Equals, true)
			c.Expect(value, gs.Equals, "Firefox")
			value, ok = pack.Message.GetFieldValue("user_agent_version")
			c.Expect(ok, gs.Equals, true)
			c.Expect(value, gs.Equals, float64(26))
			value, ok = pack.Message.GetFieldValue("user_agent_os")
			c.Expect(ok, gs.Equals, true)
			c.Expect(value, gs.Equals, "Linux")
			_, ok = pack.Message.GetFieldValue("http_user_agent")
			c.Expect(ok, gs.Equals, false)

			value, ok = pack.Message.GetFieldValue("body_bytes_sent")
			c.Expect(ok, gs.Equals, true)
			c.Expect(value, gs.Equals, float64(0))

			value, ok = pack.Message.GetFieldValue("status")
			c.Expect(ok, gs.Equals, true)
			c.Expect(value, gs.Equals, float64(304))
			decoder.Shutdown()
		})

		c.Specify("decodes an invalid messages", func() {
			data := "bogus message"
			pack.Message.SetPayload(data)
			packs, err := decoder.Decode(pack)
			c.Expect(len(packs), gs.Equals, 0)
			c.Expect(err.Error(), gs.Equals, "Failed parsing: "+data)
			c.Expect(decoder.processMessageFailures, gs.Equals, int64(1))
			decoder.Shutdown()
		})
	})

	c.Specify("rsyslog decoder", func() {
		decoder := new(SandboxDecoder)
		decoder.SetPipelineConfig(pConfig)
		conf := decoder.ConfigStruct().(*sandbox.SandboxConfig)
		conf.ScriptFilename = "../lua/decoders/rsyslog.lua"
		conf.ModuleDirectory = "../../../../../../modules"
		conf.MemoryLimit = 8e6
		conf.Config = make(map[string]interface{})
		conf.Config["type"] = "MyTestFormat"
		conf.Config["template"] = "%pri% %TIMESTAMP% %TIMEGENERATED:::date-rfc3339% %HOSTNAME% %syslogtag%%msg:::sp-if-no-1st-sp%%msg:::drop-last-lf%\n"
		conf.Config["tz"] = "America/Los_Angeles"
		supply := make(chan *pipeline.PipelinePack, 1)
		pack := pipeline.NewPipelinePack(supply)
		dRunner := pm.NewMockDecoderRunner(ctrl)
		dRunner.EXPECT().Name().Return("SandboxDecoder")
		err := decoder.Init(conf)
		c.Assume(err, gs.IsNil)
		decoder.SetDecoderRunner(dRunner)

		c.Specify("decodes simple messages", func() {
			data := "28 Feb 10 12:58:58 2014-02-10T12:58:59-08:00 testhost widget[4322]: test message.\n"
			pack.Message.SetPayload(data)
			_, err = decoder.Decode(pack)
			c.Assume(err, gs.IsNil)

			c.Expect(pack.Message.GetTimestamp(),
				gs.Equals,
				int64(1392065938000000000))

			c.Expect(pack.Message.GetSeverity(), gs.Equals, int32(4))
			c.Expect(pack.Message.GetHostname(), gs.Equals, "testhost")
			c.Expect(pack.Message.GetPid(), gs.Equals, int32(4322))
			c.Expect(pack.Message.GetPayload(), gs.Equals, "test message.")
			c.Expect(pack.Message.GetType(), gs.Equals, conf.Config["type"])

			var ok bool
			var value interface{}
			value, ok = pack.Message.GetFieldValue("programname")
			c.Expect(ok, gs.Equals, true)
			c.Expect(value, gs.Equals, "widget")

			value, ok = pack.Message.GetFieldValue("syslogfacility")
			c.Expect(ok, gs.Equals, true)
			c.Expect(value, gs.Equals, float64(3))

			value, ok = pack.Message.GetFieldValue("timegenerated")
			c.Expect(ok, gs.Equals, true)
			c.Expect(value, gs.Equals, float64(1392065939000000000))

			decoder.Shutdown()
		})

		c.Specify("decodes an invalid messages", func() {
			data := "bogus message"
			pack.Message.SetPayload(data)
			packs, err := decoder.Decode(pack)
			c.Expect(len(packs), gs.Equals, 0)
			c.Expect(err.Error(), gs.Equals, "Failed parsing: "+data)
			c.Expect(decoder.processMessageFailures, gs.Equals, int64(1))
			decoder.Shutdown()
		})
	})

	c.Specify("mysql decoder", func() {
		decoder := new(SandboxDecoder)
		decoder.SetPipelineConfig(pConfig)
		conf := decoder.ConfigStruct().(*sandbox.SandboxConfig)
		conf.ScriptFilename = "../lua/decoders/mysql_slow_query.lua"
		conf.ModuleDirectory = "../../../../../../modules"
		conf.MemoryLimit = 8e6
		conf.Config = make(map[string]interface{})
		conf.Config["truncate_sql"] = int64(5)
		supply := make(chan *pipeline.PipelinePack, 1)
		pack := pipeline.NewPipelinePack(supply)
		dRunner := pm.NewMockDecoderRunner(ctrl)
		dRunner.EXPECT().Name().Return("SandboxDecoder")
		err := decoder.Init(conf)
		c.Assume(err, gs.IsNil)
		decoder.SetDecoderRunner(dRunner)

		c.Specify("decode standard slow query log", func() {
			data := `# User@Host: syncrw[syncrw] @  [127.0.0.1]
# Query_time: 2.964652  Lock_time: 0.000050 Rows_sent: 251  Rows_examined: 9773
use widget;
SET last_insert_id=999,insert_id=1000,timestamp=1399500744;
# administrator command: do something
/* [queryName=FIND_ITEMS] */ SELECT *
FROM widget
WHERE id = 10;
`
			pack.Message.SetPayload(data)
			_, err = decoder.Decode(pack)
			c.Assume(err, gs.IsNil)

			c.Expect(pack.Message.GetTimestamp(),
				gs.Equals,
				int64(1399500744000000000))
			c.Expect(pack.Message.GetPayload(), gs.Equals, "/* [q...")
			c.Expect(pack.Message.GetType(), gs.Equals, "mysql.slow-query")

			decoder.Shutdown()
		})
	})

	c.Specify("mariadb decoder", func() {
		decoder := new(SandboxDecoder)
		decoder.SetPipelineConfig(pConfig)
		conf := decoder.ConfigStruct().(*sandbox.SandboxConfig)
		conf.ScriptFilename = "../lua/decoders/mariadb_slow_query.lua"
		conf.ModuleDirectory = "../../../../../../modules"
		conf.MemoryLimit = 8e6
		conf.Config = make(map[string]interface{})
		conf.Config["truncate_sql"] = int64(5)
		supply := make(chan *pipeline.PipelinePack, 1)
		pack := pipeline.NewPipelinePack(supply)
		dRunner := pm.NewMockDecoderRunner(ctrl)
		dRunner.EXPECT().Name().Return("SandboxDecoder")
		err := decoder.Init(conf)
		c.Assume(err, gs.IsNil)
		decoder.SetDecoderRunner(dRunner)

		c.Specify("decode standard slow query log", func() {
			data := `# User@Host: syncrw[syncrw] @  [127.0.0.1]
# Thread_id: 110804  Schema: weave0  QC_hit: No
# Query_time: 1.178108  Lock_time: 0.000053  Rows_sent: 198  Rows_examined: 198
SET timestamp=1399500744;
/* [queryName=FIND_ITEMS] */ SELECT *
FROM widget
WHERE id = 10;
`
			pack.Message.SetPayload(data)
			_, err = decoder.Decode(pack)
			c.Assume(err, gs.IsNil)

			c.Expect(pack.Message.GetTimestamp(),
				gs.Equals,
				int64(1399500744000000000))
			c.Expect(pack.Message.GetPayload(), gs.Equals, "/* [q...")
			c.Expect(pack.Message.GetType(), gs.Equals, "mariadb.slow-query")

			decoder.Shutdown()
		})
	})
}
