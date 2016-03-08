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

import(
	"strings"
	"time"
	"path/filepath"
	. "github.com/mozilla-services/heka/pipeline"
	"github.com/fsouza/go-dockerclient"
)


const (
	SLEEP_BETWEEN_RECONNECT = 500 * time.Millisecond
	EVENT_FORMAT_STRING = "Event.ID : %s , event.Type : %s , event.Name : %s"
)

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


// Handler to wrap functions with retry logic
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

		// Sleep between retries, break if we're done
		if e := retrier.Wait(); e != nil {
			break
		}
	}

	return err
}

// Inspect the container and extract the env vars/labels we were told to keep
func extractFields(id string, client DockerClient, fieldsFromLabels []string, fieldsFromEnv []string, nameFromEnv string) (map[string]string, error) {

	container, err := client.InspectContainer(id)
	if err != nil {
		return nil, err
	}
	name := container.Name[1:] // Strip the leading slas
	image := container.Config.Image

	fields := getEnvVars(container, append(fieldsFromEnv, nameFromEnv))
	if nameFromEnv != "" {
		if alt_name, ok := fields[nameFromEnv]; ok && alt_name != "" {
			name = alt_name
		}
	}
	fields["ContainerID"] = id
	fields["ContainerName"] = name
	fields["ContainerImage"] = image

	// NOTE! Anything that is a duplicate key will be overridden with the value
	// that is in the label, not the environment
	for _, key := range fieldsFromLabels {
		if value, ok := container.Config.Labels[key]; ok {
			fields[key] = value
		}
	}

	return fields, nil
}

// Process the env vars and capture the ones we want
func getEnvVars(container *docker.Container, keys []string) map[string]string {
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
