package ipc

import (
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestDefaultPathsHonourEnv(t *testing.T) {
	t.Setenv(socketEnv, "/custom/pylon.sock")
	t.Setenv(pidEnv, "/custom/pylon.pid")

	if got := DefaultSocketPath(); got != "/custom/pylon.sock" {
		t.Errorf("DefaultSocketPath() = %q, want the %s override", got, socketEnv)
	}
	if got := DefaultPIDPath(); got != "/custom/pylon.pid" {
		t.Errorf("DefaultPIDPath() = %q, want the %s override", got, pidEnv)
	}
}

// An empty env var must not shadow the platform default — otherwise a shell
// exporting PYLON_SOCKET="" would leave the daemon listening on "".
func TestEmptyEnvFallsBackToPlatformDefault(t *testing.T) {
	t.Setenv(socketEnv, "")
	if got := DefaultSocketPath(); got != platformSocketPath() {
		t.Errorf("DefaultSocketPath() = %q, want platform default %q", got, platformSocketPath())
	}
}

// The GUI (a separate module) hardcodes its own copy of these defaults, so the
// shape they must agree on is worth pinning: absolute, and never under /tmp on
// Windows, which has no such directory.
func TestPlatformDefaultsAreUsable(t *testing.T) {
	for _, p := range []string{platformSocketPath(), platformPIDPath()} {
		if !filepath.IsAbs(p) {
			t.Errorf("%q is not absolute", p)
		}
		if runtime.GOOS == "windows" && strings.HasPrefix(p, "/tmp") {
			t.Errorf("%q is a Unix path on Windows", p)
		}
		if runtime.GOOS != "windows" && p != filepath.Join("/tmp", filepath.Base(p)) {
			t.Errorf("%q left /tmp; existing configs and docs point there", p)
		}
	}
}
