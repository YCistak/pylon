package main

import (
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

// fakeDaemon listens on a temporary socket and answers every request with resp,
// recording what it was asked. It lets the IPC helpers be exercised without a
// real daemon, which the GUI otherwise has no way to stand up in a test.
func fakeDaemon(t *testing.T, resp response) *recorder {
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

	rec := &recorder{}
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			var req request
			_ = json.NewDecoder(conn).Decode(&req)
			rec.add(req)
			_ = json.NewEncoder(conn).Encode(resp)
			conn.Close()
		}
	}()
	return rec
}

// recorder collects the requests the fake daemon received. The daemon answers on
// its own goroutine, so access is guarded.
type recorder struct {
	mu   sync.Mutex
	reqs []request
}

func (r *recorder) add(req request) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.reqs = append(r.reqs, req)
}

func (r *recorder) last() request {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.reqs) == 0 {
		return request{}
	}
	return r.reqs[len(r.reqs)-1]
}

// A daemon that is not answering must read as "offline", not "unavailable".
// Conflating the two made a cold launch — the GUI has spawned the daemon but it
// is still starting — tell the user sign-in was missing from the build.
func TestAuthStatusOfflineWhenDaemonDown(t *testing.T) {
	t.Setenv("PYLON_SOCKET", filepath.Join(t.TempDir(), "yok.sock"))

	if got := (&App{}).AuthStatus("google"); got != "offline" {
		t.Fatalf("AuthStatus with no daemon = %q, want \"offline\"", got)
	}
}

// A daemon that answers but refuses the command is the real "no OAuth client in
// this build" case.
func TestAuthStatusUnavailableWhenDaemonRefuses(t *testing.T) {
	fakeDaemon(t, response{OK: false, Error: "yapılandırılmadı"})

	if got := (&App{}).AuthStatus("google"); got != "unavailable" {
		t.Fatalf("AuthStatus on a refusing daemon = %q, want \"unavailable\"", got)
	}
}

func TestAuthStatusPassesDaemonAnswerThrough(t *testing.T) {
	for _, want := range []string{"connected", "ready"} {
		t.Run(want, func(t *testing.T) {
			fakeDaemon(t, response{OK: true, Text: want})

			if got := (&App{}).AuthStatus("google"); got != want {
				t.Fatalf("AuthStatus = %q, want %q", got, want)
			}
		})
	}
}

// The service name is a parameter now, not baked into the command: asking about
// Spotify must not quietly report on Google.
func TestAuthCommandsCarryTheService(t *testing.T) {
	rec := fakeDaemon(t, response{OK: true, Text: "connected"})
	app := &App{}

	app.AuthStatus("spotify")
	if got := rec.last().Args; len(got) != 2 || got[0] != "spotify" || got[1] != "status" {
		t.Fatalf("status args = %v", got)
	}

	if err := app.AuthLogout("spotify"); err != nil {
		t.Fatalf("logout: %v", err)
	}
	if got := rec.last().Args; len(got) != 2 || got[0] != "spotify" || got[1] != "logout" {
		t.Fatalf("logout args = %v", got)
	}
}

// Deleting a key is the one vault operation with no undo, so a daemon that
// refuses must surface as an error rather than a silent success.
func TestDeleteSecretReportsRefusal(t *testing.T) {
	rec := fakeDaemon(t, response{OK: false, Error: "silinemedi"})

	err := (&App{}).DeleteSecret("gemini")
	if err == nil {
		t.Fatal("expected the daemon's refusal to surface")
	}
	if got := rec.last().Args; len(got) != 2 || got[0] != "rm" || got[1] != "gemini" {
		t.Fatalf("args = %v", got)
	}
}
