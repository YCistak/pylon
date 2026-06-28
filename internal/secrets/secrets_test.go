package secrets

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// memStore is an in-memory Store for the package-level Resolve/Set/Delete tests.
type memStore struct {
	m       map[string]string
	failGet bool
}

func newMem() *memStore { return &memStore{m: map[string]string{}} }

func (s *memStore) Set(name, value string) error { s.m[name] = value; return nil }
func (s *memStore) Get(name string) (string, error) {
	if s.failGet {
		return "", errors.New("backend unavailable")
	}
	v, ok := s.m[name]
	if !ok {
		return "", ErrNotFound
	}
	return v, nil
}
func (s *memStore) Delete(name string) error {
	if _, ok := s.m[name]; !ok {
		return ErrNotFound
	}
	delete(s.m, name)
	return nil
}

func withStore(t *testing.T, s Store) {
	t.Helper()
	prev := Default
	Default = s
	t.Cleanup(func() { Default = prev })
}

func TestResolvePlainPassthrough(t *testing.T) {
	withStore(t, newMem())
	for _, in := range []string{"", "literal-token", "${ALREADY_EXPANDED}", "https://x"} {
		if got, err := Resolve(in); err != nil || got != in {
			t.Fatalf("Resolve(%q) = %q,%v want unchanged", in, got, err)
		}
	}
}

func TestResolveRef(t *testing.T) {
	withStore(t, newMem())
	if err := Set("freshrss", "s3cret"); err != nil {
		t.Fatalf("set: %v", err)
	}
	got, err := Resolve("secret:freshrss")
	if err != nil || got != "s3cret" {
		t.Fatalf("Resolve ref = %q,%v", got, err)
	}
}

func TestResolveEmptyRef(t *testing.T) {
	withStore(t, newMem())
	if _, err := Resolve("secret:"); err == nil {
		t.Fatal("empty ref should error")
	}
}

func TestResolveMissing(t *testing.T) {
	withStore(t, newMem())
	if _, err := Resolve("secret:nope"); err == nil {
		t.Fatal("missing secret should error")
	}
}

func TestResolveBackendFailureSurfaces(t *testing.T) {
	m := newMem()
	m.failGet = true
	withStore(t, m)
	if _, err := Resolve("secret:x"); err == nil {
		t.Fatal("backend read failure should surface")
	}
}

func TestSetEmptyName(t *testing.T) {
	withStore(t, newMem())
	if err := Set("  ", "v"); err == nil {
		t.Fatal("empty name should error")
	}
}

func TestDeleteMissingIsOK(t *testing.T) {
	withStore(t, newMem())
	if err := Delete("ghost"); err != nil {
		t.Fatalf("deleting a missing secret should be a no-op, got %v", err)
	}
}

func TestRefHelpers(t *testing.T) {
	if !IsRef("secret:foo") || IsRef("plain") {
		t.Fatal("IsRef")
	}
	if RefName("secret:  foo  ") != "foo" {
		t.Fatalf("RefName trim = %q", RefName("secret:  foo  "))
	}
}

// --- file store: real AES-256-GCM round trip on disk ---

func TestFileStoreRoundTrip(t *testing.T) {
	dir := t.TempDir()
	fs := NewFileStore(dir)

	if err := fs.Set("github", "ghp_TopSecretToken"); err != nil {
		t.Fatalf("set: %v", err)
	}
	got, err := fs.Get("github")
	if err != nil || got != "ghp_TopSecretToken" {
		t.Fatalf("get = %q,%v", got, err)
	}

	// The vault on disk must NOT contain the plaintext, and the key file exists 0600.
	vault, err := os.ReadFile(filepath.Join(dir, "secrets.json"))
	if err != nil {
		t.Fatalf("read vault: %v", err)
	}
	if strings.Contains(string(vault), "ghp_TopSecretToken") {
		t.Fatal("plaintext secret leaked into the vault file")
	}
	keyInfo, err := os.Stat(filepath.Join(dir, "secret.key"))
	if err != nil {
		t.Fatalf("key file missing: %v", err)
	}
	if keyInfo.Mode().Perm() != 0o600 {
		t.Fatalf("key perms = %v want 0600", keyInfo.Mode().Perm())
	}
}

func TestFileStoreReplaceAndDelete(t *testing.T) {
	fs := NewFileStore(t.TempDir())
	if err := fs.Set("k", "v1"); err != nil {
		t.Fatal(err)
	}
	if err := fs.Set("k", "v2"); err != nil {
		t.Fatal(err)
	}
	if got, _ := fs.Get("k"); got != "v2" {
		t.Fatalf("replace = %q want v2", got)
	}
	if err := fs.Delete("k"); err != nil {
		t.Fatal(err)
	}
	if _, err := fs.Get("k"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("after delete err = %v want ErrNotFound", err)
	}
}

func TestFileStoreTamperDetected(t *testing.T) {
	dir := t.TempDir()
	fs := NewFileStore(dir)
	if err := fs.Set("a", "value-a"); err != nil {
		t.Fatal(err)
	}
	// Re-encrypting "a"'s ciphertext under name "b" must fail to decrypt: the
	// name is bound in as additional authenticated data.
	vault, _ := fs.load()
	vault["b"] = vault["a"]
	if err := fs.save(vault); err != nil {
		t.Fatal(err)
	}
	if _, err := fs.Get("b"); err == nil {
		t.Fatal("ciphertext moved to another name should fail to decrypt")
	}
}

func TestFileStorePersistsAcrossInstances(t *testing.T) {
	dir := t.TempDir()
	if err := NewFileStore(dir).Set("k", "persisted"); err != nil {
		t.Fatal(err)
	}
	// A fresh store over the same dir reuses the key file + vault.
	if got, err := NewFileStore(dir).Get("k"); err != nil || got != "persisted" {
		t.Fatalf("reopened get = %q,%v", got, err)
	}
}
