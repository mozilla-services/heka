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
	"crypto/tls"
	"errors"
	"fmt"
	"github.com/mozilla-services/heka/plugins/tcp"
	"github.com/thoj/go-ircevent"
	"time"
)

const (
	// These are replies from the Irc Server
	CONNECTED                = "001"
	ERROR                    = "ERROR" // This is what we get on a disconnect
	QUIT                     = "QUIT"
	PART                     = "PART"
	KICK                     = "KICK"
	IRC_RPL_ENDOFNAMES       = "366"
	IRC_NOSUCHCHANNEL        = "403"
	IRC_ERR_YOUREBANNEDCREEP = "465"
	IRC_ERR_CHANNELISFULL    = "471"
	IRC_ERR_INVITEONLYCHAN   = "473"
	IRC_ERR_BANNEDFROMCHAN   = "474"
	IRC_ERR_BADCHANNELKEY    = "475"
	// These are to track our JoinedChannels slice of joined/not joined
	NOTJOINED  int32 = 0
	JOINED     int32 = 1
	CANNOTJOIN int32 = 2
)

var (
	ErrOutQueueFull        = errors.New("Dropped message. OutQueue is full.")
	ErrBacklogQueueFull    = errors.New("Dropped message. BacklogQueue is full.")
	ErrBannedFromServer    = errors.New("Banned from irc server. Exiting plugin.")
	ErrNoJoinableChannels  = errors.New("No joinable channels. Exiting plugin.")
	ErrReconnecting        = "Error reconnecting: %s"
	ErrUsedUpRetryAttempts = "Used up retry attempts attempting to join %s. " +
		"No longer going to attempt to join channel."
	DisconnectMsg  = "Disconnected from Irc. Retrying to connect in 3 seconds.."
	ReconnectedMsg = "Reconnected to Irc!"
)

type IrcCannotJoinError struct {
	Channel string
	Reason  string
}

func (e IrcCannotJoinError) Error() string {
	return fmt.Sprintf("An error occurred joining channel: %s. Reason: %s",
		e.Channel, e.Reason)
}

type IrcConnection interface {
	Privmsg(channel, message string)
	Join(channel string)
	Part(channel string)
	Quit()
	Connect(server string) error
	Disconnect()
	Reconnect() error
	AddCallback(event string, callback func(event *irc.Event)) string
	ClearCallback(event string) bool
	RunCallbacks(event *irc.Event)
	ErrorChan() chan error
}

// NewIrcConn creates an *irc.Connection. It handles using Heka's tcp plugin to
// create a cryto/tls config
func NewIrcConn(config *IrcOutputConfig) (IrcConnection, error) {
	conn := irc.IRC(config.Nick, config.Ident)
	if conn == nil {
		return nil, errors.New("Nick or Ident cannot be blank")
	}
	if config.Server == "" {
		return nil, errors.New("Irc server cannot be blank.")
	}
	if len(config.Channels) < 1 {
		return nil, errors.New("Need at least 1 channel to join.")
	}
	var tlsConf *tls.Config = nil
	var err error = nil
	if tlsConf, err = tcp.CreateGoTlsConfig(&config.Tls); err != nil {
		return nil, fmt.Errorf("TLS init error: %s", err)
	}
	conn.UseTLS = config.UseTLS
	conn.TLSConfig = tlsConf
	conn.Password = config.Password
	conn.Timeout = time.Duration(config.Timeout) * time.Second
	conn.VerboseCallbackHandler = config.VerboseIRCLogging
	return conn, nil
}
