//go:build !windows

package daemon

import (
	"fmt"
	"os"
	"syscall"
)

// The socket and PID file live in a directory only their owner may write to
// (see internal/ipc/paths_unix.go). These two functions are what makes that
// claim true rather than assumed: one on the way up, refusing to listen inside
// a directory somebody else controls, and one on the way in, refusing to send a
// secret to a socket somebody else owns.

// secureDir creates dir 0700 and reports an error unless it belongs to us and
// nobody else can write to it.
//
// Creating it is not enough. On the /tmp fallback the name is predictable, so
// another user can create it first — and then own the directory the daemon is
// about to put its socket in. MkdirAll succeeds silently on a directory that
// already exists, whoever made it, which is exactly the case worth catching.
func secureDir(dir string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create %s: %w", dir, err)
	}
	fi, err := os.Stat(dir)
	if err != nil {
		return fmt.Errorf("stat %s: %w", dir, err)
	}
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return nil // no ownership information: nothing to check against
	}
	if int(st.Uid) != os.Getuid() {
		return fmt.Errorf("%s belongs to uid %d, not to you (uid %d) — refusing to run there",
			dir, st.Uid, os.Getuid())
	}
	// Tighten a directory that exists but is loose (an upgrade from an older
	// layout, or a umask that was open when it was made).
	if fi.Mode().Perm()&0o077 != 0 {
		if err := os.Chmod(dir, 0o700); err != nil {
			return fmt.Errorf("%s is writable by others and cannot be tightened: %w", dir, err)
		}
	}
	return nil
}

// secureSocket takes the listening socket down to 0600.
//
// Connecting to a Unix socket needs write permission on it, so its mode is the
// last gate — and it was left to whatever umask happened to be set. With the
// usual 022 that lands on 0755, which happens to deny others; with 002 it does
// not. A security property should not depend on a shell setting.
func secureSocket(path string) error {
	return os.Chmod(path, 0o600)
}

// ownedByUs reports whether path belongs to the current user. Clients call it
// before sending anything: the daemon's own protections do not help if the
// thing answering is not the daemon.
func ownedByUs(path string) bool {
	fi, err := os.Stat(path)
	if err != nil {
		return false
	}
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return true // no ownership information to judge on
	}
	return int(st.Uid) == os.Getuid()
}
