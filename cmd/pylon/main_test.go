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
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", home)
	t.Chdir(t.TempDir()) // no pylon.yaml here

	want := filepath.Join(home, "pylon", "pylon.yaml")
	if got := configPath(); got != want {
		t.Fatalf("configPath() = %q, want %q", got, want)
	}
}
