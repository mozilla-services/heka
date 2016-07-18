package docker

// Built upon  the existing DockerLogInput plugin
// by Guy Templeton (guy.templeton@skyscanner.net)
// Copyright (C) 2016
//
// DockerLogInput is:
// Originally based on Logspout (https://github.com/progrium/logspout)
// Copyright (C) 2014 Jeff Lindsay
//
// Significantly modified by Karl Matthias (karl.matthias@gonitro.com) ,Rob
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
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/fsouza/go-dockerclient"
	. "github.com/mozilla-services/heka/pipeline"
)

type DockerStat struct {
	Container   string
	Time        time.Time
	StatsString string
	Fields      map[string]string
}

type StatsAttachEvent struct {
	Type string
	ID   string
	Name string
}

type Source struct {
	ID     string
	Name   string
	Filter string
	Types  []string
}

type StatsManager struct {
	sync.RWMutex
	ir               InputRunner
	attached         map[string]*StatsPump
	channels         map[chan *StatsAttachEvent]struct{}
	client           DockerClient
	errors           chan<- error
	events           chan *docker.APIEvents
	nameFromEnv      string
	fieldsFromEnv    []string
	fieldsFromLabels []string
}

func NewStatsManager(endpoint, certPath string, attachErrors chan<- error, nameFromEnv string,
	fieldsFromEnv []string, fieldsFromLabels []string) (*StatsManager, error) {

	var client DockerClient
	var err error

	//Get a new DockerClient from attacher
	client, err = newDockerClient(certPath, endpoint)

	if err != nil {
		return nil, err
	}

	m := &StatsManager{
		attached:         make(map[string]*StatsPump),
		channels:         make(map[chan *StatsAttachEvent]struct{}),
		client:           client,
		errors:           attachErrors,
		events:           make(chan *docker.APIEvents),
		nameFromEnv:      nameFromEnv,
		fieldsFromEnv:    fieldsFromEnv,
		fieldsFromLabels: fieldsFromLabels,
	}
	return m, nil
}

//The main initial work happens here
func (m *StatsManager) Run(statsstream chan *DockerStat, closer <-chan struct{},
	stopChan chan error) {

	m.attachAll()

	//Launch routine for listening to events
	go m.handleDockerEvents(stopChan)
	m.ir.LogMessage("Listen ")

	source := new(Source)
	events := make(chan *StatsAttachEvent)
	m.addStatsListener(events)
	defer m.removeStatsListener(events)
	//Add listeners for all currently running containers
	for {
		select {
		case event := <-events:
			m.ir.LogMessage(fmt.Sprintf(EVENT_FORMAT_STRING, event.ID, event.Type, event.Name))

			if event.Type == "attach" && (source.AllContainers() ||
				(source.ID != "" && strings.HasPrefix(event.ID, source.ID)) ||
				(source.Name != "" && event.Name == source.Name) ||
				(source.Filter != "" && strings.Contains(event.Name, source.Filter))) {

				pump := m.Get(event.ID)
				pump.AddStatsListener(statsstream)
				defer func() {
					if pump != nil {
						pump.RemoveStatsListener(statsstream)
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

// Attach to all running containers
func (m *StatsManager) attachAll() error {
	containers, err := m.client.ListContainers(docker.ListContainersOptions{})
	if err != nil {
		m.ir.LogError(err)
		return err
	}

	for _, listing := range containers {
		err := withRetries(func() error {
			return m.statsAttach(listing.ID[:12], m.client)
		})

		if err != nil {
			return err
		}
	}

	m.ir.LogMessage("Stats Attached to all containers")

	return nil
}

// Attach to the output of a single running container
func (m *StatsManager) statsAttach(id string, client DockerClient) error {
	m.ir.LogMessage(fmt.Sprintf("Attaching container: %s", id))

	done := make(chan bool)
	statsrd := make(chan *docker.Stats)
	failure := make(chan error)

	// Spin up one of these for each container we're watching
	go func() {
		// This will block until the container exits
		err := client.Stats(docker.StatsOptions{
			ID:     id,
			Stats:  statsrd,
			Stream: true,
			Done:   done,
		})

		// Once it has exited, close our pipes
		close(done)
		if err != nil {
			failure <- err
		}
	}()

	container, err := m.client.InspectContainer(id)
	if err != nil {
		m.errors <- err
	}
	name := container.Name[1:]
	fields, err := extractFields(id, m.client, m.fieldsFromLabels, m.fieldsFromEnv, m.nameFromEnv)
	if m.nameFromEnv != "" {
		if alt_name, ok := fields[m.nameFromEnv]; ok && alt_name != "" {
			name = alt_name
		}
	}
	fields["ContainerID"] = id
	fields["ContainerName"] = name
	var resultStats []*docker.Stats
	for {
		stats, ok := <-statsrd
		if !ok {
			break
		}
		resultStats = append(resultStats, stats)
		m.Lock()
		m.attached[id] = NewStatsPump(statsrd, name, fields)
		m.Unlock()
		m.send(&StatsAttachEvent{ID: id, Name: name, Type: "attach"})
		return nil
	}
	return nil
}

func (m *StatsManager) send(event *StatsAttachEvent) {
	m.RLock()
	for ch, _ := range m.channels {
		ch <- event
	}
	m.RUnlock()
}

func (m *StatsManager) addStatsListener(ch chan *StatsAttachEvent) {
	m.ir.LogMessage(" addListener ")
	m.Lock()
	defer m.Unlock()
	m.channels[ch] = struct{}{}
	go func() {
		for id, pump := range m.attached {
			m.ir.LogMessage("StatsID:" + id + " ,  Name: " + pump.Name + ", Type:  attach")
			ch <- &StatsAttachEvent{ID: id, Name: pump.Name, Type: "attach"}
		}
	}()
}

func (m *StatsManager) removeStatsListener(ch chan *StatsAttachEvent) {
	m.Lock()
	delete(m.channels, ch)
	m.Unlock()
}

func (m *StatsManager) Get(id string) *StatsPump {
	m.Lock()
	defer m.Unlock()
	return m.attached[id]
}

func (s *Source) AllContainers() bool {
	return s.ID == "" && s.Name == "" && s.Filter == ""
}

// Try to reboot all of our connections
func (m *StatsManager) restart() error {
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
func (m *StatsManager) handleDockerEvents(stopChan chan error) {
	for {
		select {
		case msg, ok := <-m.events:
			if !ok {
				m.ir.LogMessage("Events channel closed, restarting...")
				err := withRetries(m.restart)
				if err != nil {
					m.ir.LogError(fmt.Errorf("Unable to restart Docker connection! (%s)",
						err.Error()))
					return // Will cause the plugin to restart
				}
				time.Sleep(SLEEP_BETWEEN_RECONNECT)
				continue
			}

			if msg.Status == "start" {
				go m.statsAttach(msg.ID[:12], m.client)
			}
		case <-stopChan:
			return
		}
	}
}

// StatsPump struct. This handles receiving Dockerstat structs from
// the go-dockerclient and pumping them out into the channel
type StatsPump struct {
	sync.RWMutex
	Name     string
	channels map[chan *DockerStat]struct{}
}

func NewStatsPump(statsChan chan *docker.Stats, name string, fields map[string]string) *StatsPump {

	obj := &StatsPump{
		Name:     name,
		channels: make(map[chan *DockerStat]struct{}),
	}
	// Does the work of actually pumping out Stats structs coming in
	// from the channel
	pump := func(sourceChan chan *docker.Stats, fields map[string]string) {
		for {
			source, ok := <-sourceChan
			if !ok{
				return
			}
			json_ver, _ := json.Marshal(source)
			// Send a DockerStat struct out
			obj.send(&DockerStat{
				Container:   name,
				Time:        source.Read,
				StatsString: string(json_ver),
				Fields:      fields,
			})
		}
	}
	go pump(statsChan, fields)
	return obj
}

func (o *StatsPump) send(stats *DockerStat) {
	o.RLock()
	defer o.RUnlock()
	for ch, _ := range o.channels {
		ch <- stats
	}
}

func (o *StatsPump) AddStatsListener(ch chan *DockerStat) {
	o.Lock()
	defer o.Unlock()
	o.channels[ch] = struct{}{}
}

func (o *StatsPump) RemoveStatsListener(ch chan *DockerStat) {
	o.Lock()
	defer o.Unlock()
	delete(o.channels, ch)
}
