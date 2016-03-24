package docker

// Originally based on Logspout (https://github.com/progrium/logspout)
// Copyright (C) 2014 Jeff Lindsay
//
// Significantly modified by Karl Matthias (karl.matthias@gonitro.com) and Rob
// Miller (rmiller@mozilla.com)
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
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsouza/go-dockerclient"
	"github.com/mozilla-services/heka/message"
	. "github.com/mozilla-services/heka/pipeline"
	"github.com/pborman/uuid"
)

const (
	SLEEP_BETWEEN_RECONNECT = 500 * time.Millisecond
)

type sinces struct {
	Since      int64
	Containers map[string]int64
}

type AttachManager struct {
	hostname         string
	client           DockerClient
	events           chan *docker.APIEvents
	ir               InputRunner
	endpoint         string
	certPath         string
	nameFromEnv      string
	fieldsFromEnv    []string
	fieldsFromLabels []string
	sincePath        string
	sinces           *sinces
	sinceLock        sync.Mutex
	sinceInterval    time.Duration
}

// Return a properly configured Docker client.
func newDockerClient(certPath string, endpoint string) (DockerClient, error) {
	var client DockerClient
	var err error

	if certPath == "" {
		client, err = docker.NewClient(endpoint)
	} else {
		key := filepath.Join(certPath, "key.pem")
		ca := filepath.Join(certPath, "ca.pem")
		cert := filepath.Join(certPath, "cert.pem")
		client, err = docker.NewTLSClient(endpoint, cert, key, ca)
	}

	return client, err
}

// Construct an AttachManager and set up the Docker Client
func NewAttachManager(endpoint string, certPath string, nameFromEnv string,
	fieldsFromEnv []string, fieldsFromLabels []string,
	sincePath string, sinceInterval time.Duration) (*AttachManager, error) {

	client, err := newDockerClient(certPath, endpoint)
	if err != nil {
		return nil, err
	}

	m := &AttachManager{
		client:           client,
		events:           make(chan *docker.APIEvents),
		nameFromEnv:      nameFromEnv,
		fieldsFromEnv:    fieldsFromEnv,
		fieldsFromLabels: fieldsFromLabels,
		sincePath:        sincePath,
		sinces:           &sinces{},
		sinceInterval:    sinceInterval,
	}

	// Initialize the sinces from the JSON since file.
	sinceFile, err := os.Open(sincePath)
	if err != nil {
		return nil, fmt.Errorf("Can't open \"since\" file '%s': %s", sincePath, err.Error())
	}
	jsonDecoder := json.NewDecoder(sinceFile)
	m.sinceLock.Lock()
	err = jsonDecoder.Decode(m.sinces)
	m.sinceLock.Unlock()
	if err != nil {
		return nil, fmt.Errorf("Can't decode \"since\" file '%s': %s", sincePath, err.Error())
	}
	return m, nil
}

// Handler to wrap functions with retry logic.
func withRetries(doWork func() error) error {
	var err error

	retrier, err := NewRetryHelper(RetryOptions{
		MaxRetries: 10,
		Delay:      "1s",
		MaxJitter:  "250ms",
	})
	if err != nil {
		return err
	}

	for {
		err = doWork()
		if err == nil {
			return nil
		}

		// Sleep between retries, break if we're done.
		if e := retrier.Wait(); e != nil {
			break
		}
	}

	return err
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
	m.hostname = hostname

	// Retry this sleeping in between tries During this time
	// the Docker client should get reconnected if there is a
	// general connection issue.
	err := withRetries(m.attachAll)
	if err != nil {
		m.ir.LogError(err)
		return errors.New(
			"Failed to attach to Docker containers after retrying. Plugin giving up.")
	}

	err = withRetries(func() error { return m.client.AddEventListener(m.events) })
	if err != nil {
		m.ir.LogError(err)
		return errors.New(
			"Failed to add Docker event listener after retrying. Plugin giving up.")
	}

	if m.sinceInterval > 0 {
		go m.sinceWriteLoop(stopChan)
	}
	m.handleDockerEvents(stopChan) // Blocks until stopChan is closed.
	// Write to since file on the way out.
	m.writeSinceFile(time.Now())
	return nil
}

func (m *AttachManager) writeSinceFile(t time.Time) {
	sinceFile, err := os.Create(m.sincePath)
	if err != nil {
		m.ir.LogError(fmt.Errorf("Can't create \"since\" file '%s': %s", m.sincePath,
			err.Error()))
		return
	}
	jsonEncoder := json.NewEncoder(sinceFile)
	m.sinceLock.Lock()
	m.sinces.Since = t.Unix()
	if err = jsonEncoder.Encode(m.sinces); err != nil {
		m.ir.LogError(fmt.Errorf("Can't write to \"since\" file '%s': %s", m.sincePath,
			err.Error()))
	}
	m.sinceLock.Unlock()
	if err = sinceFile.Close(); err != nil {
		m.ir.LogError(fmt.Errorf("Can't close \"since\" file '%s': %s", m.sincePath,
			err.Error()))
	}
}

// Periodically writes out a new since file, until stopped.
func (m *AttachManager) sinceWriteLoop(stopChan chan error) {
	ticker := time.Tick(m.sinceInterval)
	ok := true
	var now time.Time
	for ok {
		select {
		case now, ok = <-ticker:
			if !ok {
				break
			}
			m.writeSinceFile(now)
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

// Inspect the container and extract the env vars/labels we were told to keep
func (m *AttachManager) extractFields(id string, client DockerClient) (map[string]string, error) {

	container, err := client.InspectContainer(id)
	if err != nil {
		return nil, err
	}
	name := container.Name[1:] // Strip the leading slas
	image := container.Config.Image

	fields := m.getEnvVars(container, append(m.fieldsFromEnv, m.nameFromEnv))
	if m.nameFromEnv != "" {
		if alt_name, ok := fields[m.nameFromEnv]; ok && alt_name != "" {
			name = alt_name
		}
	}
	fields["ContainerID"] = id
	fields["ContainerName"] = name
	fields["ContainerImage"] = image

	// NOTE! Anything that is a duplicate key will be overridden with the value
	// that is in the label, not the environment
	for _, key := range m.fieldsFromLabels {
		if value, ok := container.Config.Labels[key]; ok {
			fields[key] = value
		}
	}

	return fields, nil
}

// Attach to the log output of a single running container.
func (m *AttachManager) attach(id string, client DockerClient) error {
	m.ir.LogMessage(fmt.Sprintf("Attaching container: %s", id))

	fields, err := m.extractFields(id, client)
	if err != nil {
		return err
	}

	outrd, outwr := io.Pipe()
	errrd, errwr := io.Pipe()

	// Spin up one of these for each container we're watching.
	go func() {
		m.sinceLock.Lock()
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
		}
		m.sinceLock.Unlock()

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
			Tail:         "all",
			RawTerminal:  false,
		})

		// Once it has exited, close our pipes, remove from the sinces, and (if
		// necessary) log the error.
		outwr.Close()
		errwr.Close()
		m.sinceLock.Lock()
		m.sinces.Containers[id] = time.Now().Unix()
		m.sinceLock.Unlock()
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
func (m *AttachManager) makePackDecorator(logger string, fields map[string]string) func(*PipelinePack) {
	return func(pack *PipelinePack) {
		pack.Message.SetType("DockerLog")
		pack.Message.SetLogger(logger)       // stderr or stdout
		pack.Message.SetHostname(m.hostname) // Use the host's hosntame
		pack.Message.SetTimestamp(time.Now().Unix())
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

// Process the env vars and capture the ones we want
func (m *AttachManager) getEnvVars(container *docker.Container, keys []string) map[string]string {
	vars := make(map[string]string)
	for _, value := range container.Config.Env {
		valueParts := strings.SplitN(value, "=", 2)
		if len(valueParts) == 2 {
			for _, key := range keys {
				if key != "" && valueParts[0] == key {
					vars[valueParts[0]] = valueParts[1]
					break
				}
			}
		}
	}
	return vars
}
