package ipc

import (
	"fmt"
	"os"
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
	}
}

// The point of the whole change: the socket must never sit directly in a
// directory every other user can write to. It used to be /tmp/pylon.sock, and
// the daemon behind it authenticates nobody — so binding that name first was
// enough to be sent `secret set <name> <api-key>` in plaintext.
func TestSocketIsNotDirectlyInAWorldWritableDirectory(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("%LocalAppData% is already per-user")
	}
	for _, p := range []string{platformSocketPath(), platformPIDPath()} {
		if dir := filepath.Dir(p); dir == os.TempDir() || dir == "/tmp" {
			t.Errorf("%q sits directly in %s", p, dir)
		}
	}
}

// On Linux the session already provides a per-user directory, made 0700 and
// cleaned up on logout. Using it means Pylon creates nothing and inherits a
// guarantee it did not have to write.
func TestRuntimeDirPrefersXDG(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("no XDG_RUNTIME_DIR")
	}
	t.Setenv("XDG_RUNTIME_DIR", "/run/user/4242")
	if got := platformSocketPath(); got != "/run/user/4242/pylon/pylon.sock" {
		t.Errorf("socket = %q", got)
	}
}

// Without one — macOS, a cron job, a stripped container — the fallback carries
// the uid in its name, which is what stops two users racing for it. Same shape
// tmux and gpg-agent use.
func TestRuntimeDirFallsBackPerUser(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("no XDG_RUNTIME_DIR")
	}
	t.Setenv("XDG_RUNTIME_DIR", "")
	want := filepath.Join(os.TempDir(), fmt.Sprintf("pylon-%d", os.Getuid()))
	if got := filepath.Dir(platformSocketPath()); got != want {
		t.Errorf("dir = %q, want %q", got, want)
	}
}
