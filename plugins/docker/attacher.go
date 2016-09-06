package docker

// Originally based on Logspout (https://github.com/progrium/logspout)
// Copyright (C) 2014 Jeff Lindsay
//
// Significantly modified by Karl Matthias (karl.matthias@gonitro.com), Rob
// Miller (rmiller@mozilla.com) and Guy Templeton (guy.templeton@skyscanner.net)
// Copyright (C) 2016
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.

import (
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/fsouza/go-dockerclient"
	"github.com/mozilla-services/heka/message"
	. "github.com/mozilla-services/heka/pipeline"
	"github.com/pborman/uuid"
)


type AttachManager struct {
	hostname                string
	client                  DockerClient
	events                  chan *docker.APIEvents
	ir                      InputRunner
	endpoint                string
	certPath                string
	nameFromEnv             string
	fieldsFromEnv           []string
	fieldsFromLabels        []string
	sinces                  *SinceTracker
	newContainersReplayLogs bool
}

// Construct an AttachManager and set up the Docker Client
func NewAttachManager(endpoint string, certPath string, nameFromEnv string,
	fieldsFromEnv []string, fieldsFromLabels []string,
	sincePath string, sinceInterval time.Duration, containerExpiryDays int,
	newContainersReplayLogs bool) (*AttachManager, error) {

	client, err := newDockerClient(certPath, endpoint)
	if err != nil {
		return nil, err
	}

	sinceTracker, err := NewSinceTracker(containerExpiryDays, sincePath, sinceInterval)
	if err != nil {
		return nil, err
	}

	m := &AttachManager{
		client:                  client,
		events:                  make(chan *docker.APIEvents),
		nameFromEnv:             nameFromEnv,
		fieldsFromEnv:           fieldsFromEnv,
		fieldsFromLabels:        fieldsFromLabels,
		sinces:			 sinceTracker,
		newContainersReplayLogs: newContainersReplayLogs,
	}

	return m, nil
}

// Attach to all running containers
func (m *AttachManager) attachAll() error {
	containers, err := m.client.ListContainers(docker.ListContainersOptions{})
	if err != nil {
		m.ir.LogError(err)
		return err
	}

	for _, listing := range containers {
		err := withRetries(func() error {
			return m.attach(listing.ID[:12], m.client)
		})

		if err != nil {
			return err
		}
	}

	m.ir.LogMessage("Attached to all containers")

	return nil
}

// Main body of work
func (m *AttachManager) Run(ir InputRunner, hostname string, stopChan chan error) error {
	m.ir = ir
	m.sinces.ir = ir
	m.hostname = hostname

	// Retry this sleeping in between tries During this time
	// the Docker client should get reconnected if there is a
	// general connection issue.
	err := withRetries(m.attachAll)
	if err != nil {
		m.ir.LogError(err)
		return errors.New(
			"Failed to attach to Docker containers after retrying. Plugin giving up.",
		)
	}

	err = withRetries(func() error { return m.client.AddEventListener(m.events) })
	if err != nil {
		m.ir.LogError(err)
		return errors.New(
			"Failed to add Docker event listener after retrying. Plugin giving up.",
		)
	}

	if m.sinces.Interval > 0 {
		go m.sinceWriteLoop(stopChan)
	}
	m.handleDockerEvents(stopChan) // Blocks until stopChan is closed.
	// Write to since file on the way out.
	m.sinces.Write(time.Now())
	return nil
}


// Periodically writes out a new since file, until stopped.
func (m *AttachManager) sinceWriteLoop(stopChan chan error) {
	ticker := time.Tick(m.sinces.Interval)
	ok := true
	var now time.Time
	for ok {
		select {
		case now, ok = <-ticker:
			if !ok {
				break
			}
			m.sinces.Write(now)
		case <-stopChan:
			ok = false
			break
		}
	}
}

// Try to reboot all of our connections.
func (m *AttachManager) restart() error {
	var err error

	m.ir.LogMessage("Restarting Docker connection...")
	m.client.RemoveEventListener(m.events)

	// See if the Docker client closed the channel. We can throw away
	// the read result because we're going to attach to all containers
	// next, anyway. An EOF from the client can close it.
	if _, ok := <-m.events; !ok {
		m.events = make(chan *docker.APIEvents)
	}

	m.client.AddEventListener(m.events)

	err = withRetries(m.attachAll)
	if err != nil {
		return err
	}

	return nil
}

// Watch for Docker events, and trigger an attachment if we see a new
// container.
func (m *AttachManager) handleDockerEvents(stopChan chan error) {
	for {
		select {
		case msg, ok := <-m.events:
			if !ok {
				m.ir.LogMessage("Events channel closed, restarting...")
				err := withRetries(m.restart)
				if err != nil {
					m.ir.LogError(fmt.Errorf("Unable to restart Docker connection! (%s)", err.Error()))
					return // Will cause the plugin to restart
				}
				time.Sleep(SLEEP_BETWEEN_RECONNECT)
				continue
			}

			if msg.Status == "start" {
				go m.attach(msg.ID[:12], m.client)
			}
		case <-stopChan:
			return
		}
	}
}

// Attach to the log output of a single running container.
func (m *AttachManager) attach(id string, client DockerClient) error {
	m.ir.LogMessage(fmt.Sprintf("Attaching container: %s", id))

	fields, err := extractFields(id, client, m.fieldsFromLabels, m.fieldsFromEnv, m.nameFromEnv)
	if err != nil {
		return err
	}

	outrd, outwr := io.Pipe()
	errrd, errwr := io.Pipe()

	// Spin up one of these for each container we're watching.
	go func() {
		m.sinces.Lock()
		since, ok := m.sinces.Containers[id]
		if ok {
			// We've seen this container before, need to use a since value.
			if since == 0 {
				// Fall back to the top level since time.
				since = m.sinces.Since
			} else {
				// Clear out the container specific since time.
				m.sinces.Containers[id] = 0
			}
		} else {
			// We haven't seen it, add it to our sinces.
			m.sinces.Containers[id] = 0

			// And set the since appropriately from our settings.
			if !m.newContainersReplayLogs {
				// Use the last global since time when we connect to it
				since = m.sinces.Since
			}
		}
		m.sinces.Unlock()

		// This will block until the container exits.
		err := client.Logs(docker.LogsOptions{
			Container:    id,
			OutputStream: outwr,
			ErrorStream:  errwr,
			Follow:       true,
			Stdout:       true,
			Stderr:       true,
			Since:        since,
			Timestamps:   false,
			Tail:         "all", // This is scoped by "Since" above
			RawTerminal:  false,
		})

		// Once it has exited, close our pipes, set the since time to now, and
		// (if necessary) log the error.
		outwr.Close()
		errwr.Close()
		m.sinces.Lock()
		m.sinces.Containers[id] = time.Now().Unix()
		m.sinces.Unlock()
		if err != nil {
			err = fmt.Errorf("streaming container %s logs: %s", id, err.Error())
			m.ir.LogError(err)
		}
	}()

	// Wait for success from the attachment
	go m.handleOneStream("stdout", outrd, fields, id)
	go m.handleOneStream("stderr", errrd, fields, id)
	return nil
}

// Add our fields to the output pack
func (m *AttachManager) makePackDecorator(logger string,
	fields map[string]string) func(*PipelinePack) {

	return func(pack *PipelinePack) {
		pack.Message.SetType("DockerLog")
		pack.Message.SetLogger(logger)       // stderr or stdout
		pack.Message.SetHostname(m.hostname) // Use the host's hosntame
		pack.Message.SetTimestamp(time.Now().UnixNano())
		pack.Message.SetUuid(uuid.NewRandom())

		for name, value := range fields {
			field, err := message.NewField(name, value, "")
			if err != nil {
				m.ir.LogError(
					fmt.Errorf("can't add '%s' field: %s", name, err.Error()),
				)
				continue
			}

			pack.Message.AddField(field)
		}
	}
}

// Sets up the Heka pipeline for a single IO stream (either stdout or stderr)
func (m *AttachManager) handleOneStream(name string, in io.Reader, fields map[string]string,
	containerId string) {

	id := fmt.Sprintf("%s-%s", fields["ContainerName"], name)

	sRunner := m.ir.NewSplitterRunner(id)
	if !sRunner.UseMsgBytes() {
		sRunner.SetPackDecorator(m.makePackDecorator(name, fields))
	}

	deliverer := m.ir.NewDeliverer(id)

	var err error
	for err == nil {
		err = sRunner.SplitStream(in, deliverer)
		if err != io.EOF && err != nil {
			m.ir.LogError(fmt.Errorf("Error reading %s stream: %s", name, err.Error()))
		}
	}
	sRunner.Done()

	m.ir.LogMessage(fmt.Sprintf("Disconnecting %s stream from %s", name, containerId))
}
