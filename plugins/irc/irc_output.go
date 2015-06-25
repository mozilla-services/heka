/***** BEGIN LICENSE BLOCK *****
# This Source Code Form is subject to the terms of the Mozilla Public
# License, v. 2.0. If a copy of the MPL was not distributed with this file,
# You can obtain one at http://mozilla.org/MPL/2.0/.
#
# The Initial Developer of the Original Code is the Mozilla Foundation.
# Portions created by the Initial Developer are Copyright (C) 2014-2015
# the Initial Developer. All Rights Reserved.
#
# Contributor(s):
#   Chance Zibolski (chance.zibolski@gmail.com)
#   Rob Miller (rmiller@mozilla.com)
#
#***** END LICENSE BLOCK *****/

package irc

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/mozilla-services/heka/pipeline"
	"github.com/mozilla-services/heka/plugins/tcp"
	"github.com/thoj/go-ircevent"
)

func init() {
	pipeline.RegisterPlugin("IrcOutput", func() interface{} {
		output := new(IrcOutput)
		output.InitIrcCon = NewIrcConn
		return output
	})
}

type IrcOutputConfig struct {
	Server   string   `toml:"server"`
	Nick     string   `toml:"nick"`
	Ident    string   `toml:"ident"`
	Password string   `toml:"password"`
	Channels []string `toml:"channels"`
	Timeout  uint     `toml:"timeout"`
	UseTLS   bool     `toml:"use_tls"`
	// Subsection for TLS configuration.
	Tls tcp.TlsConfig
	// This controls the size of the OutQueue and Backlog queue for messages.
	QueueSize    int  `toml:"queue_size"`
	RejoinOnKick bool `toml:"rejoin_on_kick"`
	// Default interval at which Irc messages will be sent is minimum of 2
	// seconds between messages.
	TickerInterval uint `toml:"ticker_interval"`
	// Number of seconds to wait before reconnect
	TimeBeforeReconnect uint `toml:"time_before_reconnect"`
	// Number of seconds to wait before attempting to rejoin a channel
	TimeBeforeRejoin uint `toml:"time_before_rejoin"`
	// Max number of attempts to rejoin an irc channel before giving up
	MaxJoinRetries    uint `toml:"max_join_retries"`
	VerboseIRCLogging bool `toml:"verbose_irc_logging"`
}

func (output *IrcOutput) ConfigStruct() interface{} {
	return &IrcOutputConfig{
		Timeout:             10,
		QueueSize:           100,
		TickerInterval:      uint(2),
		TimeBeforeReconnect: uint(3),
		TimeBeforeRejoin:    uint(3),
		MaxJoinRetries:      uint(3),
	}
}

type IrcMsgQueue chan IrcMsg

type IrcOutput struct {
	*IrcOutputConfig
	// We explicitly have a list of channels which is different from the config.
	Channels       []string
	InitIrcCon     func(config *IrcOutputConfig) (IrcConnection, error)
	Conn           IrcConnection
	OutQueue       IrcMsgQueue
	BacklogQueues  []IrcMsgQueue
	JoinedChannels []int32
	runner         pipeline.OutputRunner
	die            chan bool
	killProcessing chan bool
	wg             sync.WaitGroup
	numRetries     map[string]uint
}

type IrcMsg struct {
	Output     []byte
	IrcChannel string
	Idx        int
}

func (output *IrcOutput) Init(config interface{}) error {
	conf := config.(*IrcOutputConfig)
	output.IrcOutputConfig = conf
	conn, err := output.InitIrcCon(conf)
	if err != nil {
		return fmt.Errorf("Error setting up Irc Connection: %s", err)
	}
	output.Conn = conn

	numChannels := len(conf.Channels)
	output.Channels = make([]string, numChannels)
	for i, ircChannel := range conf.Channels {
		parts := strings.SplitN(ircChannel, " ", 2)
		if parts != nil {
			ircChannel = parts[0]
		}
		output.Channels[i] = ircChannel
	}

	// Create our chans for passing messages from the main runner InChan to
	// the irc channels
	output.JoinedChannels = make([]int32, numChannels)
	output.OutQueue = make(IrcMsgQueue, output.QueueSize)
	output.BacklogQueues = make([]IrcMsgQueue, numChannels)
	for queue := range output.BacklogQueues {
		output.BacklogQueues[queue] = make(IrcMsgQueue, output.QueueSize)
	}
	output.die = make(chan bool)
	output.killProcessing = make(chan bool)
	output.numRetries = make(map[string]uint)

	return nil
}

func (output *IrcOutput) Run(runner pipeline.OutputRunner,
	helper pipeline.PluginHelper) error {
	if runner.Encoder() == nil {
		return errors.New("Encoder required.")
	}

	output.runner = runner

	// Register callbacks to handle events
	registerCallbacks(output)

	var err error

	// Connect to the Irc Server
	err = output.Conn.Connect(output.Server)
	if err != nil {
		return fmt.Errorf("Unable to connect to irc server %s: %s",
			output.Server, err)
	}

	// Start a goroutine for recieving messages, and throttling before sending
	// to the Irc Server
	output.wg.Add(1)
	go processOutQueue(output)

	var outgoing []byte
	ok := true
	inChan := runner.InChan()
	var pack *pipeline.PipelinePack
	for ok {
		select {
		case pack, ok = <-inChan:
		case <-output.die:
			ok = false
		}
		if !ok {
			break
		}
		outgoing, err = runner.Encode(pack)
		if err != nil {
			output.runner.UpdateCursor(pack.QueueCursor)
			err = fmt.Errorf("can't encode: %s", err.Error())
			pack.Recycle(err)
			continue
		} else if outgoing != nil {
			// Send the message to each irc channel. If the out queue is full,
			// then we need to drop the message and log an error.
			sentAny := false
			for i, ircChannel := range output.Channels {
				ircMsg := IrcMsg{outgoing, ircChannel, i}
				select {
				case output.OutQueue <- ircMsg:
					sentAny = true
				default:
					output.runner.LogError(ErrOutQueueFull)
				}
			}
			if !sentAny {
				err = pipeline.NewRetryMessageError("IRC delivery failed")
				pack.Recycle(err)
				continue
			}
		}
		output.runner.UpdateCursor(pack.QueueCursor)
		pack.Recycle(nil)
	}
	output.cleanup()
	return nil
}

func (output *IrcOutput) CleanupForRestart() {
	// Intentially left empty. Cleanup happens in Run()
}

// Checks if there are no joinable channels left.
func (output *IrcOutput) noneJoinable() bool {
	for i := range output.Channels {
		if atomic.LoadInt32(&output.JoinedChannels[i]) != CANNOTJOIN {
			return false
		}
	}
	return true
}

func (output *IrcOutput) canJoin(ircChan string) bool {
	joinable := false
	numChannels := len(output.Channels)
	numUnjoinable := 0
	for i, channel := range output.Channels {
		if atomic.LoadInt32(&output.JoinedChannels[i]) != CANNOTJOIN {
			if ircChan == channel {
				joinable = true
			}
		} else {
			numUnjoinable++
		}
	}
	if numUnjoinable == numChannels {
		output.runner.LogError(ErrNoJoinableChannels)
		output.die <- true
	}
	return joinable
}

func (output *IrcOutput) Join(ircChan string) {
	if output.canJoin(ircChan) {
		for i, ircChannel := range output.Channels {
			if ircChannel == ircChan {
				output.Conn.Join(output.IrcOutputConfig.Channels[i])
			}
		}
	}
}

func (output *IrcOutput) handleCannotJoin(event *irc.Event) {
	ircChan := event.Arguments[1]
	reason := event.Arguments[2]
	output.runner.LogError(IrcCannotJoinError{ircChan, reason})
	output.updateJoinList(ircChan, CANNOTJOIN)
	if output.noneJoinable() {
		output.runner.LogError(ErrNoJoinableChannels)
		output.die <- true
	}
}

// Privmsg wraps the irc.Privmsg by accepting an ircMsg struct, and checking if
// we've joined a channel before trying to send a message to it. Returns whether
// or not the message was successfully sent.
func (output *IrcOutput) Privmsg(ircMsg *IrcMsg) bool {
	idx := ircMsg.Idx
	if atomic.LoadInt32(&output.JoinedChannels[idx]) == JOINED {
		output.Conn.Privmsg(ircMsg.IrcChannel, string(ircMsg.Output))
		return true
	}
	return false
}

// updateJoinList atomically updates our global slice of joined channels for a
// particular irc channel. It sets the irc channel's joined status to 'status'.
// Returns whether or not it found the irc channel in our slice.
func (output *IrcOutput) updateJoinList(ircChan string, status int32) bool {
	for i, channel := range output.Channels {
		if ircChan == channel {
			// Update if we have or haven't joined the channel
			atomic.StoreInt32(&output.JoinedChannels[i], status)
			return true
		}
	}
	return false
}

// updateJoinListAll sets the status of all irc channels in our config to
// 'status'
func (output *IrcOutput) updateJoinListAll(status int32) {
	for channel := range output.Channels {
		atomic.StoreInt32(&output.JoinedChannels[channel], status)
	}
}

// sendFromOutQueue attempts to send a message to the irc channel specified in
// the ircMsg struct. If sending fails due to not being in the irc channel, it
// will put the message into that irc channel's backlog queue. If the queue is
// full it will drop the message and log an error.
// It returns whether or not a message was successfully delivered to an
// irc channel.
func sendFromOutQueue(output *IrcOutput, ircMsg *IrcMsg) bool {

	if output.Privmsg(ircMsg) {
		return true
	} else {
		// We haven't joined this channel yet, so we need to send
		// the message to the backlog queue of messages

		// Get the proper Channel for the backlog
		idx := ircMsg.Idx
		backlogQueue := output.BacklogQueues[idx]
		select {
		// try to put the message into the backlog queue
		case backlogQueue <- *ircMsg:

		default:
			// Failed to put, which means the backlog for this Irc
			// channel is full. So drop it and log a message.
			output.runner.LogError(
				fmt.Errorf("%s Channel: %s.",
					ErrBacklogQueueFull, ircMsg.IrcChannel))
		}
		return false
	}
}

// sendFromBacklogQueue attempts to send a message from the first backlog queue
// which has a message in it. It returns whether or not a message was
// successfully delivered to an irc channel.
func sendFromBacklogQueue(output *IrcOutput) bool {

	var ircMsg IrcMsg
	// No messages in the out queue, so lets try the backlog queue
	for i, queue := range output.BacklogQueues {
		if atomic.LoadInt32(&output.JoinedChannels[i]) != JOINED {
			continue
		}
		select {
		case ircMsg = <-queue:
			if output.Privmsg(&ircMsg) {
				return true
			}
		default:
			// No backed up messages for this irc channel
		}
	}
	return false
}

// processOutQueue attempts to send an Irc message from the OutQueue, or the
// BacklogQueue if nothing is in the OutQueue. It is throttled by a ticker to
// prevent flooding the Irc server.
func processOutQueue(output *IrcOutput) {
	var delivered bool
	var ircMsg IrcMsg
	ok := true
	ticker := output.runner.Ticker()
	for ok {
		delivered = false
		// Either wait for a tick, or the die chan to close.
		// If the die chan closes we want to break out of this loop to cleanup.
		select {
		case <-ticker:
		// We should only ever close the killProcessing channel, we're never
		// actually reading from this channel.
		case _, ok = <-output.killProcessing:
			continue
		}
		select {
		case ircMsg, ok = <-output.OutQueue:
			if !ok {
				// We havent actually delivered but we want to escape that
				// loop.
				delivered = true
				// Time to cleanup, and close our chans
				break
			}
			delivered = sendFromOutQueue(output, &ircMsg)
		default:
			// Just here to prevent blocking
		}
		if !delivered {
			sendFromBacklogQueue(output)
		}
	}
	// To prevent reconnecting if we get disconnected for flooding when quiting.
	output.Conn.ClearCallback(ERROR)
	// Cleanup heka
	for _, queue := range output.BacklogQueues {
		close(queue)
	}
	// Try to send the rest of our msgs in the backlog before quitting.
	for _, queue := range output.BacklogQueues {
		for msg := range queue {
			output.Privmsg(&msg)
		}
	}
	// Once we have no messages left, we can quit
	output.Conn.Quit()
	output.Conn.Disconnect()
	output.wg.Done()
}

// registerCallbacks sets up all the event handler callbacks for recieving
// particular irc events.
func registerCallbacks(output *IrcOutput) {
	// add a callback to check if we've gotten successfully connected
	output.Conn.AddCallback(CONNECTED, func(event *irc.Event) {
		// We use the config channels for joining,
		// since they contain the channel keys
		for _, ircChan := range output.IrcOutputConfig.Channels {
			output.Conn.Join(ircChan)
		}
	})

	// Once we've recieved the names list, we've successfully joined the channel
	// And should begin processing Heka messages
	output.Conn.AddCallback(IRC_RPL_ENDOFNAMES, func(event *irc.Event) {
		// This is the actual irc channel name (ie: #heka)
		ircChan := event.Arguments[1]
		output.updateJoinList(ircChan, JOINED)
		// Successful joins mean we should reset our number of retries
		output.numRetries[ircChan] = 0
	})

	// We want to handle errors (disconnects) ourself.
	output.Conn.ClearCallback(ERROR)
	output.Conn.AddCallback(ERROR, func(event *irc.Event) {
		output.updateJoinListAll(NOTJOINED)
		output.runner.LogMessage(DisconnectMsg)
		timer := time.NewTimer(time.Second * time.Duration(output.TimeBeforeReconnect))
		<-timer.C
		err := output.Conn.Reconnect()
		if err != nil {
			output.runner.LogError(fmt.Errorf(ErrReconnecting, err))
			output.Conn.ClearCallback(ERROR)
			output.die <- true
			return
		}
		output.runner.LogMessage(ReconnectedMsg)
	})

	output.Conn.AddCallback(KICK, func(event *irc.Event) {
		ircChan := event.Arguments[0]
		if output.RejoinOnKick {
			output.updateJoinList(ircChan, NOTJOINED)
			output.Join(ircChan)
		} else {
			output.updateJoinList(ircChan, CANNOTJOIN)
			// Since we'll never try to join a kicked channel again, we check if
			// we need to exit (there are no channels left)
			if output.noneJoinable() {
				output.runner.LogError(ErrNoJoinableChannels)
				output.die <- true
			}
		}
	})

	// The following 4 callbacks are cases where we can't join a channel ever.
	// So we should mark them as cannot join, log an error, and never attempt to
	// join them again
	cannotJoinEvents := []string{
		IRC_ERR_BADCHANNELKEY,
		IRC_ERR_BANNEDFROMCHAN,
		IRC_NOSUCHCHANNEL,
		IRC_ERR_INVITEONLYCHAN,
	}

	for i := range cannotJoinEvents {
		output.Conn.AddCallback(cannotJoinEvents[i], func(event *irc.Event) {
			output.handleCannotJoin(event)
		})
	}

	output.Conn.AddCallback(IRC_ERR_CHANNELISFULL, func(event *irc.Event) {
		ircChan := event.Arguments[1]
		// Check if we can even join this channel.
		if !output.canJoin(ircChan) {
			return
		}
		// Then check if we've used up our max number of retries
		if output.numRetries[ircChan] >= output.MaxJoinRetries {
			output.updateJoinList(ircChan, CANNOTJOIN)
			err := fmt.Errorf(ErrUsedUpRetryAttempts, ircChan)
			output.runner.LogError(err)
			return
		}
		// If we've gotten this far, we should send a message about why we
		// couldn't join
		reason := event.Arguments[2]
		err := fmt.Errorf("%s. Retrying in %d seconds.",
			IrcCannotJoinError{ircChan, reason}, output.TimeBeforeRejoin)
		output.runner.LogError(err)
		// Increment our number of retries each time we try to join
		output.numRetries[ircChan]++
		// Wait the set amount of time before joining
		timer := time.NewTimer(time.Second * time.Duration(output.TimeBeforeRejoin))
		<-timer.C
		output.Join(ircChan)
	})

	output.Conn.AddCallback(IRC_ERR_YOUREBANNEDCREEP, func(event *irc.Event) {
		output.runner.LogError(ErrBannedFromServer)
		output.die <- true
	})

	// These next 2 events shouldn't really matter much, but we should update
	// the JoinList anyways.
	output.Conn.AddCallback(QUIT, func(event *irc.Event) {
		output.updateJoinListAll(NOTJOINED)
	})

	output.Conn.AddCallback(PART, func(event *irc.Event) {
		ircChan := event.Arguments[1]
		output.updateJoinList(ircChan, NOTJOINED)
	})

}

func (output *IrcOutput) cleanup() {
	output.Conn.ClearCallback(ERROR)
	close(output.OutQueue)
	close(output.killProcessing)
	close(output.die)
	errChan := output.Conn.ErrorChan()
	for {
		err := <-errChan
		if err == irc.ErrDisconnected {
			break
		}
	}
	output.wg.Wait()
}
