package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// $PYLON_CONFIG is the explicit override and must win over everything else.
func TestConfigPathPrefersEnv(t *testing.T) {
	t.Setenv("PYLON_CONFIG", "/tmp/elsewhere.yaml")
	t.Chdir(t.TempDir())

	if got := configPath(); got != "/tmp/elsewhere.yaml" {
		t.Fatalf("configPath() = %q", got)
	}
}

// A checkout keeps working with no setup: a pylon.yaml next to you wins over
// the user config directory.
func TestConfigPathPrefersWorkingDirectory(t *testing.T) {
	t.Setenv("PYLON_CONFIG", "")
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "pylon.yaml"), []byte("voice: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)

	if got := configPath(); got != "pylon.yaml" {
		t.Fatalf("configPath() = %q, want the local file", got)
	}
}

// The case this function exists for: launched from a desktop entry or an
// application menu, the working directory is the user's home and holds no
// pylon.yaml. Before the fallback the daemon silently ran on defaults.
func TestConfigPathFallsBackToUserConfigDir(t *testing.T) {
	t.Setenv("PYLON_CONFIG", "")
	t.Chdir(t.TempDir()) // no pylon.yaml here

	// Where the user config directory lives is the OS's answer, not ours:
	// $XDG_CONFIG_HOME on Linux, ~/Library/Application Support on macOS,
	// %AppData% on Windows. Pointing XDG_CONFIG_HOME at a temp dir and asserting
	// against it passed only on Linux and failed the macOS and Windows runners.
	// Asking the same function configPath consults keeps the assertion — "the
	// user config dir, plus pylon/pylon.yaml" — without encoding one platform's
	// layout. Nothing is written, so no sandboxing is needed.
	dir, err := os.UserConfigDir()
	if err != nil {
		t.Skipf("no user config dir on this machine: %v", err)
	}

	want := filepath.Join(dir, "pylon", "pylon.yaml")
	if got := configPath(); got != want {
		t.Fatalf("configPath() = %q, want %q", got, want)
	}
}

// The last resort: an environment stripped of any home directory. The bare
// filename is returned rather than an empty path, which Load would read as the
// working directory — and a missing file there is just "use the defaults".
func TestConfigPathWithoutAUserConfigDir(t *testing.T) {
	t.Setenv("PYLON_CONFIG", "")
	t.Setenv("HOME", "")
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("AppData", "")
	t.Chdir(t.TempDir())

	if _, err := os.UserConfigDir(); err == nil {
		t.Skip("this OS still reports a user config dir with the environment cleared")
	}

	if got := configPath(); got != "pylon.yaml" {
		t.Fatalf("configPath() = %q, want the bare filename", got)
	}
}

// `pylon briefing` and `pylon briefing --speak` are two different daemon
// commands, because the generic "do" path never speaks. Sending the wrong one
// would silently give a briefing you cannot hear (or one you cannot silence).
func TestBriefingRequestPicksTheCommand(t *testing.T) {
	req, timeout, err := briefingRequest(nil)
	if err != nil {
		t.Fatalf("no args: %v", err)
	}
	if req.Cmd != "do" || len(req.Args) != 1 || req.Args[0] != "briefing.today" {
		t.Fatalf("bare briefing = %+v, want the silent do path", req)
	}
	if timeout != 30*time.Second {
		t.Fatalf("bare briefing timeout = %v", timeout)
	}

	for _, flag := range []string{"--speak", "-s"} {
		req, timeout, err := briefingRequest([]string{flag})
		if err != nil {
			t.Fatalf("%s: %v", flag, err)
		}
		if req.Cmd != "briefing" || len(req.Args) != 1 || req.Args[0] != "speak" {
			t.Fatalf("%s = %+v, want the speaking path", flag, req)
		}
		// Reading a briefing out loud runs well past the default deadline, and
		// giving up mid-sentence leaves the daemon talking to a closed socket.
		if timeout <= 30*time.Second {
			t.Fatalf("%s timeout = %v, want more than the default", flag, timeout)
		}
	}
}

func TestBriefingRequestRejectsGarbage(t *testing.T) {
	for _, args := range [][]string{{"preview"}, {"--speak", "extra"}, {"-x"}} {
		if _, _, err := briefingRequest(args); err == nil {
			t.Errorf("briefingRequest(%v) accepted an unknown argument", args)
		}
	}
}

// The catch-up exists because the scheduler only looks forward; these are the
// four cases that decide whether a start-up delivers a briefing.
func TestBriefingMissed(t *testing.T) {
	day := func(hour, min int) time.Time {
		return time.Date(2026, 8, 8, hour, min, 0, 0, time.Local)
	}
	const today = "2026-08-08"

	cases := []struct {
		name    string
		now     time.Time
		lastRun string
		want    bool
	}{
		{"before the hour", day(7, 0), "", false},
		{"after the hour, never run", day(9, 0), "", true},
		{"after the hour, ran yesterday", day(9, 0), "2026-08-07", true},
		{"after the hour, already ran today", day(9, 0), today, false},
		// Exactly on the minute counts as due: the scheduler would have fired.
		{"on the minute", day(8, 0), "", true},
	}
	for _, c := range cases {
		if got := briefingMissed(c.now, 8, 0, c.lastRun); got != c.want {
			t.Errorf("%s: briefingMissed = %v, want %v", c.name, got, c.want)
		}
	}
}
