package selfupdate

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
)

// Sentinel errors callers act on.
var (
	// ErrUpdatesDisabled means the build carries no signing key, so nothing can
	// be verified and nothing will be installed.
	ErrUpdatesDisabled = errors.New("updates are disabled in this build (no signing key)")
	// ErrDevBuild means the running binary has no release version.
	ErrDevBuild = errors.New("this is a development build, not a release")
	// ErrPackaged means a package manager owns this install.
	ErrPackaged = errors.New("this copy is managed by a package manager")
	// ErrBadSignature means SHA256SUMS was not signed by the project key.
	ErrBadSignature = errors.New("release signature does not verify")
	// ErrChecksumMismatch means the download does not match the signed hash.
	ErrChecksumMismatch = errors.New("download does not match its signed checksum")
)

// verifySums checks that sums was signed by publicKey and returns the parsed
// "hash  name" lines. Every later check leans on this one, so it fails closed:
// no signature, no map.
func verifySums(sums, sig []byte) (map[string]string, error) {
	pub, err := base64.StdEncoding.DecodeString(publicKey)
	if err != nil || len(pub) != ed25519.PublicKeySize {
		// A malformed compiled-in key is a build mistake, not a runtime state
		// to route around.
		return nil, fmt.Errorf("%w: bad public key in this build", ErrUpdatesDisabled)
	}

	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(sig)))
	if err != nil {
		return nil, fmt.Errorf("%w: signature is not base64", ErrBadSignature)
	}
	if !ed25519.Verify(ed25519.PublicKey(pub), sums, raw) {
		return nil, ErrBadSignature
	}

	out := map[string]string{}
	for _, line := range strings.Split(string(sums), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// sha256sum's format: "<hex>  <name>", two spaces. Split on any run of
		// spaces so a single-space variant still parses.
		f := strings.Fields(line)
		if len(f) != 2 {
			return nil, fmt.Errorf("%w: malformed checksum line %q", ErrBadSignature, line)
		}
		if _, err := hex.DecodeString(f[0]); err != nil || len(f[0]) != 64 {
			return nil, fmt.Errorf("%w: malformed hash %q", ErrBadSignature, f[0])
		}
		out[strings.TrimPrefix(f[1], "*")] = strings.ToLower(f[0])
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("%w: no checksums listed", ErrBadSignature)
	}
	return out, nil
}

// checkAsset verifies data against the signed hash recorded for name.
func checkAsset(sums map[string]string, name string, data []byte) error {
	want, ok := sums[name]
	if !ok {
		// An asset absent from the signed list is unverifiable, which is the
		// same as unverified.
		return fmt.Errorf("%w: %s is not in the signed checksums", ErrChecksumMismatch, name)
	}
	sum := sha256.Sum256(data)
	if got := hex.EncodeToString(sum[:]); got != want {
		return fmt.Errorf("%w: %s", ErrChecksumMismatch, name)
	}
	return nil
}
