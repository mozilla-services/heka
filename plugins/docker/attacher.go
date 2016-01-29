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
	"io"
	"path/filepath"
	"strings"
	"sync"

	"github.com/carlanton/go-dockerclient"
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

type Source struct {
	ID     string
	Name   string
	Filter string
	Types  []string
}

func (s *Source) All() bool {
	return s.ID == "" && s.Name == "" && s.Filter == ""
}

type AttachManager struct {
	sync.RWMutex
	attached      map[string]*LogPump
	channels      map[chan *AttachEvent]struct{}
	client        DockerClient
	errors        chan<- error
	events        chan *docker.APIEvents
	eventReset    chan struct{}
	sentinel      struct{}
	nameFromEnv   string
	fieldsFromEnv []string
}

func NewAttachManager(endpoint, certPath string, attachErrors chan<- error, nameFromEnv string, fieldsFromEnv []string) (*AttachManager, error) {
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

	if err != nil {
		return nil, err
	}

	m := &AttachManager{
		attached:      make(map[string]*LogPump),
		channels:      make(map[chan *AttachEvent]struct{}),
		client:        client,
		errors:        attachErrors,
		events:        make(chan *docker.APIEvents),
		nameFromEnv:   nameFromEnv,
		fieldsFromEnv: fieldsFromEnv,
	}

	// Attach to all currently running containers
	if containers, err := client.ListContainers(docker.ListContainersOptions{}); err == nil {
		for _, listing := range containers {
			m.attach(listing.ID[:12])
		}
	} else {
		return nil, err
	}

	if err := client.AddEventListener(m.events); err != nil {
		return nil, err
	}

	go m.recvDockerEvents()
	return m, nil
}

func (m *AttachManager) recvDockerEvents() {
	for msg := range m.events {
		if msg.Status == "start" {
			go m.attach(msg.ID[:12])
		}
	}
	close(m.errors)
}

func (m *AttachManager) attach(id string) {
	container, err := m.client.InspectContainer(id)
	if err != nil {
		m.errors <- err
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
	go func() {
		err := m.client.AttachToContainer(docker.AttachToContainerOptions{
			Container:    id,
			OutputStream: outwr,
			ErrorStream:  errwr,
			Stdin:        false,
			Stdout:       true,
			Stderr:       true,
			Stream:       true,
			Success:      success,
		})
		outwr.Close()
		errwr.Close()
		if err != nil {
			close(success)
			failure <- err
		}

		m.send(&AttachEvent{Type: "detach", ID: id, Name: name})
		m.Lock()
		delete(m.attached, id)
		m.Unlock()
	}()
	_, ok := <-success
	if ok {
		m.Lock()
		m.attached[id] = NewLogPump(outrd, errrd, name, fields)
		m.Unlock()
		success <- struct{}{}
		m.send(&AttachEvent{ID: id, Name: name, Type: "attach"})
		return
	}
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

func (m *AttachManager) Get(id string) *LogPump {
	m.Lock()
	defer m.Unlock()
	return m.attached[id]
}

func (m *AttachManager) Listen(logstream chan *Log, closer <-chan struct{}) {
	source := new(Source) // TODO: Make the source parameters configurable
	events := make(chan *AttachEvent)
	m.addListener(events)
	defer m.removeListener(events)

	for {
		select {
		case event := <-events:
			if event.Type == "attach" && (source.All() ||
				(source.ID != "" && strings.HasPrefix(event.ID, source.ID)) ||
				(source.Name != "" && event.Name == source.Name) ||
				(source.Filter != "" && strings.Contains(event.Name, source.Filter))) {

				pump := m.Get(event.ID)
				pump.AddListener(logstream)
				defer func() {
					if pump != nil {
						pump.RemoveListener(logstream)
					}
				}()
			} else if source.ID != "" && event.Type == "detach" &&
				strings.HasPrefix(event.ID, source.ID) {
				return
			}
		case <-closer:
			return
		}
	}
}

type LogPump struct {
	sync.RWMutex
	Name     string
	channels map[chan *Log]struct{}
}

func NewLogPump(stdout, stderr io.Reader, name string, fields map[string]string) *LogPump {
	obj := &LogPump{
		Name:     name,
		channels: make(map[chan *Log]struct{}),
	}
	pump := func(typ string, source io.Reader) {
		buf := bufio.NewReader(source)
		for {
			data, err := buf.ReadBytes('\n')
			if err != nil {
				return
			}
			obj.send(&Log{
				Data:   strings.TrimSuffix(string(data), "\n"),
				Type:   typ,
				Fields: fields,
			})
		}
	}
	go pump("stdout", stdout)
	go pump("stderr", stderr)
	return obj
}

func (o *LogPump) send(log *Log) {
	o.RLock()
	defer o.RUnlock()
	for ch, _ := range o.channels {
		// TODO: log err after timeout and continue
		ch <- log
	}
}

func (o *LogPump) AddListener(ch chan *Log) {
	o.Lock()
	defer o.Unlock()
	o.channels[ch] = struct{}{}
}

func (o *LogPump) RemoveListener(ch chan *Log) {
	o.Lock()
	defer o.Unlock()
	delete(o.channels, ch)
}
