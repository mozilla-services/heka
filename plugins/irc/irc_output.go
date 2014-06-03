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
#   Jose Donizetti (jdbjunior@gmail.com)
#
# ***** END LICENSE BLOCK *****/

package irc

import (
  "fmt"
  "time"
  . "github.com/mozilla-services/heka/pipeline"
  "net"
  "errors"
)

type IrcOutput struct {
  conn net.Conn
  conf *IrcOutputConfig
  encoder Encoder
  address *net.TCPAddr
}

type IrcOutputConfig struct {
  Address string
  Room string
  Nickname string
  Username string
}

func (t *IrcOutput) ConfigStruct() interface{} {
  return &IrcOutputConfig{
    Address:  "localhost:6667",
  }
}

func (irc *IrcOutput) Init(config interface{}) (err error) {
  irc.conf = config.(*IrcOutputConfig)

  if irc.conf.Room == "" {
    return errors.New("Room must be specified")
  }

  if irc.conf.Nickname == "" {
    return errors.New("Nickname must be specified")
  }

  irc.address, err = net.ResolveTCPAddr("tcp", irc.conf.Address)

  return
}

func (irc *IrcOutput) Run(or OutputRunner, h PluginHelper) (
  err error) {
  var (
    ok          = true
    pack        *PipelinePack
    inChan      = or.InChan()
    stopChan    = make(chan bool, 1)
  )

  defer func() {
    if irc.conn != nil {
      irc.conn.Close()
      irc.conn = nil
    }
  }()

  irc.encoder = or.Encoder()

  if irc.encoder == nil {
    return errors.New("Encoder required.")
  }

  for ok {
    select {
    case pack, ok = <-inChan:
      if !ok {
        stopChan <- true
        break
      }

      if contents, err := irc.encoder.Encode(pack); err == nil {
        if err := irc.connect(); err != nil {
          return err
        }

        irc.join()
        irc.privmsg(contents)
      }
      pack.Recycle()
    case <-stopChan:
      ok = false
    }
  }

  return
}

func (irc *IrcOutput) connect() (err error) {
  if irc.conn != nil {
    return
  }

  conn, err := net.DialTCP("tcp", nil, irc.address)

  if err != nil {
    return err
  }

  irc.conn = conn
  irc.nick()
  irc.user()

  time.Sleep(time.Second)

  go irc.ping()

  return
}

func (irc *IrcOutput) nick(){
  irc.execCmd(fmt.Sprintf("NICK %s", irc.conf.Nickname))
}

func (irc *IrcOutput) user(){
  irc.execCmd(fmt.Sprintf("USER %s 8 * : %s", irc.conf.Nickname, irc.conf.Username))
}

func (irc *IrcOutput) join(){
  irc.execCmd(fmt.Sprintf("JOIN %s", irc.conf.Room))
}

func (irc *IrcOutput) privmsg(message []byte){
  irc.execCmd(fmt.Sprintf("PRIVMSG %s :%s", irc.conf.Room, string(message)))
}

func (irc *IrcOutput) ping(){
  ticker := time.NewTicker(5 * time.Minute)
  for {
    select {
    case <-ticker.C:
      irc.execCmd(fmt.Sprintf("PING %d", time.Now().UnixNano()))
    }
  }
}

func (irc *IrcOutput) execCmd(message string){
  irc.conn.Write([]byte(message + "\r\n"))
}

func init() {
  RegisterPlugin("IrcOutput", func() interface{} {
    return new(IrcOutput)
  })
}
