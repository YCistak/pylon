//go:build !linux

package voice

import "os/exec"

// killOnParentDeath is a no-op outside Linux: there is no portable equivalent of
// Pdeathsig. exec.CommandContext still stops the server on normal shutdown.
func killOnParentDeath(*exec.Cmd) {}
