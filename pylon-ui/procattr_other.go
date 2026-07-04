//go:build !linux

package main

import "os/exec"

// killOnParentDeath is a no-op on platforms without a parent-death signal;
// orphan cleanup there relies on the graceful stop() during shutdown.
func killOnParentDeath(cmd *exec.Cmd) {}
