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
	"strings"
	"sync"
	"time"

	"github.com/fsouza/go-dockerclient"
)

type AttachEvent struct {
	Type string
	ID   string
	Name string
}

type Log struct {
	ID   string
	Name string
	Type string
	Data string
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
	attached map[string]*LogPump
	channels map[chan *AttachEvent]struct{}
	client   *docker.Client
	errors   chan<- error
}

func NewAttachManager(client *docker.Client, attachErrors chan<- error) (*AttachManager, error) {
	m := &AttachManager{
		attached: make(map[string]*LogPump),
		channels: make(map[chan *AttachEvent]struct{}),
		client:   client,
		errors:   attachErrors,
	}

	// Attach to all currently running containers
	if containers, err := client.ListContainers(docker.ListContainersOptions{}); err == nil {
		for _, listing := range containers {
			m.attach(listing.ID[:12])
		}
	} else {
		return nil, err
	}

	go func() {
		events := make(chan *docker.APIEvents)
		if err := client.AddEventListener(events); err != nil {
			m.errors <- err
		}
		for msg := range events {
			if msg.Status == "start" {
				go m.attach(msg.ID[:12])
			}
		}
		m.errors <- fmt.Errorf("Docker event channel is closed")
	}()

	return m, nil
}

func (m *AttachManager) attach(id string) {
	container, err := m.client.InspectContainer(id)
	if err != nil {
		m.errors <- err
	}
	name := container.Name[1:]
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
		m.attached[id] = NewLogPump(outrd, errrd, id, name)
		m.Unlock()
		success <- struct{}{}
		m.send(&AttachEvent{ID: id, Name: name, Type: "attach"})
		return
	}
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

func (m *AttachManager) ClientPinger(stopChan chan<- error, t *time.Ticker) {
	for _ = range t.C {
		if len(m.attached) > 0 {
			continue
		}

		if err := m.client.Ping(); err != nil {
			stopChan <- err
		}
	}
}

func (m *AttachManager) Listen(logstream chan *Log, closer <-chan bool) {
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
	ID       string
	Name     string
	channels map[chan *Log]struct{}
}

func NewLogPump(stdout, stderr io.Reader, id, name string) *LogPump {
	obj := &LogPump{
		ID:       id,
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
				Data: strings.TrimSuffix(string(data), "\n"),
				ID:   id,
				Name: name,
				Type: typ,
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
