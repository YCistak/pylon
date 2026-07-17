package selfupdate

import (
	"fmt"
	"os"
	"path/filepath"
)

// replace swaps the file at dst for data, atomically enough that a crash or a
// power cut cannot leave a half-written binary in place.
//
// The new file is written beside the target (a rename is only atomic within a
// filesystem, and /tmp is often a different one), then renamed over it. On
// Windows a running executable cannot be overwritten but can be renamed, so the
// old one is moved aside first and left for the next start to sweep up — see
// sweepOld.
func replace(dst string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(dst)

	tmp, err := os.CreateTemp(dir, ".pylon-new-*")
	if err != nil {
		return fmt.Errorf("stage %s: %w", dst, err)
	}
	staged := tmp.Name()
	defer os.Remove(staged) // no-op once the rename below succeeds

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("write %s: %w", staged, err)
	}
	// Flush to disk before the rename: renaming a file whose contents are still
	// in the page cache can survive as an empty file across a crash.
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("sync %s: %w", staged, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close %s: %w", staged, err)
	}
	if err := os.Chmod(staged, mode); err != nil {
		return fmt.Errorf("chmod %s: %w", staged, err)
	}

	if isWindows() {
		// Best effort: if dst is not running, this leaves nothing behind.
		old := dst + ".old"
		_ = os.Remove(old)
		if err := os.Rename(dst, old); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("move aside %s: %w", dst, err)
		}
		if err := os.Rename(staged, dst); err != nil {
			// Put the original back rather than leaving nothing installed.
			_ = os.Rename(old, dst)
			return fmt.Errorf("install %s: %w", dst, err)
		}
		_ = os.Remove(old) // succeeds unless it is the running process
		return nil
	}

	if err := os.Rename(staged, dst); err != nil {
		return fmt.Errorf("install %s: %w", dst, err)
	}
	return nil
}

// SweepOld removes the "<exe>.old" a Windows update leaves behind when it
// cannot delete the binary it is running from. Call it early at startup, where
// the previous executable is no longer running and the file is finally
// deletable. It is a no-op everywhere else.
func SweepOld() {
	if !isWindows() {
		return
	}
	exe, err := os.Executable()
	if err != nil {
		return
	}
	_ = os.Remove(exe + ".old")
}
