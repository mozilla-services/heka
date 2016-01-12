package docker

// Based on Logspout (https://github.com/progrium/logspout)
//
// Copyright (C) 2014 Jeff Lindsay
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
	"bufio"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsouza/go-dockerclient"
	"github.com/mozilla-services/heka/pipeline"
)

const (
	CONNECT_RETRIES = 4
)

var (
	retryInterval = []time.Duration{1, 2, 3, 7}
)

type AttachEvent struct {
	Type string
	ID   string
	Name string
}

type Log struct {
	Type   string
	Data   string
	Fields map[string]string
}

type AttachManager struct {
	sync.RWMutex
	attached      map[string]*LogPump
	channels      map[chan *AttachEvent]struct{}
	client        DockerClient
	events        chan *docker.APIEvents
	eventReset    chan struct{}
	sentinel      struct{}
	ir            pipeline.InputRunner
	endpoint      string
	certPath      string
	nameFromEnv   string
	fieldsFromEnv []string
}

// Return a properly configured Docker client
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
	fieldsFromEnv []string) (*AttachManager, error) {

	client, err := newDockerClient(certPath, endpoint)
	if err != nil {
		return nil, err
	}

	m := &AttachManager{
		attached:      make(map[string]*LogPump),
		channels:      make(map[chan *AttachEvent]struct{}),
		client:        client,
		events:        make(chan *docker.APIEvents),
		nameFromEnv:   nameFromEnv,
		fieldsFromEnv: fieldsFromEnv,
	}

	return m, nil
}

// Handler to wrap functions with retry logic
func withRetries(doWork func() error) error {
	var err error
	for i := 0; i < CONNECT_RETRIES; i++ {
		doWork()
		if err == nil {
			return nil
		}
		time.Sleep(retryInterval[i] * time.Second)
	}

	return err
}

// Attach to all running containers
func (m *AttachManager) attachAll() error {
	// If we always use a new client here, we don't have to
	// worry about a stale connection. This only gets called
	// on Run() and restart(), so shouldn't be a burden.
	client, err := newDockerClient(m.certPath, m.endpoint)
	if err != nil {
		return err
	}

	containers, err := client.ListContainers(docker.ListContainersOptions{})
	if err != nil {
		return err
	}

	for _, listing := range containers {
		err := withRetries(func() error {
			m.ir.LogMessage(fmt.Sprintf("Attaching container: %s", listing.ID[:12]))
			return m.attach(listing.ID[:12], client)
		})

		if err != nil {
			return err
		}
	}

	return nil
}

// Acutally do the work of attaching and starting up LogPumps
func (m *AttachManager) Run(ir pipeline.InputRunner) {
	m.ir = ir

	// Retry this up to CONNECT_RETRIES number of times, sleeping
	// the defined interval. During this time the Docker client should
	// get reconnected if there is a general connection issue.
	err := withRetries(m.attachAll)
	if err != nil {
		m.ir.LogError(
			fmt.Errorf("Failed to attach to Docker containers after %s retries. Plugin giving up.", CONNECT_RETRIES),
		)
		return
	}

	m.ir.LogMessage("Attached to all containers")

	err = withRetries(func() error { return m.client.AddEventListener(m.events) })
	if err != nil {
		m.ir.LogError(
			fmt.Errorf("Failed to add Docker event listener after %s retries. Plugin giving up.", CONNECT_RETRIES),
		)
		return
	}

	go m.recvDockerEvents()
}

// Try tro reboot all of our connections
func (m *AttachManager) restart() {
	var err error

	m.ir.LogMessage("Lost connection to Docker, re-connecting")
	m.client.RemoveEventListener(m.events)
	m.events = make(chan *docker.APIEvents) // RemoveEventListener closes it

	m.client.AddEventListener(m.events)

	// Remove all the currently attached entries from the attached map
	m.Lock()
	for id, _ := range m.attached {
		delete(m.attached, id)
	}
	m.Unlock()

	err = withRetries(m.attachAll)
	if err != nil {
		m.ir.LogError(fmt.Errorf("Unable to attach to call containers!"))
	}
}

// Watch for Docker events, and trigger an attachment if we see a new
// container.
func (m *AttachManager) recvDockerEvents() {
	for {
		for msg := range m.events {
			if msg.Status == "start" {
				go m.attach(msg.ID[:12], m.client)
			}
		}
		m.ir.LogMessage("Events channel closed, restarting...")
		m.restart()
		time.Sleep(500 * time.Millisecond)
	}
}

func (m *AttachManager) attach(id string, client DockerClient) error {
	container, err := client.InspectContainer(id)
	if err != nil {
		return err
	}
	name := container.Name[1:]

	fields := m.getEnvVars(container, append(m.fieldsFromEnv, m.nameFromEnv))
	if m.nameFromEnv != "" {
		if alt_name, ok := fields[m.nameFromEnv]; ok && alt_name != "" {
			name = alt_name
		}
	}
	fields["ContainerID"] = id
	fields["ContainerName"] = name

	success := make(chan struct{})
	failure := make(chan error)
	outrd, outwr := io.Pipe()
	errrd, errwr := io.Pipe()

	// Spin up one of these for each container we're watching
	go func() {
		// This will block until the container exits
		err := client.AttachToContainer(docker.AttachToContainerOptions{
			Container:    id,
			OutputStream: outwr,
			ErrorStream:  errwr,
			Stdin:        false,
			Stdout:       true,
			Stderr:       true,
			Stream:       true,
			Success:      success,
		})

		// Once it has exited, close our pipes
		outwr.Close()
		errwr.Close()
		if err != nil {
			close(success)
			failure <- err
		}

		m.Lock()
		// Remove from the attached map
		delete(m.attached, id)
		m.Unlock()
	}()

	// Wait for success from the attachment
	_, ok := <-success
	if ok {
		// Attach a LogPump to the pipes
		m.Lock()
		m.attached[id] = NewLogPump(outrd, errrd, name, fields)
		m.Unlock()

		// Signal back to the client to continue with attachment
		success <- struct{}{}
		m.send(&AttachEvent{ID: id, Name: name, Type: "attach"})
		return nil
	}

	return nil
}

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

func (m *AttachManager) changeNameByEnv(container *docker.Container, envName string) string {
	for _, value := range container.Config.Env {
		valueParts := strings.SplitN(value, "=", 2)
		if len(valueParts) == 2 && valueParts[0] == envName {
			return valueParts[1]
		}
	}
	return ""
}

func (m *AttachManager) send(event *AttachEvent) {
	m.RLock()
	defer m.RUnlock()
	for ch, _ := range m.channels {
		// TODO: log err after timeout and continue
		ch <- event
	}
}

func (m *AttachManager) addListener(ch chan *AttachEvent) {
	m.Lock()
	defer m.Unlock()
	m.channels[ch] = struct{}{}
	go func() {
		for id, pump := range m.attached {
			ch <- &AttachEvent{ID: id, Name: pump.Name, Type: "attach"}
		}
	}()
}

func (m *AttachManager) removeListener(ch chan *AttachEvent) {
	m.Lock()
	defer m.Unlock()
	delete(m.channels, ch)
}

func (m *AttachManager) GetAttached(id string) *LogPump {
	m.Lock()
	defer m.Unlock()
	return m.attached[id]
}

func (m *AttachManager) Listen(logstream chan *Log, closer <-chan struct{}) {
	events := make(chan *AttachEvent)
	m.addListener(events)
	defer m.removeListener(events)

	for {
		select {
		case event := <-events:
			if event.Type == "attach" {
				pump := m.GetAttached(event.ID)
				pump.AddListener(logstream)
				defer func() {
					if pump != nil {
						pump.RemoveListener(logstream)
					}
				}()
			}
		case <-closer:
			return
		}
	}
}

// A data pump that pulls data in on an IO and sends structured
// Logs out on listening channels.
type LogPump struct {
	sync.RWMutex
	Name      string
	listeners map[chan *Log]struct{}
}

// Return a fully configured LogPump, and begin pumping
func NewLogPump(stdout, stderr io.Reader, name string, fields map[string]string) *LogPump {
	logPump := &LogPump{
		Name:      name,
		listeners: make(map[chan *Log]struct{}),
	}

	go logPump.Pump(stdout, fields)
	go logPump.Pump(stderr, fields)

	return logPump
}

// Pull data in on the source, separated by a carriage return,
// and send a Log
func (lp *LogPump) Pump(source io.Reader, fields map[string]string) {
	buf := bufio.NewReader(source)
	for {
		data, err := buf.ReadBytes('\n')
		if err != nil {
			return
		}

		lp.send(&Log{
			Data:   strings.TrimSuffix(string(data), "\n"),
			Fields: fields,
		})
	}
}

// Send a log to all listeners
func (o *LogPump) send(log *Log) {
	o.RLock()
	defer o.RUnlock()
	for ch, _ := range o.listeners {
		// TODO: log err after timeout and continue
		ch <- log
	}
}

// Add a listener channel to receive new logs
func (o *LogPump) AddListener(ch chan *Log) {
	o.Lock()
	defer o.Unlock()
	o.listeners[ch] = struct{}{}
}

// Stop listening for Logs
func (o *LogPump) RemoveListener(ch chan *Log) {
	o.Lock()
	defer o.Unlock()
	delete(o.listeners, ch)
}
