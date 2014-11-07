package pipeline

import (
	"fmt"
)

type TerminatedError string

func (e TerminatedError) Error() string {
	return fmt.Sprintf("Terminated. Reason: %v", string(e))
}

type MultiError []error

func (e MultiError) Error() string {
	str := ""
	for _, err := range e {
		str += err.Error() + "\n"
	}
	return str
}
