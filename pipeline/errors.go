package pipeline

import (
	"fmt"
)

type TerminatedError string

func (e TerminatedError) Error() string {
	return fmt.Sprintf("Terminated. Reason: %v", string(e))
}
