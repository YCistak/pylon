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

// daemonName is the binary this package replaces. The GUI ships alongside it
// but is left to its own update path: it cannot be swapped while it is the
// process asking for the update, and on macOS it is a bundle, not a file.
func daemonName() string {
	if runtime.GOOS == "windows" {
		return "pylon.exe"
	}
	return "pylon"
}

// Apply downloads rel, verifies it, and replaces the running daemon binary.
// The caller restarts; a process cannot re-exec itself into a new binary
// without leaving the old one's state behind.
//
// Nothing touches disk until the signature and the checksum both pass.
func (c *Client) Apply(ctx context.Context, rel Release) error {
	if publicKey == "" {
		return ErrUpdatesDisabled
	}
	if by, packaged := Packaged(); packaged {
		return fmt.Errorf("%w: %s", ErrPackaged, by)
	}

	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("locate this binary: %w", err)
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
	files, err := extract(archive, map[string]bool{name: true})
	if err != nil {
		return err
	}
	bin, ok := files[name]
	if !ok {
		return fmt.Errorf("%s holds no %s", asset, name)
	}

	mode := os.FileMode(0o755)
	if fi, err := os.Stat(exe); err == nil {
		mode = fi.Mode().Perm()
	}
	return replace(exe, bin, mode)
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
