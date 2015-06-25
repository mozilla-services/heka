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
	"fmt"
	"sync"
	"testing"
	"time"

	. "github.com/mozilla-services/heka/pipeline"
	pipeline_ts "github.com/mozilla-services/heka/pipeline/testsupport"
	"github.com/mozilla-services/heka/plugins"
	plugins_ts "github.com/mozilla-services/heka/plugins/testsupport"
	"github.com/rafrombrc/gomock/gomock"
	gs "github.com/rafrombrc/gospec/src/gospec"
	"github.com/thoj/go-ircevent"
)

func TestAllSpecs(t *testing.T) {
	r := gs.NewRunner()
	r.Parallel = false
	r.AddSpec(IrcOutputSpec)
	gs.MainGoTest(r, t)
}

func IrcOutputSpec(c gs.Context) {
	t := new(pipeline_ts.SimpleT)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	pipelineConfig := NewPipelineConfig(nil)
	outTestHelper := plugins_ts.NewOutputTestHelper(ctrl)

	var wg sync.WaitGroup

	errChan := make(chan error, 1)
	tickChan := make(chan time.Time)
	inChan := make(chan *PipelinePack, 5)

	c.Specify("An IrcOutput", func() {
		ircOutput := new(IrcOutput)
		ircOutput.InitIrcCon = NewMockIrcConn
		encoder := new(plugins.PayloadEncoder)
		encoder.Init(&plugins.PayloadEncoderConfig{})
		config := ircOutput.ConfigStruct().(*IrcOutputConfig)
		config.Server = "irc.example.org"
		config.Nick = "heka_bot"
		config.Ident = "heka"
		config.Channels = []string{"#test_channel"}

		msg := pipeline_ts.GetTestMessage()
		pack := NewPipelinePack(pipelineConfig.InputRecycleChan())
		pack.Message = msg

		c.Specify("requires an encoder", func() {
			err := ircOutput.Init(config)
			c.Assume(err, gs.IsNil)

			outTestHelper.MockOutputRunner.EXPECT().Encoder().Return(nil)
			err = ircOutput.Run(outTestHelper.MockOutputRunner, outTestHelper.MockHelper)
			c.Expect(err, gs.Not(gs.IsNil))
		})

		c.Specify("that is started", func() {

			outTestHelper.MockOutputRunner.EXPECT().Ticker().Return(tickChan)
			outTestHelper.MockOutputRunner.EXPECT().Encoder().Return(encoder)
			outTestHelper.MockOutputRunner.EXPECT().InChan().Return(inChan)
			outTestHelper.MockOutputRunner.EXPECT().UpdateCursor("").AnyTimes()
			outTestHelper.MockOutputRunner.EXPECT().Encode(pack).Return(encoder.Encode(pack)).AnyTimes()

			startOutput := func() {
				wg.Add(1)
				go func() {
					err := ircOutput.Run(outTestHelper.MockOutputRunner, outTestHelper.MockHelper)
					errChan <- err
					wg.Done()
				}()
				<-mockIrcConn.connected
			}

			c.Specify("sends incoming messages to an irc server", func() {
				err := ircOutput.Init(config)
				c.Assume(err, gs.IsNil)

				c.Assume(len(ircOutput.Channels), gs.Equals, 1)
				ircChan := ircOutput.Channels[0]

				startOutput()

				// Send the data
				inChan <- pack
				// wait for it to arrive
				p := <-ircOutput.OutQueue
				ircOutput.OutQueue <- p
				// once it's arrived send a tick so it gets processed
				tickChan <- time.Now()

				close(inChan)
				wg.Wait()

				// Verify we've exited
				c.Expect(mockIrcConn.quit, gs.IsTrue)

				// verify the message was sent
				msgs := mockIrcConn.msgs[ircChan]
				c.Expect(len(msgs), gs.Equals, 1)
				c.Expect(msgs[0], gs.Equals, string(*msg.Payload))
			})

			c.Specify("drops messages when outqueue is full", func() {
				config.QueueSize = 1
				err := ircOutput.Init(config)
				c.Assume(err, gs.IsNil)

				c.Assume(len(ircOutput.Channels), gs.Equals, 1)
				ircChan := ircOutput.Channels[0]

				outTestHelper.MockOutputRunner.EXPECT().LogError(
					ErrOutQueueFull)
				outTestHelper.MockOutputRunner.EXPECT().LogError(
					fmt.Errorf("%s Channel: %s.", ErrBacklogQueueFull, ircChan))

				startOutput()

				// Need to wait for callbacks to get registered before calling part.
				// If we've called connected, then callbacks have been registered.

				// Leave the irc channel so we can fill up the backlog
				ircOutput.Conn.Part(ircChan)
				c.Expect(ircOutput.JoinedChannels[0], gs.Equals, NOTJOINED)

				// Put some data into the inChan
				inChan <- pack

				// Wait for it to arrive in the OutQueue and put it back
				// We *must* do this before sending the tick to avoid data races
				p := <-ircOutput.OutQueue
				ircOutput.OutQueue <- p

				// One tick so that it goes from the OutQueue into the BacklogQueue
				tickChan <- time.Now()

				// Verify the item is in the backlog queue and put it back
				// (It will never leave this loop if the msg never arrives)
				queue := ircOutput.BacklogQueues[0]
				msg := <-queue
				queue <- msg
				c.Expect(len(queue), gs.Equals, 1)

				// Send another message to fill the OutQueue
				inChan <- pack

				// Wait for it to arrive, again
				p = <-ircOutput.OutQueue
				ircOutput.OutQueue <- p

				// Send the tick after it's in the OutQueue so we can try processing
				// it. This is where we drop it since we cant send, and the
				// BacklogQueue is already full.
				tickChan <- time.Now()

				// Now we want to also cause the OutQueue to drop a message.
				// We don't have to wait for it to arrive in the OutQueue like we do above
				// since it shouldn't make it there.
				inChan <- pack
				inChan <- pack

				close(inChan)
				wg.Wait()

				// Verify we've exited
				c.Expect(mockIrcConn.quit, gs.IsTrue)

				// verify the backlog queue is empty
				c.Expect(len(queue), gs.Equals, 0)

				// verify that no messages were sent.
				msgs := mockIrcConn.msgs[ircChan]
				c.Expect(len(msgs), gs.Equals, 0)
			})

			c.Specify("automatically reconnects on disconnect", func() {
				config.TimeBeforeReconnect = 0
				err := ircOutput.Init(config)
				c.Assume(err, gs.IsNil)

				outTestHelper.MockOutputRunner.EXPECT().LogMessage(DisconnectMsg)
				outTestHelper.MockOutputRunner.EXPECT().LogMessage(ReconnectedMsg)

				startOutput()

				ircOutput.Conn.Disconnect()
				// verify we've been disconnected
				c.Expect(<-mockIrcConn.connected, gs.IsFalse)

				// Wait to become reconnected.
				c.Expect(<-mockIrcConn.connected, gs.IsTrue)

				close(inChan)
				wg.Wait()

				c.Expect(mockIrcConn.quit, gs.IsTrue)
			})

			c.Specify("logs an error and exits from Run() when it cannot reconnect", func() {
				config.TimeBeforeReconnect = 0

				err := ircOutput.Init(config)
				c.Assume(err, gs.IsNil)

				// Explicitly fail to reconnect
				mockIrcConn.failReconnect = true

				outTestHelper.MockOutputRunner.EXPECT().LogMessage(DisconnectMsg)
				outTestHelper.MockOutputRunner.EXPECT().LogError(fmt.Errorf(ErrReconnecting, mockErrFailReconnect))

				startOutput()

				// Need to wait for callbacks to get registered before we disconnect

				ircOutput.Conn.Disconnect()
				// verify we've been disconnected

				// Run() should return without closing the inChan because the plugin
				// automatically cleans up and returns if it fails to reconnect
				wg.Wait()

				c.Expect(mockIrcConn.quit, gs.IsTrue)
				// do this at the end for test cleanup purposes
				close(inChan)
			})

			c.Specify("when kicked from an irc channel", func() {

				c.Specify("rejoins when configured to do so", func() {
					config.RejoinOnKick = true
					err := ircOutput.Init(config)
					c.Assume(err, gs.IsNil)

					c.Assume(len(ircOutput.Channels), gs.Equals, 1)
					ircChan := ircOutput.Channels[0]

					startOutput()

					// trigger a kick event for the irc channel
					kick(mockIrcConn, ircChan)
					c.Expect(ircOutput.JoinedChannels[0], gs.Equals, JOINED)
					// quit
					close(inChan)
				})

				c.Specify("doesnt rejoin when it isnt configured to", func() {
					config.RejoinOnKick = false // this is already the default
					err := ircOutput.Init(config)
					c.Assume(err, gs.IsNil)

					c.Assume(len(ircOutput.Channels), gs.Equals, 1)
					ircChan := ircOutput.Channels[0]

					startOutput()

					// Since this is the only channel we're in, we should get an
					// error that there are no channels to join left after being
					// kicked
					outTestHelper.MockOutputRunner.EXPECT().LogError(ErrNoJoinableChannels)
					// trigger a kick for the irc channel
					kick(mockIrcConn, ircChan)
					c.Expect(ircOutput.JoinedChannels[0], gs.Equals, CANNOTJOIN)
					// We shouldnt need to close the inChan, since we have no
					// joinable channels, we should be cleaning up already.
				})

				wg.Wait()
			})

			c.Specify("when trying to join an unjoinable channel", func() {
				c.Specify("it logs an error", func() {
					config.Channels = []string{"foo", "baz"}
					err := ircOutput.Init(config)
					c.Assume(err, gs.IsNil)

					c.Assume(len(ircOutput.Channels), gs.Equals, 2)
					ircChan := ircOutput.Channels[0]

					startOutput()

					c.Expect(ircOutput.JoinedChannels[0], gs.Equals, JOINED)

					reason := "foo"

					err = IrcCannotJoinError{ircChan, reason}
					outTestHelper.MockOutputRunner.EXPECT().LogError(err)

					args := []string{"", ircChan, reason}
					event := irc.Event{Arguments: args}

					c.Specify("when the channel key is wrong", func() {
						event.Code = IRC_ERR_BADCHANNELKEY
					})

					c.Specify("when banned from the channel", func() {
						event.Code = IRC_ERR_BANNEDFROMCHAN
					})

					c.Specify("when there is no such channel", func() {
						event.Code = IRC_NOSUCHCHANNEL
					})

					c.Specify("when the channel is invite only", func() {
						event.Code = IRC_ERR_INVITEONLYCHAN
					})

					ircOutput.Conn.RunCallbacks(&event)

					c.Expect(ircOutput.JoinedChannels[0], gs.Equals, CANNOTJOIN)

					// should still be unable to join
					ircOutput.Join(ircChan)
					c.Expect(ircOutput.JoinedChannels[0], gs.Equals, CANNOTJOIN)

					close(inChan)
				})

				c.Specify("it exits if there are no joinable channels", func() {
					err := ircOutput.Init(config)
					c.Assume(err, gs.IsNil)

					c.Assume(len(ircOutput.Channels), gs.Equals, 1)
					ircChan := ircOutput.Channels[0]

					startOutput()

					c.Expect(ircOutput.JoinedChannels[0], gs.Equals, JOINED)

					reason := "foo"

					err = IrcCannotJoinError{ircChan, reason}
					outTestHelper.MockOutputRunner.EXPECT().LogError(err)
					outTestHelper.MockOutputRunner.EXPECT().LogError(ErrNoJoinableChannels)

					args := []string{"", ircChan, reason}
					event := irc.Event{Arguments: args, Code: IRC_ERR_BADCHANNELKEY}

					ircOutput.Conn.RunCallbacks(&event)

					// No close since we should exit without.
				})

				wg.Wait()
			})

			c.Specify("when trying to join a full channel", func() {
				config.TimeBeforeRejoin = 0
				err := ircOutput.Init(config)
				c.Assume(err, gs.IsNil)

				c.Assume(len(ircOutput.Channels), gs.Equals, 1)
				ircChan := ircOutput.Channels[0]

				startOutput()

				c.Expect(ircOutput.JoinedChannels[0], gs.Equals, JOINED)

				// We should be able to join before this
				ircOutput.Join(ircChan)

				reason := "full channel"
				err = fmt.Errorf("%s. Retrying in %d seconds.",
					IrcCannotJoinError{ircChan, reason}.Error(),
					ircOutput.TimeBeforeRejoin)

				args := []string{"", ircChan, reason}
				event := irc.Event{Code: IRC_ERR_CHANNELISFULL, Arguments: args}

				c.Specify("logs an error after using up its attempts", func() {
					mockIrcConn.failJoin = true
					// We should leave the channel so that we aren't just
					// causing a failure to join even when we're already in the
					// channel
					ircOutput.Conn.Part(ircChan)

					maxRetries := int(ircOutput.MaxJoinRetries)
					c.Expect(ircOutput.JoinedChannels[0], gs.Equals, NOTJOINED)

					gomock.InOrder(
						outTestHelper.MockOutputRunner.EXPECT().LogError(
							err).Times(maxRetries),
						outTestHelper.MockOutputRunner.EXPECT().LogError(
							fmt.Errorf(ErrUsedUpRetryAttempts, ircChan)),
						outTestHelper.MockOutputRunner.EXPECT().LogError(
							ErrNoJoinableChannels),
					)

					for i := 1; i <= maxRetries; i++ {
						// Trigger the event the number of times
						ircOutput.Conn.RunCallbacks(&event)
						c.Expect(int(ircOutput.numRetries[ircChan]), gs.Equals, i)

					}
					c.Expect(ircOutput.JoinedChannels[0], gs.Equals, NOTJOINED)
					ircOutput.Conn.RunCallbacks(&event)
					// We've used up our attempts,
					c.Expect(ircOutput.JoinedChannels[0], gs.Equals, CANNOTJOIN)
					ircOutput.Conn.RunCallbacks(&event)
					c.Expect(ircOutput.JoinedChannels[0], gs.Equals, CANNOTJOIN)
				})

				c.Specify("resets its attempts when it successful", func() {
					mockIrcConn.failJoin = true
					outTestHelper.MockOutputRunner.EXPECT().LogError(err).Times(2)
					ircOutput.Conn.RunCallbacks(&event)
					c.Expect(int(ircOutput.numRetries[ircChan]), gs.Equals, 1)
					mockIrcConn.failJoin = false
					ircOutput.Conn.RunCallbacks(&event)
					c.Expect(int(ircOutput.numRetries[ircChan]), gs.Equals, 0)
				})

				close(inChan)
				wg.Wait()
			})

			c.Specify("when banned from a server logs an error and quits", func() {
				err := ircOutput.Init(config)
				c.Assume(err, gs.IsNil)

				startOutput()

				event := irc.Event{Code: IRC_ERR_YOUREBANNEDCREEP}
				outTestHelper.MockOutputRunner.EXPECT().LogError(ErrBannedFromServer)
				ircOutput.Conn.RunCallbacks(&event)

				// We should be able to exit without closing the channel first
				wg.Wait()

				// Clean this up
				close(inChan)
			})

			c.Expect(<-errChan, gs.IsNil)
			c.Expect(<-mockIrcConn.connected, gs.IsFalse)

		})

		// Cleanup which should happen each run
		close(mockIrcConn.connected)
		close(mockIrcConn.delivered)
		close(mockIrcConn.Error)
	})

}
