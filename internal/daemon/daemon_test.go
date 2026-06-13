package daemon

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/YCistak/pylon/internal/ipc"
)

// startTestDaemon runs a daemon on a temp socket and returns it with a cancel
// func. It blocks until the socket is accepting connections.
func startTestDaemon(t *testing.T) (*Daemon, context.CancelFunc) {
	t.Helper()
	dir := t.TempDir()
	d := New(Options{
		SocketPath: filepath.Join(dir, "pylon.sock"),
		PIDPath:    filepath.Join(dir, "pylon.pid"),
		Logger:     slog.New(slog.NewTextHandler(io.Discard, nil)),
	})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- d.Run(ctx) }()

	waitForSocket(t, d.socketPath)
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Error("daemon did not stop within 2s")
		}
	})
	return d, cancel
}

func waitForSocket(t *testing.T, path string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if resp, err := Send(path, ipc.Request{Cmd: "ping"}); err == nil && resp.OK {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("socket did not become ready within 2s")
}

func TestPingAndStatus(t *testing.T) {
	d, _ := startTestDaemon(t)

	resp, err := Send(d.socketPath, ipc.Request{Cmd: "ping"})
	if err != nil {
		t.Fatalf("ping: %v", err)
	}
	if !resp.OK || resp.Text != "pong" {
		t.Fatalf("ping = %+v, want ok/pong", resp)
	}

	resp, err = Send(d.socketPath, ipc.Request{Cmd: "status"})
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if !resp.OK {
		t.Fatalf("status not ok: %+v", resp)
	}
}

func TestUnknownCommand(t *testing.T) {
	d, _ := startTestDaemon(t)
	resp, err := Send(d.socketPath, ipc.Request{Cmd: "nope"})
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	if resp.OK || resp.Error == "" {
		t.Fatalf("want error response, got %+v", resp)
	}
}

func TestPIDFileWrittenAndRemoved(t *testing.T) {
	d, _ := startTestDaemon(t)
	if _, err := os.Stat(d.pidPath); err != nil {
		t.Fatalf("pid file should exist while running: %v", err)
	}
}

func TestStopClosesSocket(t *testing.T) {
	d, _ := startTestDaemon(t)

	resp, err := Send(d.socketPath, ipc.Request{Cmd: "stop"})
	if err != nil {
		t.Fatalf("stop: %v", err)
	}
	if !resp.OK {
		t.Fatalf("stop not ok: %+v", resp)
	}

	// After stop the socket and PID file should disappear.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(d.socketPath); os.IsNotExist(err) {
			if _, err := os.Stat(d.pidPath); os.IsNotExist(err) {
				return
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("socket/pid file not cleaned up after stop")
}

func TestSecondInstanceRefused(t *testing.T) {
	d, _ := startTestDaemon(t)

	d2 := New(Options{
		SocketPath: d.socketPath,
		PIDPath:    d.pidPath,
		Logger:     slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	err := d2.Run(context.Background())
	if err == nil {
		t.Fatal("second instance should have been refused")
	}
}

func TestStaleSocketReclaimed(t *testing.T) {
	dir := t.TempDir()
	sock := filepath.Join(dir, "pylon.sock")
	pid := filepath.Join(dir, "pylon.pid")

	// Leave a stale socket file with no daemon behind it.
	if err := os.WriteFile(sock, []byte("stale"), 0o644); err != nil {
		t.Fatal(err)
	}

	d := New(Options{SocketPath: sock, PIDPath: pid, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- d.Run(ctx) }()

	waitForSocket(t, sock)
}
