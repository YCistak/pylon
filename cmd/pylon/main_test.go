package main

import (
	"os"
	"path/filepath"
	"testing"
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
