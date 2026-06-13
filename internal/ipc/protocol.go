// Package ipc defines the wire protocol shared between the Pylon CLI and the
// running daemon. Messages are newline-delimited JSON over a Unix socket.
package ipc

// DefaultSocketPath is where the daemon listens unless overridden via config.
const DefaultSocketPath = "/tmp/pylon.sock"

// Request is a command sent from the CLI to the daemon.
type Request struct {
	Cmd  string   `json:"cmd"`            // e.g. "status", "ping"
	Args []string `json:"args,omitempty"` // optional positional args
}

// Response is the daemon's reply to a Request.
type Response struct {
	OK    bool   `json:"ok"`
	Text  string `json:"text,omitempty"`  // human-readable result
	Error string `json:"error,omitempty"` // set when OK is false
}
