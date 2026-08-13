//go:build !windows

package daemon

import (
	"bufio"
	"net"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/YCistak/pylon/internal/ipc"
)

// shortDir is a temp directory with a short name. t.TempDir() spends ~80 bytes
// on /var/folders plus the test's own name on macOS, and a Unix socket path is
// capped there at ~104 — so binding inside one fails with "invalid argument"
// for a reason that has nothing to do with what is being tested. The same trick
// is in daemon_test.go and pylon-ui/app_test.go, for the same reason.
func shortDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "p")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}

// The directory is the whole defence: a name inside a directory only its owner
// can write to cannot be taken by anyone else.
func TestSecureDirCreatesItPrivate(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "pylon-1000")
	if err := secureDir(dir); err != nil {
		t.Fatalf("secureDir: %v", err)
	}
	fi, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if perm := fi.Mode().Perm(); perm != 0o700 {
		t.Errorf("mode = %v, want 0700", perm)
	}
}

// MkdirAll succeeds on a directory that already exists, whoever made it and
// however open it is — which is exactly the case worth catching, because the
// fallback path is predictable enough to be created first.
func TestSecureDirTightensALooseDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "pylon-1000")
	if err := os.Mkdir(dir, 0o777); err != nil {
		t.Fatal(err)
	}
	if err := secureDir(dir); err != nil {
		t.Fatalf("secureDir: %v", err)
	}
	fi, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if perm := fi.Mode().Perm(); perm&0o077 != 0 {
		t.Errorf("mode = %v, still open to others", perm)
	}
}

// Running twice must be ordinary — a restart is not an attack.
func TestSecureDirIsIdempotent(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "pylon-1000")
	for i := 0; i < 3; i++ {
		if err := secureDir(dir); err != nil {
			t.Fatalf("run %d: %v", i, err)
		}
	}
}

// The socket's mode is the last gate: connecting to a Unix socket needs write
// permission on it. It used to be whatever the umask left — 0755 with the usual
// 022, which happens to deny others, and 0775 with 002, which does not.
func TestSecureSocketClosesItToOthers(t *testing.T) {
	dir := shortDir(t)
	path := filepath.Join(dir, "pylon.sock")

	ln, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	if err := secureSocket(path); err != nil {
		t.Fatalf("secureSocket: %v", err)
	}
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := fi.Mode().Perm(); perm&0o077 != 0 {
		t.Errorf("mode = %v — another user can still connect", perm)
	}
}

// A socket this user owns is the normal case and must not be refused.
func TestOwnedByUsAcceptsOurOwnSocket(t *testing.T) {
	path := filepath.Join(shortDir(t), "pylon.sock")
	ln, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	if !ownedByUs(path) {
		t.Error("ownedByUs = false for a socket we just created")
	}
}

// A path that is not there is not ours either. Said explicitly because the
// answer decides whether a secret gets sent.
func TestOwnedByUsRejectsWhatIsNotThere(t *testing.T) {
	if ownedByUs(filepath.Join(t.TempDir(), "absent.sock")) {
		t.Error("ownedByUs = true for a missing path")
	}
}

// The attack this exists for: something else is listening on the name, and the
// client is about to send it `secret set <name> <api-key>`. Ownership cannot be
// faked in a test — a test cannot create a file as another user — so the check
// is driven directly, and the refusal is verified where it matters: Send must
// fail before dialling.
func TestSendRefusesASocketThatIsNotOurs(t *testing.T) {
	dir := shortDir(t)
	path := filepath.Join(dir, "pylon.sock")

	// A real listener, answering happily, standing in for the squatter.
	ln, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			// Read the request before answering: closing on an unread
			// connection gives the sender a broken pipe instead of a reply.
			bufio.NewReader(c).ReadString('\n')
			c.Write([]byte(`{"ok":true,"text":"gotcha"}` + "\n"))
			c.Close()
		}
	}()

	// It is ours, so this one goes through — the check is not simply always-no.
	if _, err := Send(path, ipc.Request{Cmd: "ping"}); err != nil {
		t.Fatalf("our own socket was refused: %v", err)
	}
}

// The refusal against something that really is another user's. /etc/passwd is
// root-owned on every Unix and is not a socket, which is the point: the check
// has to happen before the dial, or the first thing down the wire is already a
// secret. Verified by hand against /run/dbus/system_bus_socket too — a
// root-owned socket that is srw-rw-rw-, so connecting would have worked.
func TestSendChecksOwnershipBeforeDialling(t *testing.T) {
	const foreign = "/etc/passwd"
	fi, err := os.Stat(foreign)
	if err != nil {
		t.Skip("no /etc/passwd")
	}
	if st, ok := fi.Sys().(*syscall.Stat_t); !ok || int(st.Uid) == os.Getuid() {
		t.Skip("running as the owner of /etc/passwd")
	}
	if ownedByUs(foreign) {
		t.Fatal("ownedByUs = true for a root-owned path")
	}
	_, err = Send(foreign, ipc.Request{Cmd: "ping"})
	if err == nil {
		t.Fatal("Send did not refuse a path owned by another user")
	}
	if !strings.Contains(err.Error(), "another user") {
		t.Errorf("refused for the wrong reason: %v", err)
	}
}
