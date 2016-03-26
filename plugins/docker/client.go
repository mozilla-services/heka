package docker

import "github.com/fsouza/go-dockerclient"

type DockerClient interface {
	// AddEventListener adds a new listener to container events in the Docker
	// API.
	//
	// The parameter is a channel through which events will be sent.
	AddEventListener(listener chan<- *docker.APIEvents) error

	// RemoveEventListener removes a listener from the monitor.
	RemoveEventListener(listener chan *docker.APIEvents) error

	// ListContainersOptions specify parameters to the ListContainers function.
	//
	// See http://goo.gl/XqtcyU for more details.
	ListContainers(opts docker.ListContainersOptions) ([]docker.APIContainers, error)

	// InspectContainer returns information about a container by its ID.
	//
	// See http://goo.gl/CxVuJ5 for more details.
	InspectContainer(id string) (*docker.Container, error)

	// AttachToContainer attaches to a container, using the given options.
	//
	// See http://goo.gl/RRAhws for more details.
	AttachToContainer(opts docker.AttachToContainerOptions) error

	// StreamContainerStats attaches to a container for stats streaming
	//
	// See https://goo.gl/PKOSSE for more details.
	Stats(opts docker.StatsOptions) error

	// Logs gets stdout and stderr log from the specified container.
	//
	// See http://goo.gl/yl8PGm for more details.
	Logs(opts docker.LogsOptions) error

	// Ping pings the docker server
	//
	// See https://goo.gl/kQCfJj for more details.
	Ping() error
}
