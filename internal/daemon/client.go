package daemon

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"time"

	"github.com/YCistak/pylon/internal/ipc"
)

// defaultTimeout is generous because a "say" may trigger an LLM round-trip on
// the daemon side, which can take many seconds.
const defaultTimeout = 30 * time.Second

// Send dials the daemon socket, sends one request, and returns its response.
// It is used by the CLI and by the daemon's own liveness probe.
func Send(socketPath string, req ipc.Request) (ipc.Response, error) {
	return SendTimeout(socketPath, req, defaultTimeout)
}

// SendTimeout is Send with an explicit deadline, for the few commands that ask
// the daemon to do something open-ended — reading a briefing aloud takes as
// long as the briefing is, and giving up mid-sentence would leave the daemon
// talking to an empty socket.
func SendTimeout(socketPath string, req ipc.Request, timeout time.Duration) (ipc.Response, error) {
	// Who owns the socket, before a word is sent down it. The daemon's own
	// protections are no help if the thing answering is not the daemon, and
	// what goes down here includes `secret set <name> <api-key>` in plaintext.
	if !ownedByUs(socketPath) {
		return ipc.Response{}, fmt.Errorf(
			"refusing to talk to %s: it belongs to another user", socketPath)
	}
	conn, err := net.DialTimeout("unix", socketPath, 2*time.Second)
	if err != nil {
		return ipc.Response{}, fmt.Errorf("connect to daemon: %w", err)
	}
	defer conn.Close()

	b, err := json.Marshal(req)
	if err != nil {
		return ipc.Response{}, err
	}
	_ = conn.SetDeadline(time.Now().Add(timeout))
	if _, err := conn.Write(append(b, '\n')); err != nil {
		return ipc.Response{}, fmt.Errorf("send request: %w", err)
	}

	scanner := bufio.NewScanner(conn)
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return ipc.Response{}, fmt.Errorf("read response: %w", err)
		}
		return ipc.Response{}, fmt.Errorf("no response from daemon")
	}
	var resp ipc.Response
	if err := json.Unmarshal(scanner.Bytes(), &resp); err != nil {
		return ipc.Response{}, fmt.Errorf("decode response: %w", err)
	}
	return resp, nil
}
