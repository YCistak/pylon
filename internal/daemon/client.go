package daemon

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"time"

	"github.com/YCistak/pylon/internal/ipc"
)

// Send dials the daemon socket, sends one request, and returns its response.
// It is used by the CLI and by the daemon's own liveness probe.
func Send(socketPath string, req ipc.Request) (ipc.Response, error) {
	conn, err := net.DialTimeout("unix", socketPath, 2*time.Second)
	if err != nil {
		return ipc.Response{}, fmt.Errorf("connect to daemon: %w", err)
	}
	defer conn.Close()

	b, err := json.Marshal(req)
	if err != nil {
		return ipc.Response{}, err
	}
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
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
