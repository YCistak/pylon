package selfupdate

import (
	"strings"
	"testing"
)

func TestNewer(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"v0.2.0", "v0.1.0", true},
		{"v0.1.1", "v0.1.0", true},
		{"v1.0.0", "v0.9.9", true},
		{"v0.10.0", "v0.9.0", true}, // not a string compare
		{"v0.1.0", "v0.1.0", false},
		{"v0.1.0", "v0.2.0", false}, // never downgrade
		{"v0.1.0-rc1", "v0.1.0", false},
		{"v0.2.0-rc1", "v0.1.0", true},

		// Prerelease precedence. The suffix used to be discarded, so every
		// v0.1.0-alpha.N compared equal to v0.1.0 and to each other: an alpha
		// could never see a later alpha — or the release it led to — as an
		// upgrade, and `pylon update` reported "already current" on a build the
		// project had moved past.
		{"v0.1.0", "v0.1.0-alpha.1", true},  // the release supersedes its prerelease
		{"v0.1.0-alpha.1", "v0.1.0", false}, // and never the other way round
		{"v0.1.0-alpha.2", "v0.1.0-alpha.1", true},
		{"v0.1.0-alpha.1", "v0.1.0-alpha.2", false},
		{"v0.1.0-alpha.10", "v0.1.0-alpha.9", true}, // numeric, not a string compare
		{"v0.1.0-beta", "v0.1.0-alpha", true},
		{"v0.1.0-alpha", "v0.1.0-beta", false},
		{"v0.1.0-alpha.1", "v0.1.0-alpha", true}, // more fields wins when equal so far
		{"v0.1.0-alpha", "v0.1.0-alpha.1", false},
		{"v0.1.0-alpha.1", "v0.1.0-alpha.1", false}, // identical is not newer
		{"v0.1.0-alpha", "v0.1.0-1", true},          // numeric ranks below alphanumeric
		{"v0.2.0-alpha.1", "v0.1.0", true},          // the numeric part still decides first

		// Build metadata carries no precedence.
		{"v0.1.0+build.5", "v0.1.0", false},
		{"v0.1.1+build.5", "v0.1.0", true},

		// Anything unparseable must not read as newer: this decides whether a
		// binary gets overwritten, and "do nothing" is the safe direction.
		{"garbage", "v0.1.0", false},
		{"v0.1.0", "garbage", false},
		{"v1.2", "v0.1.0", false},
		{"1.2.3", "v0.1.0", false}, // no v prefix
		{"v1.2.3.4", "v0.1.0", false},
		{"v1.x.0", "v0.1.0", false},
		{"", "v0.1.0", false},
		{"v..", "v0.1.0", false},
		{"v1.2.3-", "v0.1.0", false},     // empty prerelease
		{"v1.2.3-a..b", "v0.1.0", false}, // empty identifier
	}
	for _, c := range cases {
		if got := Newer(c.a, c.b); got != c.want {
			t.Errorf("Newer(%q, %q) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
}

func TestAssetNameMatchesReleaseNaming(t *testing.T) {
	// The workflow packages "pylon-<tag>-<target>.<ext>"; drifting from that
	// silently makes every update fail to find its build.
	got := AssetName("v1.2.3")
	if got == "" {
		t.Fatal("empty asset name")
	}
	for _, want := range []string{"pylon-", "v1.2.3"} {
		if !strings.Contains(got, want) {
			t.Errorf("AssetName = %q, want it to contain %q", got, want)
		}
	}
}
