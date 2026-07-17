//go:build !windows

package ipc

// Unix keeps the historical /tmp paths: they are what existing configs, docs
// and running setups already point at, and /tmp is a real directory here.

func platformSocketPath() string { return "/tmp/pylon.sock" }

func platformPIDPath() string { return "/tmp/pylon.pid" }
