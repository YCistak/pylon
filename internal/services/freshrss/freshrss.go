// Package freshrss integrates a FreshRSS instance through its Fever API. Pylon
// reports the unread-item count ("kaç okunmamış haberim var"), and the morning
// briefing (Phase 3) reuses the same count. Auth is the Fever api_key —
// md5("username:apiPassword") — POSTed over HTTP. Like the other services it
// speaks through a small interface so it can be faked in tests.
package freshrss

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/YCistak/pylon/internal/intent"
)

// ActionUnreadCount answers "how many unread items do I have".
const ActionUnreadCount intent.Action = "freshrss.unread_count"

// Config holds FreshRSS/Fever settings. Provide either APIKey directly, or
// Username+APIPassword (the key is md5("username:apiPassword")). The config
// loader expands ${ENV}, so the password/key can live in the environment.
type Config struct {
	URL         string // base instance URL, e.g. https://rss.example.com
	Username    string // FreshRSS user
	APIPassword string // the Fever API password (Settings → Profile → API)
	APIKey      string // precomputed api_key; overrides Username/APIPassword
}

// feverAPI is the slice of the Fever API the service uses; a fake implements it
// in tests.
type feverAPI interface {
	unreadCount(ctx context.Context) (int, error)
}

// FreshRSS is the FreshRSS Service.
type FreshRSS struct {
	cfg Config
	api feverAPI // injected in tests; otherwise built lazily
}

// New builds the service from config. It does not touch the network until first
// use.
func New(cfg Config) *FreshRSS {
	cfg.URL = strings.TrimRight(strings.TrimSpace(cfg.URL), "/")
	return &FreshRSS{cfg: cfg}
}

// apiKey resolves the Fever api_key: an explicit key wins, otherwise it is
// md5("username:apiPassword"). Empty when neither is available.
func apiKey(cfg Config) string {
	if cfg.APIKey != "" {
		return cfg.APIKey
	}
	if cfg.Username == "" || cfg.APIPassword == "" {
		return ""
	}
	sum := md5.Sum([]byte(cfg.Username + ":" + cfg.APIPassword))
	return hex.EncodeToString(sum[:])
}

// Configured reports whether FreshRSS is ready (URL + a resolvable api_key).
// main.go registers the service only when true.
func Configured(cfg Config) bool {
	return strings.TrimSpace(cfg.URL) != "" && apiKey(cfg) != ""
}

func (f *FreshRSS) Name() string { return "freshrss" }

func (f *FreshRSS) Actions() []intent.ActionSpec {
	return []intent.ActionSpec{
		{
			Name: ActionUnreadCount,
			Desc: `"freshrss.unread_count": how many unread RSS items the user has. No args. Use for "kaç okunmamış haberim var", "RSS'te kaç okunmamış var".`,
		},
	}
}

func (f *FreshRSS) Execute(ctx context.Context, action intent.Action, _ map[string]string) (string, error) {
	switch action {
	case ActionUnreadCount:
		n, err := f.UnreadCount(ctx)
		if err != nil {
			return "", err
		}
		if n == 0 {
			return "Okunmamış haberin yok.", nil
		}
		return fmt.Sprintf("%d okunmamış haberin var.", n), nil
	default:
		return "", fmt.Errorf("freshrss: bilinmeyen aksiyon %q", action)
	}
}

// UnreadCount returns the raw unread-item count. Exported so the morning
// briefing (Phase 3) can reuse it without going through the intent layer.
func (f *FreshRSS) UnreadCount(ctx context.Context) (int, error) {
	api, err := f.client()
	if err != nil {
		return 0, err
	}
	return api.unreadCount(ctx)
}

func (f *FreshRSS) client() (feverAPI, error) {
	if f.api != nil {
		return f.api, nil
	}
	key := apiKey(f.cfg)
	if f.cfg.URL == "" || key == "" {
		return nil, fmt.Errorf("freshrss: yapılandırılmamış (url + kimlik gerekli)")
	}
	return &realFever{base: f.cfg.URL, key: key, hc: &http.Client{Timeout: 10 * time.Second}}, nil
}

// realFever calls the Fever API over plain HTTP.
type realFever struct {
	base string
	key  string
	hc   *http.Client
}

func (r *realFever) unreadCount(ctx context.Context) (int, error) {
	endpoint := r.base + "/api/fever.php?api&unread_item_ids"
	form := url.Values{"api_key": {r.key}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := r.hc.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 256))
		return 0, fmt.Errorf("freshrss API %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var out struct {
		Auth          int    `json:"auth"`
		UnreadItemIDs string `json:"unread_item_ids"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return 0, err
	}
	if out.Auth != 1 {
		return 0, fmt.Errorf("freshrss: kimlik doğrulama başarısız (api_key hatalı?)")
	}
	return countIDs(out.UnreadItemIDs), nil
}

// countIDs counts the comma-separated ids the Fever API returns; "" → 0.
func countIDs(s string) int {
	if s = strings.TrimSpace(s); s == "" {
		return 0
	}
	return strings.Count(s, ",") + 1
}
