//go:build linux

package main

import (
	"os/exec"
	"syscall"
)

// killOnParentDeath makes the spawned daemon receive SIGTERM if the GUI process
// dies unexpectedly (crash / kill -9), so a dead UI never leaves an orphaned
// daemon behind. The normal quit path still stops it gracefully via stop().
func killOnParentDeath(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Pdeathsig = syscall.SIGTERM
}
