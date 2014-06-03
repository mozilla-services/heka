/***** BEGIN LICENSE BLOCK *****
# This Source Code Form is subject to the terms of the Mozilla Public
# License, v. 2.0. If a copy of the MPL was not distributed with this file,
# You can obtain one at http://mozilla.org/MPL/2.0/.
#
# The Initial Developer of the Original Code is the Mozilla Foundation.
# Portions created by the Initial Developer are Copyright (C) 2012-2014
# the Initial Developer. All Rights Reserved.
#
# Contributor(s):
#   Jose Donizetti (jdbjunior@gmail.com)
#
# ***** END LICENSE BLOCK *****/
package irc

import (
  "code.google.com/p/gomock/gomock"
  . "github.com/mozilla-services/heka/pipeline"
  pipeline_ts "github.com/mozilla-services/heka/pipeline/testsupport"
  plugins_ts "github.com/mozilla-services/heka/plugins/testsupport"
  gs "github.com/rafrombrc/gospec/src/gospec"
  "github.com/mozilla-services/heka/plugins"
  "testing"
  "strings"
  "net"
)

func TestAllSpecs(t *testing.T) {
  r := gs.NewRunner()
  r.Parallel = false

  r.AddSpec(TcpOutputSpec)

  gs.MainGoTest(r, t)
}


func TcpOutputSpec(c gs.Context) {
  t := new(pipeline_ts.SimpleT)
  ctrl := gomock.NewController(t)
  defer ctrl.Finish()

  oth := plugins_ts.NewOutputTestHelper(ctrl)
  inChan := make(chan *PipelinePack, 1)
  pConfig := NewPipelineConfig(nil)

  encoder := new(plugins.PayloadEncoder)
  econfig := encoder.ConfigStruct().(*plugins.PayloadEncoderConfig)
  encoder.Init(econfig)

  c.Specify("IrcOutput", func() {
    ircOutput := new(IrcOutput)

    config := ircOutput.ConfigStruct().(*IrcOutputConfig)
    config.Nickname = "heka"
    config.Room = "#heka"

    msg := pipeline_ts.GetTestMessage()
    pack := NewPipelinePack(pConfig.InputRecycleChan())
    pack.Message = msg
    pack.Decoded = true
    inChanCall := oth.MockOutputRunner.EXPECT().InChan().AnyTimes()
    inChanCall.Return(inChan)
    runnerName := oth.MockOutputRunner.EXPECT().Name().AnyTimes()
    runnerName.Return("IrcOutput")
    oth.MockOutputRunner.EXPECT().Encoder().Return(encoder).AnyTimes()

    c.Specify("writes out to the network", func() {
      err := ircOutput.Init(config)
      c.Assume(err, gs.IsNil)

      collectData := func(ch chan string) {
        listener, err := net.Listen("tcp", "localhost:6667")
        if err != nil {
          ch <- err.Error()
          return
        }
        ch <- "ready"

        conn, err := listener.Accept()
        if err != nil {
          ch <- err.Error()
          return
        }

        buf := make([]byte, 1000)
        msg := ""
        ok := true
        for ok {
          n, _ := conn.Read(buf)
          m := string(buf[0:n])

          if strings.Contains(m, "PING"){
            continue
          }

          msg += m

          if strings.Contains(m, "PRIVMSG"){
            ok = false
          }
        }
        ch <- msg

        conn.Close()
        listener.Close()
      }
      ch := make(chan string, 1)
      go collectData(ch)
      result := <-ch

      outStr := "Write me out to the network"
      pack.Message.SetPayload(outStr)

      go func() {
        ircOutput.Run(oth.MockOutputRunner, oth.MockHelper)
      }()

      inChan <- pack
      result = <-ch
      expected := "NICK heka\r\nUSER heka 8 * : \r\nJOIN #heka\r\nPRIVMSG #heka :Write me out to the network\r\n"

      c.Expect(strings.TrimSpace(result), gs.Equals, strings.TrimSpace(expected))
      close(inChan)
    })
  })
}
