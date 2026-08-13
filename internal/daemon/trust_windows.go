//go:build windows

package daemon

import (
	"fmt"
	"os"
)

// Windows keeps the socket under %LocalAppData%\pylon, which is already inside
// the user's own profile — the shared-/tmp problem these functions exist for
// does not arise. The Unix permission bits have no meaning here either, so the
// mode calls would be lies rather than protections.

func secureDir(dir string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create %s: %w", dir, err)
	}
	return nil
}

func secureSocket(string) error { return nil }

func ownedByUs(string) bool { return true }
