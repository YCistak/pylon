// Package google integrates Google services (Calendar) into Pylon via OAuth2.
package google

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	calendar "google.golang.org/api/calendar/v3"
	drive "google.golang.org/api/drive/v3"

	"github.com/YCistak/pylon/internal/i18n"
	"github.com/YCistak/pylon/internal/secrets"
)

// tokenSecretName is the vault key the user's Google OAuth token is stored
// under (internal/secrets — AES-256-GCM, never plaintext on disk).
const tokenSecretName = "google-token"

// Config is the resolved Google service configuration. The OAuth *client* (which
// app this is) comes from ClientID/Secret or a credentials file or build-time
// embedded defaults; the *token* (which user) lives in the encrypted vault.
type Config struct {
	ClientID        string // OAuth client id (overrides the embedded default)
	ClientSecret    string // OAuth client secret (desktop apps: not confidential)
	CredentialsPath string // optional: OAuth client JSON instead of id/secret
	CalendarID      string // "primary" by default
}

// embeddedClientID/Secret are baked into release builds via
//
//	-ldflags "-X .../google.embeddedClientID=... -X .../google.embeddedClientSecret=..."
//
// so end users only sign in — they never register an app or touch Google Cloud.
var (
	embeddedClientID     string
	embeddedClientSecret string
)

// scopes: read + write calendar events, and read-only Drive file metadata
// (search + link). Adding a scope means the user must re-run `pylon auth google`
// to grant it — an existing calendar-only token keeps working for calendar but
// Drive calls fail until re-consent.
var scopes = []string{
	calendar.CalendarEventsScope,
	drive.DriveMetadataReadonlyScope,
}

// HasClient reports whether an OAuth client is available (config, credentials
// file, or embedded build default) — i.e. `pylon auth google` can run.
func HasClient(c Config) bool {
	if c.ClientID != "" || embeddedClientID != "" {
		return true
	}
	if c.CredentialsPath != "" {
		if _, err := os.Stat(expandHome(c.CredentialsPath)); err == nil {
			return true
		}
	}
	return false
}

// Configured reports whether Google is ready to serve commands (an OAuth client
// plus a saved user token). main.go registers the service only when true.
func Configured(c Config) bool {
	if !HasClient(c) {
		return false
	}
	_, err := secrets.Default.Get(tokenSecretName)
	return err == nil
}

// oauthConfig resolves the OAuth client: explicit id/secret first, then a
// credentials JSON file, then the build-time embedded default.
func oauthConfig(c Config) (*oauth2.Config, error) {
	if id := firstNonEmpty(c.ClientID, embeddedClientID); id != "" {
		return &oauth2.Config{
			ClientID:     id,
			ClientSecret: firstNonEmpty(c.ClientSecret, embeddedClientSecret),
			Endpoint:     google.Endpoint,
			Scopes:       scopes,
		}, nil
	}
	if c.CredentialsPath != "" {
		data, err := os.ReadFile(expandHome(c.CredentialsPath))
		if err == nil {
			return google.ConfigFromJSON(data, scopes...)
		}
	}
	return nil, errors.New("google: no OAuth client configured (no client_id built into Pylon)")
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

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

// persistingSource writes a refreshed token back to the vault. oauth2 refreshes
// the access token in memory only, so without this the fresh token dies with the
// process: every daemon restart starts from an expired one and pays a refresh
// round-trip before the first calendar call. Worse, when the provider rotates
// the refresh token the stored copy goes stale and the user is silently signed
// out until they run `pylon auth google` again.
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
		// Best effort: a vault write failure must not fail the API call that
		// triggered the refresh — the token is valid for this process either way.
		_ = p.save(tok)
	}
	return tok, nil
}

// httpClient returns an authorized client using the saved token (auto-refresh,
// persisted). Errors with a hint when no token exists.
func httpClient(ctx context.Context, c Config) (*http.Client, error) {
	cfg, err := oauthConfig(c)
	if err != nil {
		return nil, err
	}
	tok, err := loadToken()
	if err != nil {
		return nil, fmt.Errorf("google: no saved token — run `pylon auth google` first (%v)", err)
	}
	return oauth2.NewClient(ctx, &persistingSource{
		src:  cfg.TokenSource(ctx, tok),
		save: saveToken,
		last: tok.AccessToken,
	}), nil
}

// Authorize runs the one-time OAuth consent flow via a loopback redirect and
// saves the resulting token. Driven by `pylon auth google`.
func Authorize(ctx context.Context, c Config) error {
	cfg, err := oauthConfig(c)
	if err != nil {
		return err
	}

	// Loopback server on a free port catches the redirect with the auth code.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return fmt.Errorf("start loopback listener: %w", err)
	}
	defer ln.Close()
	cfg.RedirectURL = fmt.Sprintf("http://%s/callback", ln.Addr().String())

	state := fmt.Sprintf("pylon-%d", time.Now().UnixNano())
	// Force the account chooser + consent so multi-account sign-ins don't trip
	// the flow (a common "something went wrong" cause in test mode).
	authURL := cfg.AuthCodeURL(state, oauth2.AccessTypeOffline,
		oauth2.SetAuthURLParam("prompt", "select_account consent"))

	codeCh := make(chan string, 1)
	errCh := make(chan error, 1)
	var once sync.Once
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		// Ignore unrelated hits (e.g. /favicon.ico) so they don't trip the flow.
		if q.Get("code") == "" && q.Get("error") == "" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if e := q.Get("error"); e != "" {
			desc := q.Get("error_description")
			http.Error(w, "Pylon: yetki reddedildi — "+e, http.StatusBadRequest)
			once.Do(func() { errCh <- fmt.Errorf("oauth reddedildi: %s %s", e, desc) })
			return
		}
		if q.Get("state") != state {
			http.Error(w, "state mismatch", http.StatusBadRequest)
			once.Do(func() { errCh <- errors.New("oauth state mismatch") })
			return
		}
		fmt.Fprintln(w, i18n.T("auth.google.done"))
		once.Do(func() { codeCh <- q.Get("code") })
	})}
	go srv.Serve(ln)
	defer srv.Close()

	fmt.Println("Tarayıcıda Google izni açılıyor. Açılmazsa şu adrese git:")
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
		return errors.New("oauth: timed out after 3 minutes")
	}

	tok, err := cfg.Exchange(ctx, code)
	if err != nil {
		return fmt.Errorf("token exchange: %w", err)
	}
	if err := saveToken(tok); err != nil {
		return fmt.Errorf("save token: %w", err)
	}
	return nil
}

// Logout forgets the user's token. Configured then reports false, the calendar
// and drive services drop out of the registry on the next daemon start, and the
// next sign-in asks for consent again. Signing out when nothing is stored is
// not an error — the caller asked for an end state, not a transition.
func Logout() error { return secrets.Delete(tokenSecretName) }

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

func expandHome(p string) string {
	if strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, p[2:])
		}
	}
	return p
}
