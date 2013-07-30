// +build nolua

package pipeline

import (
	"errors"
	"github.com/mozilla-services/heka/sandbox"
)

func CreateLuaSandbox(sbc *sandbox.SandboxConfig) (sandbox.Sandbox, error) {
	return nil, errors.New("no lua is compiled in!")
}
