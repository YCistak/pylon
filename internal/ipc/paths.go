package ipc

import "os"

// PYLON_SOCKET / PYLON_PID override the platform defaults. The GUI is a
// separate module and cannot import this package (internal/), so it carries its
// own copy of the default paths; the env vars are the one lever that moves both
// sides at once, which is what makes a non-standard location workable.
const (
	socketEnv = "PYLON_SOCKET"
	pidEnv    = "PYLON_PID"
)

// DefaultSocketPath is where the daemon listens unless overridden via config.
func DefaultSocketPath() string {
	if p := os.Getenv(socketEnv); p != "" {
		return p
	}
	return platformSocketPath()
}

// DefaultPIDPath is where the daemon records its PID unless overridden.
func DefaultPIDPath() string {
	if p := os.Getenv(pidEnv); p != "" {
		return p
	}
	return platformPIDPath()
}
