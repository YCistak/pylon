// Package secrets stores Pylon's credentials — API keys, tokens, passwords —
// encrypted at rest with AES-256-GCM. No OS keyring, no external daemon: a
// random 32-byte vault key is generated once (`secret.key`, 0600) and the
// secrets themselves live as ciphertext in `secrets.json` (0600), both under
// the user config dir. The user saves a secret once (the `pylon secret` CLI
// now, a settings UI later); the daemon decrypts at runtime.
//
// Config refers to a secret by name with a "secret:<name>" value, e.g.
// `api_password: secret:freshrss`. Resolve turns that into the plaintext.
//
// Threat model: this keeps secrets out of config/git and unreadable at rest
// without the key. The key sits on the same disk (0600), so it protects against
// accidental exposure and casual reads — not an attacker who can read the user's
// home directory. Unattended decryption without a hardware/OS root of trust
// cannot do better; that is the deliberate trade for "save once, runs headless".
package secrets

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// refPrefix marks a config value as an encrypted-secret lookup ("secret:name")
// rather than a literal.
const refPrefix = "secret:"

// ErrNotFound is returned when a secret name has no stored value.
var ErrNotFound = errors.New("secrets: not found")

// Store is the secret backend. The default encrypts to files; tests inject an
// in-memory fake.
type Store interface {
	Set(name, value string) error
	Get(name string) (string, error)
	Delete(name string) error
}

// Default is the process-wide store. Swap it in tests.
var Default Store = NewFileStore("")

// IsRef reports whether a config value is a secret reference ("secret:name").
func IsRef(value string) bool { return strings.HasPrefix(value, refPrefix) }

// RefName returns the secret name inside a "secret:name" reference.
func RefName(value string) string {
	return strings.TrimSpace(strings.TrimPrefix(value, refPrefix))
}

// Set stores (or replaces) a named secret.
func Set(name, value string) error {
	if name = strings.TrimSpace(name); name == "" {
		return errors.New("secrets: empty name")
	}
	return Default.Set(name, value)
}

// Has reports whether a secret with this name is stored. It never returns the
// value, so the GUI can show "saved ✓" without decrypting anything into view.
func Has(name string) bool {
	name = strings.TrimSpace(name)
	if name == "" {
		return false
	}
	_, err := Default.Get(name)
	return err == nil
}

// Delete removes a named secret. Removing a missing one is not an error.
func Delete(name string) error {
	if name = strings.TrimSpace(name); name == "" {
		return errors.New("secrets: empty name")
	}
	if err := Default.Delete(name); err != nil && !errors.Is(err, ErrNotFound) {
		return err
	}
	return nil
}

// Resolve expands a config value: "secret:foo" becomes the decrypted secret
// "foo"; any other value (a literal or an already env-expanded string) is
// returned unchanged. An empty string stays empty.
func Resolve(value string) (string, error) {
	if !IsRef(value) {
		return value, nil
	}
	name := RefName(value)
	if name == "" {
		return "", errors.New("secrets: empty secret reference")
	}
	v, err := Default.Get(name)
	if err != nil {
		return "", fmt.Errorf("secrets: read %q: %w", name, err)
	}
	return v, nil
}

// fileStore encrypts secrets with AES-256-GCM under a local key file.
type fileStore struct {
	mu        sync.Mutex
	keyPath   string
	vaultPath string
}

// NewFileStore builds a file-backed store. An empty dir defaults to the user
// config dir (~/.config/pylon).
func NewFileStore(dir string) *fileStore {
	if dir == "" {
		dir = configDir()
	}
	return &fileStore{
		keyPath:   filepath.Join(dir, "secret.key"),
		vaultPath: filepath.Join(dir, "secrets.json"),
	}
}

func configDir() string {
	if d, err := os.UserConfigDir(); err == nil {
		return filepath.Join(d, "pylon")
	}
	return filepath.Join(os.Getenv("HOME"), ".config", "pylon")
}

func (s *fileStore) Set(name, value string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	aead, err := s.aead()
	if err != nil {
		return err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return err
	}
	// The name is bound in as additional data, so ciphertext can't be moved to
	// another key without failing decryption.
	sealed := aead.Seal(nonce, nonce, []byte(value), []byte(name))

	vault, err := s.load()
	if err != nil {
		return err
	}
	vault[name] = base64.StdEncoding.EncodeToString(sealed)
	return s.save(vault)
}

func (s *fileStore) Get(name string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	vault, err := s.load()
	if err != nil {
		return "", err
	}
	enc, ok := vault[name]
	if !ok {
		return "", ErrNotFound
	}
	sealed, err := base64.StdEncoding.DecodeString(enc)
	if err != nil {
		return "", fmt.Errorf("secrets: corrupt entry %q: %w", name, err)
	}
	aead, err := s.aead()
	if err != nil {
		return "", err
	}
	if len(sealed) < aead.NonceSize() {
		return "", fmt.Errorf("secrets: corrupt entry %q (too short)", name)
	}
	nonce, body := sealed[:aead.NonceSize()], sealed[aead.NonceSize():]
	plain, err := aead.Open(nil, nonce, body, []byte(name))
	if err != nil {
		return "", fmt.Errorf("secrets: decrypt %q: %w", name, err)
	}
	return string(plain), nil
}

func (s *fileStore) Delete(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	vault, err := s.load()
	if err != nil {
		return err
	}
	if _, ok := vault[name]; !ok {
		return ErrNotFound
	}
	delete(vault, name)
	return s.save(vault)
}

// aead builds the AES-256-GCM cipher from the vault key (created on first use).
func (s *fileStore) aead() (cipher.AEAD, error) {
	key, err := s.key()
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

// key returns the 32-byte vault key, generating and persisting it (0600) the
// first time.
func (s *fileStore) key() ([]byte, error) {
	if data, err := os.ReadFile(s.keyPath); err == nil {
		if len(data) != 32 {
			return nil, fmt.Errorf("secrets: key file %s is corrupt (want 32 bytes, got %d)", s.keyPath, len(data))
		}
		return data, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(s.keyPath), 0o700); err != nil {
		return nil, err
	}
	if err := os.WriteFile(s.keyPath, key, 0o600); err != nil {
		return nil, err
	}
	return key, nil
}

func (s *fileStore) load() (map[string]string, error) {
	data, err := os.ReadFile(s.vaultPath)
	if errors.Is(err, os.ErrNotExist) {
		return map[string]string{}, nil
	}
	if err != nil {
		return nil, err
	}
	var vault map[string]string
	if err := json.Unmarshal(data, &vault); err != nil {
		return nil, fmt.Errorf("secrets: vault %s unreadable: %w", s.vaultPath, err)
	}
	if vault == nil {
		vault = map[string]string{}
	}
	return vault, nil
}

func (s *fileStore) save(vault map[string]string) error {
	data, err := json.MarshalIndent(vault, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.vaultPath), 0o700); err != nil {
		return err
	}
	return os.WriteFile(s.vaultPath, data, 0o600)
}
