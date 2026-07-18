package selfupdate

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"testing"
)

// testKey installs a throwaway signing key for the duration of a test and
// returns a signer for it.
func testKey(t *testing.T) func([]byte) []byte {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	old := publicKey
	publicKey = base64.StdEncoding.EncodeToString(pub)
	t.Cleanup(func() { publicKey = old })
	return func(msg []byte) []byte {
		return []byte(base64.StdEncoding.EncodeToString(ed25519.Sign(priv, msg)))
	}
}

func sumsFor(files map[string][]byte) []byte {
	var out []byte
	for name, data := range files {
		sum := sha256.Sum256(data)
		out = append(out, fmt.Sprintf("%s  %s\n", hex.EncodeToString(sum[:]), name)...)
	}
	return out
}

func TestVerifySumsAcceptsSignedList(t *testing.T) {
	sign := testKey(t)
	sums := sumsFor(map[string][]byte{"pylon-v1.0.0-linux-amd64.tar.gz": []byte("archive")})

	got, err := verifySums(sums, sign(sums))
	if err != nil {
		t.Fatalf("verifySums: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("parsed %d checksums, want 1", len(got))
	}
}

// The point of the whole package: nothing that fails to verify may be installed.
func TestVerifySumsRejects(t *testing.T) {
	sign := testKey(t)
	good := sumsFor(map[string][]byte{"asset.tar.gz": []byte("archive")})

	tests := []struct {
		name string
		sums []byte
		sig  []byte
		want error
	}{
		{
			// The attack this exists to stop: swap the artifact, rewrite its
			// hash, keep the old signature.
			name: "tampered checksum list",
			sums: sumsFor(map[string][]byte{"asset.tar.gz": []byte("evil")}),
			sig:  sign(good),
			want: ErrBadSignature,
		},
		{
			name: "signature from another key",
			sums: good,
			sig:  signWithForeignKey(t, good),
			want: ErrBadSignature,
		},
		{
			name: "signature not base64",
			sums: good,
			sig:  []byte("!!!not base64!!!"),
			want: ErrBadSignature,
		},
		{
			name: "empty signature",
			sums: good,
			sig:  nil,
			want: ErrBadSignature,
		},
		{
			name: "signed but malformed list",
			sums: []byte("not-a-hash  asset.tar.gz\n"),
			sig:  sign([]byte("not-a-hash  asset.tar.gz\n")),
			want: ErrBadSignature,
		},
		{
			name: "signed but empty list",
			sums: []byte("\n"),
			sig:  sign([]byte("\n")),
			want: ErrBadSignature,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := verifySums(tc.sums, tc.sig); !errors.Is(err, tc.want) {
				t.Fatalf("verifySums err = %v, want %v", err, tc.want)
			}
		})
	}
}

// A build with no key must refuse to verify anything, not wave it through.
func TestVerifySumsFailsClosedWithoutKey(t *testing.T) {
	old := publicKey
	publicKey = ""
	t.Cleanup(func() { publicKey = old })

	if _, err := verifySums([]byte("x"), []byte("y")); !errors.Is(err, ErrUpdatesDisabled) {
		t.Fatalf("err = %v, want ErrUpdatesDisabled", err)
	}
}

func TestCheckAsset(t *testing.T) {
	sign := testKey(t)
	data := []byte("archive")
	list := sumsFor(map[string][]byte{"a.tar.gz": data})
	parsed, err := verifySums(list, sign(list))
	if err != nil {
		t.Fatal(err)
	}

	if err := checkAsset(parsed, "a.tar.gz", data); err != nil {
		t.Errorf("matching asset rejected: %v", err)
	}
	if err := checkAsset(parsed, "a.tar.gz", []byte("swapped")); !errors.Is(err, ErrChecksumMismatch) {
		t.Errorf("swapped asset err = %v, want ErrChecksumMismatch", err)
	}
	// An asset nobody signed for is unverifiable, which is unverified.
	if err := checkAsset(parsed, "unlisted.tar.gz", data); !errors.Is(err, ErrChecksumMismatch) {
		t.Errorf("unlisted asset err = %v, want ErrChecksumMismatch", err)
	}
}

func signWithForeignKey(t *testing.T, msg []byte) []byte {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return []byte(base64.StdEncoding.EncodeToString(ed25519.Sign(priv, msg)))
}
