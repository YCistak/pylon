package selfupdate

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// The GUI is updated because it cannot update itself, and it is found by sitting
// next to the daemon — the layout every archive unpacks into.
func TestGuiBesideFindsTheInstalledGUI(t *testing.T) {
	if guiName() == "" {
		t.Skip("no single-file GUI on " + runtime.GOOS)
	}
	dir := t.TempDir()
	exe := filepath.Join(dir, daemonName())
	gui := filepath.Join(dir, guiName())
	write(t, exe)
	write(t, gui)

	// Compared against the resolved path, not the one just built: on Windows
	// t.TempDir() hands back an 8.3 short path (RUNNER~1) and guiBeside
	// canonicalises it, which is the same EvalSymlinks call that makes an
	// install through a symlinked ~/.local/bin land on the real file.
	want := gui
	if resolved, err := filepath.EvalSymlinks(gui); err == nil {
		want = resolved
	}
	if got := guiBeside(exe); got != want {
		t.Errorf("guiBeside = %q, want %q", got, want)
	}
}

// A daemon installed on its own is the normal CLI-only case, and must not turn
// the update into an error about a GUI nobody has.
func TestGuiBesideWithoutAGUI(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, daemonName())
	write(t, exe)

	if got := guiBeside(exe); got != "" {
		t.Errorf("guiBeside = %q, want empty", got)
	}
}

// Only a regular file is ours to swap. A directory of that name is a macOS-style
// bundle or someone's build tree, and replacing it with a binary would destroy
// it — hence the Mode().IsRegular() check this pins down.
func TestGuiBesideIgnoresADirectory(t *testing.T) {
	if guiName() == "" {
		t.Skip("no single-file GUI on " + runtime.GOOS)
	}
	dir := t.TempDir()
	exe := filepath.Join(dir, daemonName())
	write(t, exe)
	if err := os.Mkdir(filepath.Join(dir, guiName()), 0o755); err != nil {
		t.Fatal(err)
	}

	if got := guiBeside(exe); got != "" {
		t.Errorf("guiBeside = %q, want empty — a directory is not a binary", got)
	}
}

// A GUI somewhere else on $PATH belongs to an installation nobody asked us to
// touch, so only the daemon's own directory is searched.
func TestGuiBesideDoesNotSearchElsewhere(t *testing.T) {
	if guiName() == "" {
		t.Skip("no single-file GUI on " + runtime.GOOS)
	}
	home := t.TempDir()
	other := t.TempDir()
	exe := filepath.Join(home, daemonName())
	write(t, exe)
	write(t, filepath.Join(other, guiName()))

	if got := guiBeside(exe); got != "" {
		t.Errorf("guiBeside = %q, want empty", got)
	}
}

// The archive nests everything under a versioned directory and extract() matches
// on the base name, so the names asked for have to be the bare ones.
func TestBinaryNamesAreBare(t *testing.T) {
	for _, name := range []string{daemonName(), guiName()} {
		if name == "" {
			continue
		}
		if filepath.Base(name) != name {
			t.Errorf("%q is a path, not a name", name)
		}
	}
}

// Permissions survive the swap: a copy someone made group-executable stays that
// way rather than being reset to whatever the archive happened to carry.
func TestPermOfKeepsWhatIsThere(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix permission bits")
	}
	path := filepath.Join(t.TempDir(), "pylon")
	write(t, path)
	if err := os.Chmod(path, 0o750); err != nil {
		t.Fatal(err)
	}
	if got := permOf(path); got != 0o750 {
		t.Errorf("permOf = %v, want %v", got, os.FileMode(0o750))
	}
}

// A target that is not there yet installs executable, not 0.
func TestPermOfFallsBackToExecutable(t *testing.T) {
	if got := permOf(filepath.Join(t.TempDir(), "absent")); got != 0o755 {
		t.Errorf("permOf = %v, want %v", got, os.FileMode(0o755))
	}
}

func write(t *testing.T, path string) {
	t.Helper()
	if err := os.WriteFile(path, []byte("binary"), 0o755); err != nil {
		t.Fatal(err)
	}
}

// buildArchive packs files into a .tar.gz shaped like a real release: everything
// nested under a versioned directory, which is why extract() matches on the base
// name.
func buildArchive(t *testing.T, files map[string][]byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, data := range files {
		// guiName() is empty on macOS, and callers pass it unconditionally.
		// An empty name would write a header ending in a slash, which tar
		// rejects — a test failing on the archive it built itself.
		if name == "" {
			continue
		}
		h := &tar.Header{
			Name:     "pylon-v9.9.9-linux-amd64/" + name,
			Mode:     0o755,
			Size:     int64(len(data)),
			Typeflag: tar.TypeReg,
		}
		if err := tw.WriteHeader(h); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write(data); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// fakeRelease serves a signed release over HTTP and returns it ready to install.
func fakeRelease(t *testing.T, contents map[string][]byte) (*Client, Release) {
	t.Helper()
	sign := testKey(t)

	asset := AssetName("v9.9.9")
	archive := buildArchive(t, contents)
	sums := sumsFor(map[string][]byte{asset: archive})
	sig := sign(sums)

	body := map[string][]byte{asset: archive, "SHA256SUMS": sums, "SHA256SUMS.sig": sig}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, ok := body[strings.TrimPrefix(r.URL.Path, "/")]
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Write(data)
	}))
	t.Cleanup(srv.Close)

	rel := Release{Version: "v9.9.9", assets: map[string]string{}}
	for name := range body {
		rel.assets[name] = srv.URL + "/" + name
	}
	return &Client{HTTP: srv.Client()}, rel
}

// The point of the whole change: an update replaces the GUI too. It cannot
// replace itself — on Windows a process cannot overwrite the binary it runs
// from — so the daemon does it, and before this the GUI simply stayed behind at
// the old version with nothing saying so.
func TestApplyReplacesBothBinaries(t *testing.T) {
	if guiName() == "" {
		t.Skip("no single-file GUI on " + runtime.GOOS)
	}
	dir := t.TempDir()
	exe := filepath.Join(dir, daemonName())
	gui := filepath.Join(dir, guiName())
	write(t, exe)
	write(t, gui)

	c, rel := fakeRelease(t, map[string][]byte{
		daemonName(): []byte("new daemon"),
		guiName():    []byte("new gui"),
	})
	if err := c.applyTo(context.Background(), rel, exe); err != nil {
		t.Fatalf("applyTo: %v", err)
	}

	if got := read(t, exe); got != "new daemon" {
		t.Errorf("daemon = %q, want the new one", got)
	}
	if got := read(t, gui); got != "new gui" {
		t.Errorf("GUI = %q, want the new one — it was left at the old version", got)
	}
}

// A daemon installed on its own must still update. Older releases predate the
// GUI shipping at all, and refusing over that would strand every CLI install.
func TestApplyWithoutAGUIInstalled(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, daemonName())
	write(t, exe)

	c, rel := fakeRelease(t, map[string][]byte{
		daemonName(): []byte("new daemon"),
		guiName():    []byte("new gui"),
	})
	if err := c.applyTo(context.Background(), rel, exe); err != nil {
		t.Fatalf("applyTo: %v", err)
	}
	if got := read(t, exe); got != "new daemon" {
		t.Errorf("daemon = %q, want the new one", got)
	}
	// Guarded, not skipped: the daemon half of this test is the part that
	// matters on macOS too. filepath.Join(dir, "") is dir, which of course
	// exists, so asking whether "the GUI appeared" is only a question where
	// there is a GUI file to appear.
	if name := guiName(); name != "" {
		if _, err := os.Stat(filepath.Join(dir, name)); !os.IsNotExist(err) {
			t.Error("a GUI was installed where the user had none")
		}
	}
}

// An archive with no GUI in it is an older release, not a broken one.
func TestApplyWhenTheArchiveHasNoGUI(t *testing.T) {
	if guiName() == "" {
		t.Skip("no single-file GUI on " + runtime.GOOS)
	}
	dir := t.TempDir()
	exe := filepath.Join(dir, daemonName())
	gui := filepath.Join(dir, guiName())
	write(t, exe)
	write(t, gui)

	c, rel := fakeRelease(t, map[string][]byte{daemonName(): []byte("new daemon")})
	if err := c.applyTo(context.Background(), rel, exe); err != nil {
		t.Fatalf("applyTo: %v", err)
	}
	if got := read(t, exe); got != "new daemon" {
		t.Errorf("daemon = %q, want the new one", got)
	}
	if got := read(t, gui); got != "binary" {
		t.Errorf("GUI = %q, want it untouched", got)
	}
}

// Nothing is written until the signature verifies. A tampered archive has to
// leave both binaries exactly as they were, not one of them.
func TestApplyWritesNothingWhenTheChecksumIsWrong(t *testing.T) {
	if guiName() == "" {
		t.Skip("no single-file GUI on " + runtime.GOOS)
	}
	dir := t.TempDir()
	exe := filepath.Join(dir, daemonName())
	gui := filepath.Join(dir, guiName())
	write(t, exe)
	write(t, gui)

	sign := testKey(t)
	asset := AssetName("v9.9.9")
	archive := buildArchive(t, map[string][]byte{daemonName(): []byte("new daemon")})
	// Sign the checksums of something else entirely: the signature is valid,
	// the hash it vouches for is not this download's.
	sums := sumsFor(map[string][]byte{asset: []byte("a different archive")})

	body := map[string][]byte{asset: archive, "SHA256SUMS": sums, "SHA256SUMS.sig": sign(sums)}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(body[strings.TrimPrefix(r.URL.Path, "/")])
	}))
	defer srv.Close()

	rel := Release{Version: "v9.9.9", assets: map[string]string{}}
	for name := range body {
		rel.assets[name] = srv.URL + "/" + name
	}

	err := (&Client{HTTP: srv.Client()}).applyTo(context.Background(), rel, exe)
	if !errors.Is(err, ErrChecksumMismatch) {
		t.Fatalf("applyTo error = %v, want ErrChecksumMismatch", err)
	}
	if got := read(t, exe); got != "binary" {
		t.Errorf("daemon = %q, want it untouched", got)
	}
	if got := read(t, gui); got != "binary" {
		t.Errorf("GUI = %q, want it untouched", got)
	}
}

func read(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
