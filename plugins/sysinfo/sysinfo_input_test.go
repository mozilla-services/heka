package sysinfo

import (
	"code.google.com/p/gomock/gomock"
	. "github.com/mozilla-services/heka/pipeline"
	pipeline_ts "github.com/mozilla-services/heka/pipeline/testsupport"
	"github.com/mozilla-services/heka/pipelinemock"
	plugins_ts "github.com/mozilla-services/heka/plugins/testsupport"
	gs "github.com/rafrombrc/gospec/src/gospec"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"testing"
	"time"
)

func TestAllSpecs(t *testing.T) {
	r := gs.NewRunner()
	r.Parallel = false

	r.AddSpec(SysinfoInputSpec)
	gs.MainGoTest(r, t)
}

func FakeSysinfo(info *syscall.Sysinfo_t) error {
	return nil
}

func SysinfoInputSpec(c gs.Context) {
	t := &pipeline_ts.SimpleT{}
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	pConfig := NewPipelineConfig(nil)

	var wg sync.WaitGroup
	errChan := make(chan error, 1)
	tickChan := make(chan time.Time)

	c.Specify("A SysinfoInput", func() {
		input := new(SysinfoInput)
		input.getSysinfo = FakeSysinfo
		pwd, err := os.Getwd()
		c.Assume(err, gs.IsNil)
		input.proc_meminfo_location = filepath.Join(pwd, "testsupport", "meminfo.txt")

		ith := new(plugins_ts.InputTestHelper)
		ith.MockHelper = pipelinemock.NewMockPluginHelper(ctrl)
		ith.MockInputRunner = pipelinemock.NewMockInputRunner(ctrl)

		ith.Pack = NewPipelinePack(pConfig.InputRecycleChan())
		ith.PackSupply = make(chan *PipelinePack, 2)
		ith.PackSupply <- ith.Pack
		ith.PackSupply <- ith.Pack

		config := input.ConfigStruct().(*SysinfoInputConfig)

		c.Specify("that is started", func() {
			startInput := func() {
				wg.Add(1)
				go func() {
					errChan <- input.Run(ith.MockInputRunner, ith.MockHelper)
					wg.Done()
				}()
			}

			ith.MockInputRunner.EXPECT().Name().Return("SysinfoInput").AnyTimes()

			c.Specify("works without a decoder", func() {
				ith.MockHelper.EXPECT().PipelineConfig().Return(pConfig)
				ith.MockInputRunner.EXPECT().Ticker().Return(tickChan)
				ith.MockInputRunner.EXPECT().InChan().Return(ith.PackSupply)

				err := input.Init(config)
				c.Assume(err, gs.IsNil)

				c.Expect(input.DecoderName, gs.Equals, "")

				startInput()
				tickChan <- time.Now()

				ith.MockInputRunner.EXPECT().Inject(ith.Pack).Times(2)

				input.Stop()
				wg.Wait()
				c.Expect(<-errChan, gs.IsNil)

			})

			c.Specify("works with a decoder", func() {
				ith.MockHelper.EXPECT().PipelineConfig().Return(pConfig)
				ith.MockInputRunner.EXPECT().InChan().Return(ith.PackSupply)
				ith.MockInputRunner.EXPECT().Ticker().Return(tickChan)

				decoderName := "ScribbleDecoder"
				config.DecoderName = decoderName

				mockDecoderRunner := pipelinemock.NewMockDecoderRunner(ctrl)

				dRunnerInChan := make(chan *PipelinePack, 2)
				mockDecoderRunner.EXPECT().InChan().Return(dRunnerInChan).Times(2)

				ith.MockHelper.EXPECT().DecoderRunner(decoderName,
					"SysinfoInput-"+decoderName).Return(mockDecoderRunner, true)

				err := input.Init(config)
				c.Assume(err, gs.IsNil)
				c.Expect(input.DecoderName, gs.Equals, decoderName)

				startInput()
				tickChan <- time.Now()

				input.Stop()
				wg.Wait()
				c.Expect(<-errChan, gs.IsNil)
			})

		})

	})
}
