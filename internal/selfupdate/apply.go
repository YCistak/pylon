package selfupdate

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
)

// maxDownload caps a release asset. The archives are ~15 MB.
const maxDownload = 512 << 20

// daemonName is the binary this package always replaces.
func daemonName() string {
	if runtime.GOOS == "windows" {
		return "pylon.exe"
	}
	return "pylon"
}

// guiName is the GUI that ships in the same archive, and is empty where there is
// no single file to swap.
//
// The GUI cannot update itself — a process cannot overwrite the binary it is
// running from on Windows, and on neither platform can it restart into the new
// one without losing its state. The daemon can, because it is a different
// process, and this is the whole reason the button in the GUI's Hakkında tab
// goes through the daemon rather than doing the work itself.
//
// macOS is left out: there the GUI is a .app bundle, and swapping one file
// inside it produces a bundle whose Info.plist, resources and executable
// disagree about which version they are.
func guiName() string {
	switch runtime.GOOS {
	case "windows":
		return "pylon-ui.exe"
	case "darwin":
		return ""
	}
	return "pylon-ui"
}

// guiBeside returns the GUI binary installed next to exe, or "" if there is
// none. Only a GUI in the daemon's own directory is treated as part of this
// installation: one found anywhere else belongs to a copy nobody asked us to
// touch.
func guiBeside(exe string) string {
	name := guiName()
	if name == "" {
		return ""
	}
	p := filepath.Join(filepath.Dir(exe), name)
	fi, err := os.Stat(p)
	if err != nil || !fi.Mode().IsRegular() {
		return ""
	}
	// Same as the daemon: install where the symlink points, not over the link.
	if resolved, err := filepath.EvalSymlinks(p); err == nil {
		p = resolved
	}
	return p
}

// Apply downloads rel, verifies it, and replaces the running daemon binary —
// and the GUI beside it, which cannot replace itself. The caller restarts; a
// process cannot re-exec itself into a new binary without leaving the old one's
// state behind.
//
// Nothing touches disk until the signature and the checksum both pass.
func (c *Client) Apply(ctx context.Context, rel Release) error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("locate this binary: %w", err)
	}
	return c.applyTo(ctx, rel, exe)
}

// applyTo is Apply with the install location handed in. It is separate only so
// that the part which overwrites binaries can be tested against a temp
// directory: os.Executable() is the one input a test cannot supply, and
// everything downstream of it is the part worth proving.
func (c *Client) applyTo(ctx context.Context, rel Release, exe string) error {
	if publicKey == "" {
		return ErrUpdatesDisabled
	}
	if by, packaged := Packaged(); packaged {
		return fmt.Errorf("%w: %s", ErrPackaged, by)
	}

	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	// Fail before the download rather than after it: an unwritable target is
	// the common case (a copy under /opt someone installed by hand), and
	// finding out at the last step wastes the transfer and reads as a crash.
	if err := writable(exe); err != nil {
		return err
	}
	gui := guiBeside(exe)
	if gui != "" {
		if err := writable(gui); err != nil {
			return err
		}
	}

	sums, err := c.signedSums(ctx, rel)
	if err != nil {
		return err
	}

	asset := AssetName(rel.Version)
	url, ok := rel.assets[asset]
	if !ok {
		return fmt.Errorf("release %s has no build for %s/%s", rel.Version, runtime.GOOS, runtime.GOARCH)
	}
	archive, err := c.fetch(ctx, url)
	if err != nil {
		return fmt.Errorf("download %s: %w", asset, err)
	}
	if err := checkAsset(sums, asset, archive); err != nil {
		return err
	}

	name := daemonName()
	want := map[string]bool{name: true}
	if gui != "" {
		want[guiName()] = true
	}
	files, err := extract(archive, want)
	if err != nil {
		return err
	}
	bin, ok := files[name]
	if !ok {
		return fmt.Errorf("%s holds no %s", asset, name)
	}

	if err := replace(exe, bin, permOf(exe)); err != nil {
		return err
	}

	// The daemon goes first on purpose. Two binaries cannot be swapped as one
	// operation, so an interrupted update leaves a mismatched pair either way —
	// and of the two, an old GUI against a new daemon is the harmless one. The
	// wire protocol is kept in sync by hand and only grows, so an old GUI sends
	// commands the new daemon still understands; the reverse pairing is what
	// produces "unknown command" against a daemon that is simply older.
	//
	// A GUI missing from the archive is not an error. Older releases predate it
	// shipping, and refusing to update the daemon over that would be a strange
	// place to draw the line.
	if gui != "" {
		if bin, ok := files[guiName()]; ok {
			if err := replace(gui, bin, permOf(gui)); err != nil {
				return fmt.Errorf("the daemon was updated but the GUI was not: %w", err)
			}
		}
	}
	return nil
}

// permOf reports the permissions to install with: whatever the file being
// replaced already had, so a copy someone made group-executable stays that way.
func permOf(path string) os.FileMode {
	if fi, err := os.Stat(path); err == nil {
		return fi.Mode().Perm()
	}
	return 0o755
}

// signedSums fetches SHA256SUMS and its signature and returns the verified map.
func (c *Client) signedSums(ctx context.Context, rel Release) (map[string]string, error) {
	sumsURL, ok := rel.assets["SHA256SUMS"]
	if !ok {
		return nil, fmt.Errorf("%w: release %s publishes no checksums", ErrBadSignature, rel.Version)
	}
	sigURL, ok := rel.assets["SHA256SUMS.sig"]
	if !ok {
		return nil, fmt.Errorf("%w: release %s is unsigned", ErrBadSignature, rel.Version)
	}

	sums, err := c.fetch(ctx, sumsURL)
	if err != nil {
		return nil, fmt.Errorf("download checksums: %w", err)
	}
	sig, err := c.fetch(ctx, sigURL)
	if err != nil {
		return nil, fmt.Errorf("download signature: %w", err)
	}
	return verifySums(sums, sig)
}

func (c *Client) fetch(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s: %s", url, resp.Status)
	}
	return io.ReadAll(io.LimitReader(resp.Body, maxDownload))
}

// writable reports whether the update could install over path. It probes the
// directory, since replacing a binary means creating a sibling and renaming
// over it — which needs the directory writable, not the file.
func writable(path string) error {
	dir := filepath.Dir(path)
	f, err := os.CreateTemp(dir, ".pylon-probe-*")
	if err != nil {
		return fmt.Errorf("cannot update %s: %s is not writable (%w)", path, dir, err)
	}
	name := f.Name()
	f.Close()
	return os.Remove(name)
}
