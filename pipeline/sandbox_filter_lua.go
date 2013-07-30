// +build !nolua

package pipeline

import (
	"github.com/mozilla-services/heka/sandbox"
	"github.com/mozilla-services/heka/sandbox/lua"
)

func CreateLuaSandbox(sbc *sandbox.SandboxConfig) (sandbox.Sandbox, error) {
	return lua.CreateLuaSandbox(this.sbc)
}
