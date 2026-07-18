package selfupdate

import "testing"

// The AUR package sets Channel at build time. Missing this check means the
// updater overwrites a pacman-owned file, which desyncs the package database
// and gets reverted on the next -Syu anyway.
func TestPackagedHonoursChannel(t *testing.T) {
	old := Channel
	Channel = "aur"
	t.Cleanup(func() { Channel = old })

	by, packaged := Packaged()
	if !packaged {
		t.Fatal("Channel=aur should report packaged")
	}
	if by != "aur" {
		t.Errorf("reason = %q, want %q", by, "aur")
	}
}

// A distro that compiles without the ldflag still must not be written over,
// so a system prefix alone is enough to refuse.
func TestSystemPrefixesCoverPackageManagers(t *testing.T) {
	if isWindows() {
		t.Skip("no system prefix convention on Windows")
	}
	prefixes := systemPrefixes()
	for _, want := range []string{"/usr/bin/", "/opt/homebrew/", "/nix/store/"} {
		found := false
		for _, p := range prefixes {
			if p == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("%s missing from systemPrefixes", want)
		}
	}
}

// An unmarked binary somewhere writable — the unpacked-a-zip case — is exactly
// what self-update is for, so it must not be mistaken for a packaged install.
func TestUnpackedBuildIsNotPackaged(t *testing.T) {
	old := Channel
	Channel = ""
	t.Cleanup(func() { Channel = old })

	// The test binary itself lives under a temp dir, which is the shape of an
	// unpacked release.
	if by, packaged := Packaged(); packaged {
		t.Fatalf("test binary reported packaged (%s); it is not under a system prefix", by)
	}
}
