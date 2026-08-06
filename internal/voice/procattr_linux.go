//go:build linux

package voice

import (
	"os/exec"
	"syscall"
)

// killOnParentDeath makes the STT server receive SIGTERM if the daemon dies
// unexpectedly (crash / kill -9), so a dead daemon never leaves the model
// resident in GPU memory. The normal shutdown path still stops it with SIGINT.
func killOnParentDeath(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Pdeathsig = syscall.SIGTERM
}
