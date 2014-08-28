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
#   Chance Zibolski (chance.zibolski@gmail.com)
#
#***** END LICENSE BLOCK *****/

package irc

import (
	"errors"
	"github.com/thoj/go-ircevent"
)

type MockIrcConnection struct {
	server string
	// Our code only uses 1 callback per event, so this is fine.
	callbacks      map[string]func(*irc.Event)
	quit           bool
	msgs           map[string][]string
	joinedChannels map[string]bool
	Error          chan error

	failReconnect bool
	failJoin      bool
	// These are used in the tests to allow the test to wait for specific
	// actions to finish so we don't have race conditions
	connected chan bool
	delivered chan bool
}

var (
	mockIrcConn          *MockIrcConnection
	mockErrFailReconnect = errors.New("Couldn't connect")
)

func NewMockIrcConn(config *IrcOutputConfig) (IrcConnection, error) {
	mockIrcConn = new(MockIrcConnection)
	mockIrcConn.callbacks = make(map[string]func(*irc.Event))
	mockIrcConn.msgs = make(map[string][]string)
	mockIrcConn.Error = make(chan error, 2)
	mockIrcConn.connected = make(chan bool, 2)
	mockIrcConn.delivered = make(chan bool, 10)
	mockIrcConn.joinedChannels = make(map[string]bool)
	return mockIrcConn, nil
}

func (conn *MockIrcConnection) Privmsg(channel, message string) {
	conn.msgs[channel] = append(conn.msgs[channel], message)
	mockIrcConn.delivered <- true
}

func (conn *MockIrcConnection) Join(channel string) {
	if mockIrcConn.failJoin {
		return
	}
	args := []string{"", channel}
	event := &irc.Event{Arguments: args, Code: IRC_RPL_ENDOFNAMES}
	conn.RunCallbacks(event)
}

func (conn *MockIrcConnection) Part(channel string) {
	args := []string{"", channel}
	event := &irc.Event{Arguments: args, Code: PART}
	conn.RunCallbacks(event)
}

func (conn *MockIrcConnection) Quit() {
	event := &irc.Event{Code: QUIT}
	conn.RunCallbacks(event)
	conn.quit = true
}

func (conn *MockIrcConnection) Connect(server string) error {
	conn.server = server
	event := &irc.Event{Code: CONNECTED}
	conn.RunCallbacks(event)
	conn.connected <- true
	return nil
}

func (conn *MockIrcConnection) Disconnect() {
	event := &irc.Event{Code: ERROR}
	conn.connected <- false
	conn.RunCallbacks(event)
	conn.ErrorChan() <- irc.ErrDisconnected
}

func (conn *MockIrcConnection) Reconnect() error {
	if conn.failReconnect {
		return mockErrFailReconnect
	}
	conn.Connect(conn.server)
	return nil
}

func (conn *MockIrcConnection) AddCallback(event string,
	callback func(*irc.Event)) string {
	conn.callbacks[event] = callback
	return ""
}

func (conn *MockIrcConnection) ClearCallback(event string) bool {
	delete(conn.callbacks, event)
	return true
}

func (conn *MockIrcConnection) RunCallbacks(event *irc.Event) {
	if cb, ok := conn.callbacks[event.Code]; ok {
		cb(event)
	}
}

func (conn *MockIrcConnection) ErrorChan() chan error {
	return conn.Error
}

func kick(conn *MockIrcConnection, channel string) {
	args := []string{channel}
	event := &irc.Event{Arguments: args, Code: KICK}
	conn.RunCallbacks(event)
}
