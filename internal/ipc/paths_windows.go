//go:build windows

package ipc

import (
	"os"
	"path/filepath"
)

// Windows has no /tmp, so the Unix defaults are not just unidiomatic there —
// they are unopenable. AF_UNIX itself works (Windows 10 1803+), it only needs a
// real Windows path, so the socket lives under %LocalAppData%\pylon alongside
// the PID file. Callers must ensure that directory exists before listening; see
// daemon.Run.

// pylonDir is %LocalAppData%\pylon, falling back to the temp dir when the cache
// dir is unavailable (e.g. a stripped service account with no profile).
func pylonDir() string {
	if dir, err := os.UserCacheDir(); err == nil {
		return filepath.Join(dir, "pylon")
	}
	return filepath.Join(os.TempDir(), "pylon")
}

func platformSocketPath() string { return filepath.Join(pylonDir(), "pylon.sock") }

func platformPIDPath() string { return filepath.Join(pylonDir(), "pylon.pid") }
