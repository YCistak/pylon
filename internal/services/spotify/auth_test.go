package spotify

import (
	"testing"

	"golang.org/x/oauth2"
)

type fakeSource struct{ tok *oauth2.Token }

func (f *fakeSource) Token() (*oauth2.Token, error) { return f.tok, nil }

// Spotify access tokens expire after an hour, so any long-lived daemon is
// running on a refreshed token. It has to reach the vault, or every restart
// pays a refresh before the first play/pause.
func TestPersistingSourceSavesRefreshedToken(t *testing.T) {
	var saved []string
	p := &persistingSource{
		src:  &fakeSource{tok: &oauth2.Token{AccessToken: "new", RefreshToken: "r"}},
		save: func(tok *oauth2.Token) error { saved = append(saved, tok.AccessToken); return nil },
		last: "old",
	}

	if _, err := p.Token(); err != nil {
		t.Fatalf("token: %v", err)
	}
	if _, err := p.Token(); err != nil {
		t.Fatalf("token: %v", err)
	}

	if len(saved) != 1 || saved[0] != "new" {
		t.Fatalf("expected exactly one write of the new token, got %v", saved)
	}
}
