package google

import (
	"errors"
	"sync"
	"testing"

	"golang.org/x/oauth2"
)

// fakeSource hands out whatever token it is currently pointed at, standing in
// for oauth2's refresher without touching the network.
type fakeSource struct {
	mu  sync.Mutex
	tok *oauth2.Token
	err error
}

func (f *fakeSource) Token() (*oauth2.Token, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.tok, f.err
}

func (f *fakeSource) set(access string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.tok = &oauth2.Token{AccessToken: access, RefreshToken: "r"}
}

// The bug this exists for: a refreshed access token used to live only in
// memory, so the next daemon start went back to the expired one it had saved.
func TestPersistingSourceSavesRefreshedToken(t *testing.T) {
	src := &fakeSource{}
	src.set("old")

	var saved []string
	p := &persistingSource{
		src:  src,
		save: func(tok *oauth2.Token) error { saved = append(saved, tok.AccessToken); return nil },
		last: "old",
	}

	// Nothing changed yet: reading the cached token must not write the vault.
	if _, err := p.Token(); err != nil {
		t.Fatalf("token: %v", err)
	}
	if len(saved) != 0 {
		t.Fatalf("unchanged token was written: %v", saved)
	}

	src.set("new")
	if _, err := p.Token(); err != nil {
		t.Fatalf("token: %v", err)
	}
	if len(saved) != 1 || saved[0] != "new" {
		t.Fatalf("refreshed token not persisted: %v", saved)
	}

	// Repeated reads of the same refreshed token stay quiet — an API-heavy hour
	// must not mean an encrypted vault rewrite per request.
	for i := 0; i < 3; i++ {
		if _, err := p.Token(); err != nil {
			t.Fatalf("token: %v", err)
		}
	}
	if len(saved) != 1 {
		t.Fatalf("cached token rewritten %d times", len(saved))
	}
}

// A vault that can't be written (read-only home, disk full) must not take the
// calendar down with it: the token in hand is still valid for this process.
func TestPersistingSourceSurvivesSaveFailure(t *testing.T) {
	src := &fakeSource{}
	src.set("new")

	p := &persistingSource{
		src:  src,
		save: func(*oauth2.Token) error { return errors.New("vault yazılamadı") },
		last: "old",
	}

	tok, err := p.Token()
	if err != nil {
		t.Fatalf("save failure leaked to caller: %v", err)
	}
	if tok.AccessToken != "new" {
		t.Fatalf("access token = %q", tok.AccessToken)
	}
}

// A refresh that fails (revoked consent, no network) must surface, not be
// swallowed into a nil token.
func TestPersistingSourcePropagatesRefreshError(t *testing.T) {
	p := &persistingSource{
		src:  &fakeSource{err: errors.New("refresh reddedildi")},
		save: func(*oauth2.Token) error { t.Fatal("failed refresh must not be saved"); return nil },
	}
	if _, err := p.Token(); err == nil {
		t.Fatal("expected the refresh error")
	}
}
