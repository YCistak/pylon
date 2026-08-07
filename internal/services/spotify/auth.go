package spotify

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os/exec"
	"runtime"
	"sync"
	"time"

	"golang.org/x/oauth2"

	"github.com/YCistak/pylon/internal/secrets"
)

// tokenSecretName is the vault key the user's Spotify OAuth token is stored
// under (internal/secrets — AES-256-GCM, never plaintext on disk).
const tokenSecretName = "spotify-token"

// Config is the resolved Spotify configuration. The OAuth client id is normally
// baked into the build (embeddedClientID) so end users only click "Connect" —
// mirroring the Google flow. Authorization Code + PKCE means no client secret
// is ever needed (public/desktop clients can't keep one confidential anyway).
// The redirect URI must still match a Spotify app's registered Redirect URI
// EXACTLY (Spotify, unlike Google, rejects any-port loopback).
type Config struct {
	ClientID     string // overrides the embedded OAuth client
	RedirectPort int    // loopback port; must match the registered redirect URI
}

// embeddedClientID is baked into release builds via
//
//	-ldflags "-X .../spotify.embeddedClientID=..."
//
// so end users only connect — they never register a Spotify app. (No embedded
// secret: PKCE needs none.)
var embeddedClientID string

const defaultRedirectPort = 8888

// scopes: read playback state + control playback (requires Premium at runtime).
var scopes = []string{"user-read-playback-state", "user-modify-playback-state"}

var spotifyEndpoint = oauth2.Endpoint{
	AuthURL:  "https://accounts.spotify.com/authorize",
	TokenURL: "https://accounts.spotify.com/api/token",
}

// HasClient reports whether an OAuth client is available (config or embedded
// build default) — i.e. `pylon auth spotify` can run.
func HasClient(c Config) bool {
	return firstNonEmpty(c.ClientID, embeddedClientID) != ""
}

// Configured reports whether Spotify is ready to serve commands (an OAuth client
// plus a saved user token). main.go registers the service only when true.
func Configured(c Config) bool {
	if !HasClient(c) {
		return false
	}
	_, err := secrets.Default.Get(tokenSecretName)
	return err == nil
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

func redirectPort(c Config) int {
	if c.RedirectPort > 0 {
		return c.RedirectPort
	}
	return defaultRedirectPort
}

// RedirectURI is the exact URI the user must register in their Spotify app.
func RedirectURI(c Config) string {
	return fmt.Sprintf("http://127.0.0.1:%d/callback", redirectPort(c))
}

func oauthConfig(c Config) *oauth2.Config {
	return &oauth2.Config{
		ClientID:    firstNonEmpty(c.ClientID, embeddedClientID),
		Endpoint:    spotifyEndpoint,
		RedirectURL: RedirectURI(c),
		Scopes:      scopes,
	}
}

// pkcePair generates an RFC 7636 code_verifier (43 random-byte, base64url, no
// padding — well within the 43-128 char range) and its S256 code_challenge.
func pkcePair() (verifier, challenge string, err error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", "", err
	}
	verifier = base64.RawURLEncoding.EncodeToString(raw)
	sum := sha256.Sum256([]byte(verifier))
	challenge = base64.RawURLEncoding.EncodeToString(sum[:])
	return verifier, challenge, nil
}

// Authorize runs the one-time OAuth consent flow (Authorization Code + PKCE) on
// the FIXED redirect port and saves the token. Driven by `pylon auth spotify`.
func Authorize(ctx context.Context, c Config) error {
	cfg := oauthConfig(c)

	verifier, challenge, err := pkcePair()
	if err != nil {
		return fmt.Errorf("pkce: %w", err)
	}

	addr := fmt.Sprintf("127.0.0.1:%d", redirectPort(c))
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("loopback %s dinlenemedi (port meşgul olabilir): %w", addr, err)
	}
	defer ln.Close()

	state := fmt.Sprintf("pylon-%d", time.Now().UnixNano())
	authURL := cfg.AuthCodeURL(state,
		oauth2.SetAuthURLParam("show_dialog", "true"),
		oauth2.SetAuthURLParam("code_challenge_method", "S256"),
		oauth2.SetAuthURLParam("code_challenge", challenge))

	codeCh := make(chan string, 1)
	errCh := make(chan error, 1)
	var once sync.Once
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("code") == "" && q.Get("error") == "" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if e := q.Get("error"); e != "" {
			http.Error(w, "Pylon: yetki reddedildi — "+e, http.StatusBadRequest)
			once.Do(func() { errCh <- fmt.Errorf("oauth reddedildi: %s", e) })
			return
		}
		if q.Get("state") != state {
			http.Error(w, "state mismatch", http.StatusBadRequest)
			once.Do(func() { errCh <- fmt.Errorf("oauth state uyuşmazlığı") })
			return
		}
		fmt.Fprintln(w, "Pylon: Spotify bağlandı ✓ — bu sekmeyi kapatabilirsin.")
		once.Do(func() { codeCh <- q.Get("code") })
	})}
	go srv.Serve(ln)
	defer srv.Close()

	fmt.Printf("Spotify uygulamanın Redirect URI'sine şunu eklemiş ol: %s\n", RedirectURI(c))
	fmt.Println("Tarayıcıda Spotify izni açılıyor. Açılmazsa şu adrese git:")
	fmt.Println(" ", authURL)
	openBrowser(authURL)

	var code string
	select {
	case code = <-codeCh:
	case err = <-errCh:
		return err
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(3 * time.Minute):
		return fmt.Errorf("oauth: zaman aşımı (3 dk)")
	}

	tok, err := cfg.Exchange(ctx, code, oauth2.SetAuthURLParam("code_verifier", verifier))
	if err != nil {
		return fmt.Errorf("token exchange: %w", err)
	}
	return saveToken(tok)
}

// persistingSource writes a refreshed token back to the vault. oauth2 refreshes
// in memory only, so the fresh token would otherwise die with the process. That
// matters more here than for Google: Spotify access tokens last an hour, so a
// daemon that has been up a while is always running on a refreshed token that
// was never saved.
type persistingSource struct {
	src  oauth2.TokenSource
	save func(*oauth2.Token) error

	mu   sync.Mutex
	last string // access token last written, so a cached token costs no write
}

func (p *persistingSource) Token() (*oauth2.Token, error) {
	tok, err := p.src.Token()
	if err != nil {
		return nil, err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if tok.AccessToken != p.last {
		p.last = tok.AccessToken
		// Best effort: a vault write failure must not fail the playback command
		// that triggered the refresh.
		_ = p.save(tok)
	}
	return tok, nil
}

// httpClient returns an OAuth client using the saved token (auto-refresh,
// persisted).
func httpClient(ctx context.Context, c Config) (*http.Client, error) {
	tok, err := loadToken()
	if err != nil {
		return nil, fmt.Errorf("spotify: kayıtlı token yok — önce `pylon auth spotify` çalıştır (%v)", err)
	}
	return oauth2.NewClient(ctx, &persistingSource{
		src:  oauthConfig(c).TokenSource(ctx, tok),
		save: saveToken,
		last: tok.AccessToken,
	}), nil
}

// Logout forgets the user's token, so Configured reports false and the next
// connect asks for consent again. Signing out when nothing is stored is not an
// error.
func Logout() error { return secrets.Delete(tokenSecretName) }

// --- token/browser helpers (kept local so the package is self-contained) ---

// loadToken reads the user's token from the encrypted vault.
func loadToken() (*oauth2.Token, error) {
	data, err := secrets.Default.Get(tokenSecretName)
	if err != nil {
		return nil, err
	}
	var t oauth2.Token
	if err := json.Unmarshal([]byte(data), &t); err != nil {
		return nil, err
	}
	return &t, nil
}

// saveToken persists the user's token in the encrypted vault (AES-256-GCM).
func saveToken(t *oauth2.Token) error {
	data, err := json.Marshal(t)
	if err != nil {
		return err
	}
	return secrets.Default.Set(tokenSecretName, string(data))
}

func openBrowser(url string) {
	var cmd string
	var args []string
	switch runtime.GOOS {
	case "darwin":
		cmd = "open"
	case "windows":
		cmd, args = "rundll32", []string{"url.dll,FileProtocolHandler"}
	default:
		cmd = "xdg-open"
	}
	_ = exec.Command(cmd, append(args, url)...).Start()
}
