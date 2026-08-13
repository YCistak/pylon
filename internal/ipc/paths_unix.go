//go:build !windows

package ipc

import (
	"fmt"
	"os"
	"path/filepath"
)

// The socket used to be /tmp/pylon.sock, chosen because /tmp is a real
// directory on every Unix. It is also the one directory every other user on the
// machine can write to, and the daemon authenticates nobody: whoever reaches
// the socket can run `secret set`, `do`, and `listen`, which opens the
// microphone. Nothing checked who owned the socket either, so on a shared
// machine another user could bind that name first — before Pylon's first run,
// or after the daemon exits and removes it — and receive
// `pylon secret set gemini <api-key>` in plaintext.
//
// So it moves somewhere only its owner can write:
//
//	$XDG_RUNTIME_DIR/pylon/   the answer on Linux — /run/user/<uid>, made 0700
//	                          by the login session and cleaned up on logout
//	/tmp/pylon-<uid>/         everywhere else, created 0700 by daemon.Run
//
// The fallback is the shape tmux and gpg-agent use, for this reason. A
// directory only its owner can write to means the name inside it cannot be
// taken by anyone else, which is the property that was missing.
//
// $PYLON_SOCKET still overrides both, and stays the way to put the socket
// somewhere unusual — it is also the only lever that moves the GUI, which
// carries its own copy of these paths.

// runtimeDir is the per-user directory holding the socket and PID file. It
// creates nothing: daemon.Run does that, and checks what it finds there.
func runtimeDir() string {
	if d := os.Getenv("XDG_RUNTIME_DIR"); d != "" {
		return filepath.Join(d, "pylon")
	}
	return filepath.Join(os.TempDir(), fmt.Sprintf("pylon-%d", os.Getuid()))
}

func platformSocketPath() string { return filepath.Join(runtimeDir(), "pylon.sock") }

func platformPIDPath() string { return filepath.Join(runtimeDir(), "pylon.pid") }
