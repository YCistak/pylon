package selfupdate

import (
	"os"
	"path/filepath"
	"strings"
)

// Channel records how this copy was installed. The release workflow leaves it
// empty; a distro package sets it at build time, e.g. the AUR PKGBUILD passing
//
//	-ldflags "-X github.com/YCistak/pylon/internal/selfupdate.Channel=aur"
//
// so the binary knows not to fight pacman for its own file.
var Channel = ""

// Packaged reports whether a package manager owns this install, and why. Self
// updating then has to be refused: pacman, apt and Homebrew track the files
// they install, so replacing one behind their back leaves the package database
// describing a binary that no longer exists — and the next upgrade silently
// reverts the update anyway.
func Packaged() (string, bool) {
	if Channel != "" {
		return Channel, true
	}

	exe, err := os.Executable()
	if err != nil {
		return "", false
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}

	// A build that was not marked can still be packaged — a distro may compile
	// without the ldflag. Living under a system prefix is the giveaway: nothing
	// puts a binary there except a package manager or a deliberate root install,
	// and neither wants this process rewriting it.
	for _, prefix := range systemPrefixes() {
		if strings.HasPrefix(exe, prefix) {
			return "system install (" + prefix + ")", true
		}
	}
	return "", false
}

// systemPrefixes are the locations a package manager installs into.
func systemPrefixes() []string {
	if isWindows() {
		// Windows has no equivalent convention; installs land wherever the user
		// unpacked them, and scoop/winget keep their own copies under the user
		// profile, which is writable and fine to replace.
		return nil
	}
	return []string{
		"/usr/bin/", "/usr/local/bin/", "/usr/lib/", "/opt/",
		"/nix/store/",                 // immutable by design
		"/var/lib/flatpak/", "/snap/", // sandboxed, read-only mounts
		"/Applications/",                     // macOS: a .app installed system-wide
		"/opt/homebrew/", "/home/linuxbrew/", // Homebrew prefixes
	}
}
