package docker

import "github.com/carlanton/go-dockerclient"

type DockerClient interface {
	// AddEventListener adds a new listener to container events in the Docker
	// API.
	//
	// The parameter is a channel through which events will be sent.
	AddEventListener(listener chan<- *docker.APIEvents) error

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
}
