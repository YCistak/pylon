package daemon

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/YCistak/pylon/internal/ipc"
)

// ErrAlreadyRunning is returned by Run when another live daemon owns the PID file.
var ErrAlreadyRunning = errors.New("pylon daemon already running")

// ensureDirs creates the parent directory of each path. Paths sharing a parent
// (the usual case) collapse into one harmless repeat MkdirAll.
func ensureDirs(paths ...string) error {
	for _, p := range paths {
		dir := filepath.Dir(p)
		if dir == "" || dir == "." {
			continue
		}
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("create %s: %w", dir, err)
		}
	}
	return nil
}

// writePIDFile records the current PID, refusing if a live daemon already owns it.
func (d *Daemon) writePIDFile() error {
	if pid, ok := d.livePID(); ok {
		return fmt.Errorf("%w (pid %d)", ErrAlreadyRunning, pid)
	}
	// 0600: on Unix this sits in /tmp alongside every other user's files, and
	// which PID Pylon runs as is nobody else's business.
	return os.WriteFile(d.pidPath, []byte(strconv.Itoa(os.Getpid())), 0o600)
}

func (d *Daemon) removePIDFile() {
	_ = os.Remove(d.pidPath)
}

// livePID returns the PID in the PID file if that process is still alive.
func (d *Daemon) livePID() (int, bool) {
	b, err := os.ReadFile(d.pidPath)
	if err != nil {
		return 0, false
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(b)))
	if err != nil || pid <= 0 {
		return 0, false
	}
	if !processAlive(pid) {
		return 0, false
	}
	return pid, true
}

// clearStaleSocket removes a leftover socket file when no daemon is alive behind
// it, so a crash-restart can rebind cleanly.
func (d *Daemon) clearStaleSocket() error {
	if _, err := os.Stat(d.socketPath); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	// A socket exists. If a live daemon answers, refuse; otherwise it's stale.
	if pingDaemon(d.socketPath) {
		return ErrAlreadyRunning
	}
	return os.Remove(d.socketPath)
}

// processAlive reports whether a process with the given PID exists.
func processAlive(pid int) bool {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	// On Unix, signal 0 probes existence without affecting the process.
	err = proc.Signal(syscall.Signal(0))
	return err == nil || errors.Is(err, os.ErrPermission)
}

// pingDaemon returns true if a daemon answers "ping" on the socket.
func pingDaemon(socketPath string) bool {
	resp, err := Send(socketPath, ipc.Request{Cmd: "ping"})
	return err == nil && resp.OK
}
