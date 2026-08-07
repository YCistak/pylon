// Package selfupdate upgrades Pylon in place from its GitHub releases.
//
// An updater runs whatever it downloads, so this one refuses to run anything it
// cannot prove came from a real release: every release publishes a SHA256SUMS
// file signed with the project's ed25519 key, and the public half is compiled
// in below. An asset is only unpacked once its hash matches a line in a
// SHA256SUMS whose signature verifies.
//
// What that does not cover: the signing key lives in a CI secret, so whoever
// fully owns the repository owns the key too and can sign anything. This raises
// the bar to that — a leaked token with only release-write scope, a tampered
// asset, a bad mirror — rather than to nothing. It is not a defence against the
// project's own account being taken over.
//
// Installs owned by a package manager are left alone (see Packaged): pacman and
// friends own their files, and writing under them would be undone on the next
// upgrade at best.
package selfupdate

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"runtime"
	"strings"
	"time"
)

// publicKey verifies release signatures (base64 ed25519, 32 bytes). Generate a
// pair with `go run ./cmd/pylon-sign keygen`, commit the public half here, and
// put the private half in the RELEASE_SIGNING_KEY repository secret.
//
// Empty disables updating entirely rather than accepting unverified downloads:
// an unconfigured project must not be a softer target than a configured one.
// A var only so the tests can install a throwaway key; nothing outside this
// package can reach it.
var publicKey = "5KVJP2O/qZgWZ5mFrowyouDhdvil2wNI+RS1B4WAbxc="

// latestURL is the GitHub API for the newest published release. Drafts and
// prereleases are excluded by the endpoint itself, which is what we want — a
// draft is not ready for anyone to run.
const latestURL = "https://api.github.com/repos/YCistak/pylon/releases/latest"

// Release is a published release relevant to this platform.
type Release struct {
	Version string // tag, e.g. "v0.2.0"
	Notes   string
	assets  map[string]string // asset name → download URL
}

// AssetName is the archive this platform installs from. It mirrors the naming
// the release workflow packages with.
func AssetName(version string) string {
	switch runtime.GOOS {
	case "darwin":
		return fmt.Sprintf("pylon-%s-macos-universal.zip", version)
	case "windows":
		return fmt.Sprintf("pylon-%s-windows-amd64.zip", version)
	default:
		return fmt.Sprintf("pylon-%s-linux-amd64.tar.gz", version)
	}
}

type ghRelease struct {
	TagName string `json:"tag_name"`
	Body    string `json:"body"`
	Assets  []struct {
		Name string `json:"name"`
		URL  string `json:"browser_download_url"`
	} `json:"assets"`
}

// Client fetches release metadata and assets.
type Client struct {
	HTTP    *http.Client
	BaseURL string // overrides latestURL in tests
}

// NewClient builds a Client with a bounded timeout.
func NewClient() *Client {
	return &Client{HTTP: &http.Client{Timeout: 30 * time.Second}}
}

// Check reports the latest release when it is newer than current. ok is false
// when Pylon is already up to date, when the build has no version to compare
// (a `go build` with no ldflags reports "dev"), or when updating is disabled.
func (c *Client) Check(ctx context.Context, current string) (Release, bool, error) {
	if publicKey == "" {
		return Release{}, false, ErrUpdatesDisabled
	}
	if current == "" || current == "dev" {
		// A dev build has no place on the release track: every release would
		// look newer, and replacing a local build with one is never wanted.
		return Release{}, false, ErrDevBuild
	}

	url := c.BaseURL
	if url == "" {
		url = latestURL
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return Release{}, false, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return Release{}, false, fmt.Errorf("check for updates: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Release{}, false, fmt.Errorf("check for updates: github returned %s", resp.Status)
	}

	var gr ghRelease
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&gr); err != nil {
		return Release{}, false, fmt.Errorf("check for updates: %w", err)
	}

	rel := Release{Version: gr.TagName, Notes: gr.Body, assets: map[string]string{}}
	for _, a := range gr.Assets {
		rel.assets[a.Name] = a.URL
	}
	return rel, Newer(gr.TagName, current), nil
}

// Newer reports whether release version a supersedes b. Both are
// "vMAJOR.MINOR.PATCH", optionally with a prerelease suffix (v1.2.3-alpha.1).
// An unparseable version is never treated as newer: the safe direction for a
// comparison that decides whether to overwrite a binary is "do nothing".
func Newer(a, b string) bool {
	av, aok := parse(a)
	bv, bok := parse(b)
	if !aok || !bok {
		return false
	}
	return compare(av, bv) > 0
}

// version is a parsed release version. pre holds the dot-separated prerelease
// identifiers; empty means a final release.
type version struct {
	nums [3]int
	pre  []string
}

// compare orders two versions by semver precedence: -1, 0 or 1.
//
// The prerelease half used to be discarded, which made every v1.0.0-alpha.N
// compare equal to v1.0.0 and to each other. That is not a cosmetic
// inaccuracy: it meant an alpha could never see a later alpha, or the final
// release, as an upgrade — `pylon update` would report "already current" on a
// build the project had moved past.
func compare(a, b version) int {
	for i := range a.nums {
		if a.nums[i] != b.nums[i] {
			return sign(a.nums[i] - b.nums[i])
		}
	}

	// A prerelease precedes the release it leads to: 1.0.0-alpha < 1.0.0.
	switch {
	case len(a.pre) == 0 && len(b.pre) == 0:
		return 0
	case len(a.pre) == 0:
		return 1
	case len(b.pre) == 0:
		return -1
	}

	for i := 0; i < len(a.pre) && i < len(b.pre); i++ {
		if c := compareIdent(a.pre[i], b.pre[i]); c != 0 {
			return c
		}
	}
	// All the shared identifiers matched, so the longer one wins:
	// 1.0.0-alpha < 1.0.0-alpha.1.
	return sign(len(a.pre) - len(b.pre))
}

// compareIdent orders two prerelease identifiers. Numeric ones compare as
// numbers (so alpha.9 < alpha.10, which a string compare gets backwards) and
// always rank below alphanumeric ones.
func compareIdent(a, b string) int {
	an, aNum := atoi(a)
	bn, bNum := atoi(b)
	switch {
	case aNum && bNum:
		return sign(an - bn)
	case aNum:
		return -1
	case bNum:
		return 1
	case a == b:
		return 0
	case a < b:
		return -1
	default:
		return 1
	}
}

func sign(n int) int {
	switch {
	case n > 0:
		return 1
	case n < 0:
		return -1
	default:
		return 0
	}
}

// atoi parses a non-negative decimal identifier, reporting whether it was one.
func atoi(s string) (int, bool) {
	if s == "" {
		return 0, false
	}
	n := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0, false
		}
		n = n*10 + int(r-'0')
	}
	return n, true
}

// parse turns "v1.2.3" or "v1.2.3-alpha.1" into a version. Anything else fails
// rather than guessing.
func parse(v string) (version, bool) {
	var out version
	s, ok := strings.CutPrefix(strings.TrimSpace(v), "v")
	if !ok {
		return out, false
	}
	// Build metadata is ignored by semver precedence, so drop it outright.
	if i := strings.IndexByte(s, '+'); i >= 0 {
		s = s[:i]
	}
	if i := strings.IndexByte(s, '-'); i >= 0 {
		pre := s[i+1:]
		s = s[:i]
		if pre == "" {
			return out, false
		}
		out.pre = strings.Split(pre, ".")
		for _, id := range out.pre {
			if id == "" {
				return out, false
			}
		}
	}

	parts := strings.Split(s, ".")
	if len(parts) != 3 {
		return out, false
	}
	for i, p := range parts {
		n, ok := atoi(p)
		if !ok {
			return out, false
		}
		out.nums[i] = n
	}
	return out, true
}
