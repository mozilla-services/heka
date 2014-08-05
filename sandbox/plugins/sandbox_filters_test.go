package plugins

import (
	"fmt"
	"github.com/mozilla-services/heka/message"
	"github.com/mozilla-services/heka/pipeline"
	ts "github.com/mozilla-services/heka/pipeline/testsupport"
	pm "github.com/mozilla-services/heka/pipelinemock"
	"github.com/mozilla-services/heka/sandbox"
	"github.com/rafrombrc/gomock/gomock"
	gs "github.com/rafrombrc/gospec/src/gospec"
	"os"
	"path/filepath"
	"time"
)

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
		sbFilter.SetPipelineConfig(pConfig)
		config := sbFilter.ConfigStruct().(*sandbox.SandboxConfig)
		config.MemoryLimit = 32000
		config.InstructionLimit = 1000
		config.OutputLimit = 1024

		msg := getTestMessage()
		pack := pipeline.NewPipelinePack(pConfig.InjectRecycleChan())
		pack.Message = msg
		pack.Decoded = true

		c.Specify("Uninitialized", func() {
			err := sbFilter.ReportMsg(msg)
			c.Expect(err, gs.IsNil)
		})

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

		c.Specify("Preserves data", func() {
			var timer <-chan time.Time
			fth.MockFilterRunner.EXPECT().Ticker().Return(timer)
			fth.MockFilterRunner.EXPECT().InChan().Return(inChan)

			config.ScriptFilename = "../lua/testsupport/serialize.lua"
			config.PreserveData = true
			sbFilter.SetName("serialize")
			err := sbFilter.Init(config)
			c.Assume(err, gs.IsNil)
			close(inChan)
			sbFilter.Run(fth.MockFilterRunner, fth.MockHelper)
			_, err = os.Stat("sandbox_preservation/serialize.data")
			c.Expect(err, gs.IsNil)
			err = os.Remove("sandbox_preservation/serialize.data")
			c.Expect(err, gs.IsNil)
		})
	})

	c.Specify("A SandboxManagerFilter", func() {
		pConfig.Globals.BaseDir = os.TempDir()
		sbxMgrsDir := filepath.Join(pConfig.Globals.BaseDir, "sbxmgrs")
		defer func() {
			tmpErr := os.RemoveAll(sbxMgrsDir)
			c.Expect(tmpErr, gs.IsNil)
		}()

		sbmFilter := new(SandboxManagerFilter)
		sbmFilter.SetPipelineConfig(pConfig)
		config := sbmFilter.ConfigStruct().(*SandboxManagerFilterConfig)
		config.MaxFilters = 1

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

		c.Specify("Sanity check the default sandbox configuration limits", func() {
			sbmFilter.Init(config)
			c.Expect(sbmFilter.memoryLimit, gs.Equals, uint(8*1024*1024))
			c.Expect(sbmFilter.instructionLimit, gs.Equals, uint(1e6))
			c.Expect(sbmFilter.outputLimit, gs.Equals, uint(63*1024))
		})

		c.Specify("Sanity check the user specified sandbox configuration limits", func() {
			config.MemoryLimit = 123456
			config.InstructionLimit = 4321
			config.OutputLimit = 8765
			sbmFilter.Init(config)
			c.Expect(sbmFilter.memoryLimit, gs.Equals, config.MemoryLimit)
			c.Expect(sbmFilter.instructionLimit, gs.Equals, config.InstructionLimit)
			c.Expect(sbmFilter.outputLimit, gs.Equals, config.OutputLimit)
		})

		c.Specify("Creates a SandboxFilter runner", func() {
			sbxName := "SandboxFilter"
			sbxMgrName := "SandboxManagerFilter"
			code := `
			require("cjson")

			function process_message()
			    inject_payload(cjson.encode({a = "b"}))
			    return 0
			end
			`
			cfg := `
			[%s]
			type = "SandboxFilter"
			message_matcher = "TRUE"
			script_type = "lua"
			`
			cfg = fmt.Sprintf(cfg, sbxName)
			msg.SetPayload(code)
			f, err := message.NewField("config", cfg, "toml")
			c.Assume(err, gs.IsNil)
			msg.AddField(f)

			fMatchChan := pConfig.Router().AddFilterMatcher()
			errChan := make(chan error)

			fth.MockFilterRunner.EXPECT().Name().Return(sbxMgrName)
			fullSbxName := fmt.Sprintf("%s-%s", sbxMgrName, sbxName)
			fth.MockHelper.EXPECT().Filter(fullSbxName).Return(nil, false)
			fth.MockFilterRunner.EXPECT().LogMessage(fmt.Sprintf("Loading: %s", fullSbxName))

			sbmFilter.Init(config)
			go func() {
				err := sbmFilter.loadSandbox(fth.MockFilterRunner, fth.MockHelper, sbxMgrsDir,
					msg)
				errChan <- err
			}()

			fMatch := <-fMatchChan
			c.Expect(fMatch.MatcherSpecification().String(), gs.Equals, "TRUE")
			c.Expect(<-errChan, gs.IsNil)

			go func() {
				<-pConfig.Router().RemoveFilterMatcher()
			}()
			ok := pConfig.RemoveFilterRunner(fullSbxName)
			c.Expect(ok, gs.IsTrue)
		})
	})

	c.Specify("A Cpu Stats filter", func() {
		filter := new(SandboxFilter)
		filter.SetPipelineConfig(pConfig)
		filter.name = "cpustats"
		conf := filter.ConfigStruct().(*sandbox.SandboxConfig)
		conf.ScriptFilename = "../lua/filters/cpustats.lua"
		conf.ModuleDirectory = "../lua/modules"

		conf.Config = make(map[string]interface{})
		conf.Config["rows"] = int64(3)
		conf.Config["sec_per_row"] = int64(1)

		timer := make(chan time.Time, 1)
		errChan := make(chan error, 1)
		retPackChan := make(chan *pipeline.PipelinePack, 1)
		recycleChan := make(chan *pipeline.PipelinePack, 1)

		defer func() {
			close(errChan)
			close(retPackChan)
		}()

		msg := getTestMessage()
		fields := make([]*message.Field, 4)
		fields[0], _ = message.NewField("1MinAvg", 0.08, "")
		fields[1], _ = message.NewField("5MinAvg", 0.04, "")
		fields[2], _ = message.NewField("15MinAvg", 0.02, "")
		fields[3], _ = message.NewField("NumProcesses", 5, "")
		msg.Fields = fields

		pack := pipeline.NewPipelinePack(recycleChan)

		fth.MockHelper.EXPECT().PipelinePack(uint(0)).Return(pack)
		fth.MockFilterRunner.EXPECT().Ticker().Return(timer)
		fth.MockFilterRunner.EXPECT().InChan().Return(inChan)
		fth.MockFilterRunner.EXPECT().Name().Return("cpustats")
		fth.MockFilterRunner.EXPECT().Inject(pack).Do(func(pack *pipeline.PipelinePack) {
			retPackChan <- pack
		}).Return(true)

		err := filter.Init(conf)
		c.Assume(err, gs.IsNil)

		c.Specify("should fill a cbuf with cpuload data", func() {
			go func() {
				errChan <- filter.Run(fth.MockFilterRunner, fth.MockHelper)
			}()

			for i := 1; i <= 3; i++ {
				// Fill in the data
				t := int64(i * 1000000000)
				pack.Message = msg
				pack.Message.SetTimestamp(t)

				// Feed in a pack
				inChan <- pack
				pack = <-recycleChan
			}

			timer <- time.Now()
			p := <-retPackChan
			// Check the result of the filter's inject
			pl := `{"time":1,"rows":3,"columns":4,"seconds_per_row":1,"column_info":[{"name":"1MinAvg","unit":"Count","aggregation":"max"},{"name":"5MinAvg","unit":"Count","aggregation":"max"},{"name":"15MinAvg","unit":"Count","aggregation":"max"},{"name":"NumProcesses","unit":"Count","aggregation":"max"}]}
0.08	0.04	0.02	5
0.08	0.04	0.02	5
0.08	0.04	0.02	5
`

			c.Expect(p.Message.GetPayload(), gs.Equals, pl)
		})

		close(inChan)
		c.Expect(<-errChan, gs.IsNil)
	})

	c.Specify("A Memstats filter", func() {
		filter := new(SandboxFilter)
		filter.SetPipelineConfig(pConfig)
		filter.name = "memstats"
		conf := filter.ConfigStruct().(*sandbox.SandboxConfig)
		conf.ScriptFilename = "../lua/filters/memstats.lua"
		conf.ModuleDirectory = "../lua/modules"

		conf.Config = make(map[string]interface{})
		conf.Config["rows"] = int64(3)
		conf.Config["sec_per_row"] = int64(1)

		timer := make(chan time.Time, 1)
		errChan := make(chan error, 1)
		retPackChan := make(chan *pipeline.PipelinePack, 1)
		recycleChan := make(chan *pipeline.PipelinePack, 1)

		defer func() {
			close(errChan)
			close(retPackChan)
		}()

		msg := getTestMessage()
		field_names := []string{"MemFree", "Cached", "Active", "Inactive", "VmallocUsed", "Shmem", "SwapCached", "SwapTotal", "SwapFree"}
		fields := make([]*message.Field, len(field_names))
		for i, name := range field_names {
			fields[i], _ = message.NewField(name, 100, "")
		}
		msg.Fields = fields

		pack := pipeline.NewPipelinePack(recycleChan)

		fth.MockHelper.EXPECT().PipelinePack(uint(0)).Return(pack)
		fth.MockFilterRunner.EXPECT().Ticker().Return(timer)
		fth.MockFilterRunner.EXPECT().InChan().Return(inChan)
		fth.MockFilterRunner.EXPECT().Name().Return("memstats")
		fth.MockFilterRunner.EXPECT().Inject(pack).Do(func(pack *pipeline.PipelinePack) {
			retPackChan <- pack
		}).Return(true)

		err := filter.Init(conf)
		c.Assume(err, gs.IsNil)

		c.Specify("should fill a cbuf with memstats data", func() {
			go func() {
				errChan <- filter.Run(fth.MockFilterRunner, fth.MockHelper)
			}()

			for i := 1; i <= 3; i++ {
				// Fill in the data
				t := int64(i * 1000000000)
				pack.Message = msg
				pack.Message.SetTimestamp(t)

				// Feed in a pack
				inChan <- pack
				pack = <-recycleChan
			}

			timer <- time.Now()
			p := <-retPackChan
			// Check the result of the filter's inject
			pl := `{"time":1,"rows":3,"columns":9,"seconds_per_row":1,"column_info":[{"name":"MemFree","unit":"Count","aggregation":"max"},{"name":"Cached","unit":"Count","aggregation":"max"},{"name":"Active","unit":"Count","aggregation":"max"},{"name":"Inactive","unit":"Count","aggregation":"max"},{"name":"VmallocUsed","unit":"Count","aggregation":"max"},{"name":"Shmem","unit":"Count","aggregation":"max"},{"name":"SwapCached","unit":"Count","aggregation":"max"},{"name":"SwapFree","unit":"Count","aggregation":"max"},{"name":"SwapUsed","unit":"Count","aggregation":"max"}]}
100	100	100	100	100	100	100	100	0
100	100	100	100	100	100	100	100	0
100	100	100	100	100	100	100	100	0
`
			c.Expect(p.Message.GetPayload(), gs.Equals, pl)

		})

		close(inChan)
		c.Expect(<-errChan, gs.IsNil)
	})

	c.Specify("A diskstats filter", func() {
		filter := new(SandboxFilter)
		filter.SetPipelineConfig(pConfig)
		filter.name = "diskstats"
		conf := filter.ConfigStruct().(*sandbox.SandboxConfig)
		conf.ScriptFilename = "../lua/filters/diskstats.lua"
		conf.ModuleDirectory = "../lua/modules"

		conf.Config = make(map[string]interface{})
		conf.Config["rows"] = int64(3)

		timer := make(chan time.Time, 1)
		errChan := make(chan error, 1)
		retMsgChan := make(chan *message.Message, 3)
		recycleChan := make(chan *pipeline.PipelinePack, 1)

		defer func() {
			close(errChan)
			close(retMsgChan)
		}()

		msg := getTestMessage()

		field_names := []string{
			"WritesCompleted",
			"ReadsCompleted",
			"SectorsWritten",
			"SectorsRead",
			"WritesMerged",
			"ReadsMerged",
			"TimeWriting",
			"TimeReading",
			"TimeDoingIO",
			"WeightedTimeDoingIO",
		}

		num_fields := len(field_names) + 1
		fields := make([]*message.Field, num_fields)
		msg.Fields = fields
		timeInterval, _ := message.NewField("TickerInterval", 1, "")
		fields[num_fields-1] = timeInterval
		fieldVal := 100

		pack := pipeline.NewPipelinePack(recycleChan)

		fth.MockHelper.EXPECT().PipelineConfig().AnyTimes()
		fth.MockHelper.EXPECT().PipelinePack(uint(0)).Return(pack).AnyTimes()
		fth.MockFilterRunner.EXPECT().Ticker().Return(timer).AnyTimes()
		fth.MockFilterRunner.EXPECT().InChan().Return(inChan).AnyTimes()
		fth.MockFilterRunner.EXPECT().Name().Return("diskstats").AnyTimes()
		fth.MockFilterRunner.EXPECT().Inject(pack).Do(func(pack *pipeline.PipelinePack) {
			msg := pack.Message
			pack.Message = new(message.Message)
			retMsgChan <- msg
		}).Return(true).AnyTimes()

		err := filter.Init(conf)
		c.Assume(err, gs.IsNil)

		c.Specify("should fill a cbuf with diskstats data", func() {
			go func() {
				errChan <- filter.Run(fth.MockFilterRunner, fth.MockHelper)
			}()

			// Iterate 4 times since the first one doesn't actually set the cbuf
			// in order to set the delta in the cbuf
			for i := 1; i <= 4; i++ {
				// Fill in the fields
				for i, name := range field_names {
					fields[i], _ = message.NewField(name, fieldVal, "")
				}
				// Scale up the value so we can see the delta growing
				// by 100 each iteration
				fieldVal += i * 100

				t := int64(i * 1000000000)
				pack.Message = msg
				pack.Message.SetTimestamp(t)

				// Feed in a pack
				inChan <- pack
				pack = <-recycleChan
			}

			testExpects := map[string]string{
				"Time doing IO": `{"time":2,"rows":3,"columns":4,"seconds_per_row":1,"column_info":[{"name":"TimeWriting","unit":"ms","aggregation":"max"},{"name":"TimeReading","unit":"ms","aggregation":"max"},{"name":"TimeDoingIO","unit":"ms","aggregation":"max"},{"name":"WeightedTimeDoi","unit":"ms","aggregation":"max"}]}
200	200	200	200
400	400	400	400
700	700	700	700
`,
				"Disk Stats": `{"time":2,"rows":3,"columns":6,"seconds_per_row":1,"column_info":[{"name":"WritesCompleted","unit":"per_1_s","aggregation":"none"},{"name":"ReadsCompleted","unit":"per_1_s","aggregation":"none"},{"name":"SectorsWritten","unit":"per_1_s","aggregation":"none"},{"name":"SectorsRead","unit":"per_1_s","aggregation":"none"},{"name":"WritesMerged","unit":"per_1_s","aggregation":"none"},{"name":"ReadsMerged","unit":"per_1_s","aggregation":"none"}]}
100	100	100	100	100	100
200	200	200	200	200	200
300	300	300	300	300	300
`,
			}

			timer <- time.Now()
			for i := 0; i < 2; i++ {
				m := <-retMsgChan
				name, ok := m.GetFieldValue("payload_name")
				c.Assume(ok, gs.IsTrue)
				nameVal, ok := name.(string)
				c.Assume(ok, gs.IsTrue)
				c.Expect(m.GetPayload(), gs.Equals, testExpects[nameVal])
			}
		})

		close(inChan)
		c.Expect(<-errChan, gs.IsNil)
	})

}
