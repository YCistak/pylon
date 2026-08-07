package main

import (
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"testing"
)

// fakeDaemon listens on a temporary socket and answers every request with resp.
// It lets the status helpers be exercised without a real daemon, which the GUI
// otherwise has no way to stand up in a test.
func fakeDaemon(t *testing.T, resp response) {
	t.Helper()

	// A Unix socket path is capped near 104 bytes on macOS, and it is the
	// directory that blows the budget, not the filename: t.TempDir() names the
	// directory after the test, so a long test name inside a long $TMPDIR
	// (/var/folders/…/T/ on macOS) overflows and Listen fails with "bind:
	// invalid argument" — which is what the macOS runner did while Linux and
	// Windows passed. A plain temp directory keeps the whole path short
	// regardless of what this test is called.
	dir, err := os.MkdirTemp("", "p")
	if err != nil {
		t.Fatalf("temp dir: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })

	sock := filepath.Join(dir, "s")
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { ln.Close() })
	t.Setenv("PYLON_SOCKET", sock)

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			var req request
			_ = json.NewDecoder(conn).Decode(&req)
			_ = json.NewEncoder(conn).Encode(resp)
			conn.Close()
		}
	}()
}

// A daemon that is not answering must read as "offline", not "unavailable".
// Conflating the two made a cold launch — the GUI has spawned the daemon but it
// is still starting — tell the user Google sign-in was missing from the build.
func TestGoogleStatusOfflineWhenDaemonDown(t *testing.T) {
	t.Setenv("PYLON_SOCKET", filepath.Join(t.TempDir(), "yok.sock"))

	if got := (&App{}).GoogleStatus(); got != "offline" {
		t.Fatalf("GoogleStatus with no daemon = %q, want \"offline\"", got)
	}
}

// A daemon that answers but refuses the command is the real "no OAuth client in
// this build" case.
func TestGoogleStatusUnavailableWhenDaemonRefuses(t *testing.T) {
	fakeDaemon(t, response{OK: false, Error: "yapılandırılmadı"})

	if got := (&App{}).GoogleStatus(); got != "unavailable" {
		t.Fatalf("GoogleStatus on a refusing daemon = %q, want \"unavailable\"", got)
	}
}

func TestGoogleStatusPassesDaemonAnswerThrough(t *testing.T) {
	for _, want := range []string{"connected", "ready"} {
		t.Run(want, func(t *testing.T) {
			fakeDaemon(t, response{OK: true, Text: want})

			if got := (&App{}).GoogleStatus(); got != want {
				t.Fatalf("GoogleStatus = %q, want %q", got, want)
			}
		})
	}
}
