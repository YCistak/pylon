// Package config loads and validates the Pylon configuration (pylon.yaml).
//
// Every field has a sensible default, so a missing or partial file still yields
// a usable Config. Values of the form ${ENV_VAR} are expanded from the
// environment at load time, keeping secrets (API keys, tokens) out of the file.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/YCistak/pylon/internal/ipc"
)

// Config is the fully-resolved Pylon configuration.
type Config struct {
	Voice   Voice   `yaml:"voice"`
	Intent  Intent  `yaml:"intent"`
	Persona Persona `yaml:"persona"`

	Briefing Briefing `yaml:"briefing"`
	Work     Work     `yaml:"work"`
	Services Services `yaml:"services"`

	WatchProcesses []WatchProcess `yaml:"watch_processes"`

	// Paths are runtime locations, overridable for tests / packaging.
	Paths Paths `yaml:"paths"`
}

// Voice holds STT/TTS and audio I/O settings. STT/TTS run as subprocesses
// (whisper.cpp / piper) so the daemon stays CGo-free and cross-compiles. Audio
// capture/playback commands default per-OS (see internal/voice) and are fully
// overridable here; "{file}" and "{seconds}" placeholders are substituted.
type Voice struct {
	STTBin   string `yaml:"stt_bin"`   // whisper.cpp CLI binary (e.g. whisper-cli)
	STTModel string `yaml:"stt_model"` // ggml model path (e.g. ggml-large-v3.bin)
	Language string `yaml:"language"`  // "auto" (detect) or ISO code: tr, en, ...

	// TTSCmd is the synthesis command: receives the text on stdin and must write
	// a WAV to the "{file}" placeholder. Engine-agnostic — piper, an XTTS server
	// client (curl), espeak, etc. Empty disables spoken replies.
	TTSCmd []string `yaml:"tts_cmd"`

	RecordCmd     []string `yaml:"record_cmd"`     // empty → per-OS default
	RecordSeconds int      `yaml:"record_seconds"` // push-to-talk capture window
	PlayCmd       []string `yaml:"play_cmd"`       // empty → per-OS default

	Hotkey string `yaml:"hotkey"` // informational; bind `pylon listen` in your DE/OS
}

// Intent configures the local router and the LLM fallback chain.
type Intent struct {
	RouterThreshold float64     `yaml:"router_threshold"` // local match confidence to skip the API
	Models          []ModelSpec `yaml:"models"`           // fallback chain, tried in order
}

// ModelSpec is one entry in the intent fallback chain. Models are tried in the
// order listed; when one hits its quota (HTTP 429) or errors, the next is used.
// Mixing providers lets the chain spread load across independent quota buckets.
type ModelSpec struct {
	Provider  string `yaml:"provider"`           // "gemini" | "openai" | "anthropic"
	Model     string `yaml:"model"`              // provider-specific model id
	APIKeyEnv string `yaml:"api_key_env"`        // env var name holding the key
	BaseURL   string `yaml:"base_url,omitempty"` // optional endpoint override
}

// Persona configures the local style-learning engine.
type Persona struct {
	Enabled           bool    `yaml:"enabled"`
	DecayHalfLifeDays float64 `yaml:"decay_half_life_days"`     // how fast old style fades
	AdoptThreshold    float64 `yaml:"adopt_threshold"`          // min weighted frequency to adopt a trait
	StyleCardRefreshN int     `yaml:"style_card_refresh_every"` // rebuild style card every N messages
}

// Services configures external integrations (Phase 2+).
type Services struct {
	Google   GoogleService   `yaml:"google"`
	GitHub   GitHubService   `yaml:"github"`
	FreshRSS FreshRSSService `yaml:"freshrss"`
	Spotify  SpotifyService  `yaml:"spotify"`
	Docker   DockerService   `yaml:"docker"`
	Weather  WeatherService  `yaml:"weather"`
}

// WeatherService sets the location the weather service reports for. It uses
// Open-Meteo (no API key); an unset location defaults to İstanbul. Find your
// coordinates at e.g. https://open-meteo.com/en/docs (geocoding) or any map.
type WeatherService struct {
	Latitude  float64 `yaml:"latitude"`
	Longitude float64 `yaml:"longitude"`
	Name      string  `yaml:"name"` // spoken place name, e.g. "İstanbul"
}

// DockerService lets Pylon observe and control Docker containers on the host
// (Phase 2.6). Zero-config on Linux: when the local Engine socket exists the
// service auto-enables. Socket overrides its path; Host points at a remote
// Engine (http[s] base URL) and Token is sent as a Bearer header for a
// proxy-fronted Engine. ${ENV} is expanded and secret: refs resolve.
type DockerService struct {
	Socket string `yaml:"socket"` // default /var/run/docker.sock
	Host   string `yaml:"host"`   // remote Engine base URL, overrides socket
	Token  string `yaml:"token"`  // optional Bearer token for a fronted Engine
}

// FreshRSSService configures a FreshRSS instance via its Fever API (Phase 2.3).
// Provide api_key directly, or username + api_password (the Fever key is
// md5("username:api_password")). ${ENV} is expanded, so keep secrets in the env.
type FreshRSSService struct {
	URL         string `yaml:"url"`
	Username    string `yaml:"username"`
	APIPassword string `yaml:"api_password"`
	APIKey      string `yaml:"api_key"`
}

// GitHubService configures GitHub access (Phase 2.2) via a Personal Access
// Token. The config loader expands ${ENV}, so the token can live in the
// environment (token: ${GITHUB_TOKEN}) rather than in the file. The service is
// enabled once a token is present.
type GitHubService struct {
	Token string `yaml:"token"` // PAT; classic needs repo+read:org, fine-grained PR/issue read

	// Background jobs (scheduler). Empty PollInterval disables the PR poll;
	// empty CommitReminder or no Repos disables the daily commit nudge.
	PollInterval   string   `yaml:"poll_interval"`   // e.g. "15m"
	CommitReminder string   `yaml:"commit_reminder"` // "HH:MM" local
	Repos          []string `yaml:"repos"`           // local git repo paths to check for today's commit
}

// SpotifyService configures Spotify playback control (Phase 2.4) via the Web
// API. The OAuth client is normally embedded in the build (like Google) so end
// users just click "Connect" — client_id here overrides it for self-hosters.
// Authorization Code + PKCE means there is no client secret to configure. The
// user's token lives in the encrypted vault, written by `pylon auth spotify`.
type SpotifyService struct {
	ClientID     string `yaml:"client_id"`     // overrides the embedded OAuth client
	RedirectPort int    `yaml:"redirect_port"` // must match the app's registered redirect URI
}

// GoogleService holds Google OAuth settings (Calendar, later Drive). The OAuth
// client is normally embedded in the build so end users just sign in; client_id/
// client_secret (or a credentials file) override it for self-hosters. The user's
// token lives in the encrypted vault, written by `pylon auth google`.
type GoogleService struct {
	ClientID     string `yaml:"client_id"`     // overrides the embedded OAuth client
	ClientSecret string `yaml:"client_secret"` // desktop apps: not confidential
	Credentials  string `yaml:"credentials"`   // optional: OAuth client JSON instead
	CalendarID   string `yaml:"calendar_id"`   // "primary" by default
}

// Briefing configures the daily briefing: when it runs and how it is presented
// on the desktop. BannerCmd receives the briefing text on stdin and shows it as
// an on-screen banner (see scripts/briefing_banner.py, a Wayland layer-shell
// overlay); empty disables the banner.
type Briefing struct {
	Time      string   `yaml:"time"`       // HH:MM
	Timezone  string   `yaml:"timezone"`   // IANA tz, e.g. Europe/Istanbul
	BannerCmd []string `yaml:"banner_cmd"` // desktop banner command; text on stdin
}

// Work configures session tracking.
type Work struct {
	DailyGoalHours float64  `yaml:"daily_goal_hours"`
	TrackedApps    []string `yaml:"tracked_apps"`
}

// WatchProcess is a single watched process entry.
type WatchProcess struct {
	Name        string `yaml:"name"`
	TasksOnExit bool   `yaml:"tasks_on_exit"`
}

// Paths holds runtime file locations.
type Paths struct {
	Socket string `yaml:"socket"` // Unix socket
	PID    string `yaml:"pid"`    // PID file
	DB     string `yaml:"db"`     // SQLite database
}

// Default returns a Config populated with built-in defaults.
func Default() Config {
	return Config{
		Voice: Voice{
			STTBin:        "whisper-cli",
			Language:      "auto",
			RecordSeconds: 5,
			Hotkey:        "super+p",
			// STTModel + TTSCmd are user-specific (no default); RecordCmd/PlayCmd
			// empty → internal/voice fills the per-OS default.
		},
		Intent: Intent{
			RouterThreshold: 0.8,
			// Default chain: flash first (fast, consistent ~2s), flash-lite as a
			// fallback. flash-lite's "latest" preview has shown bad tail latency
			// (frequent 15-20s), so it must not be the primary for voice.
			Models: []ModelSpec{
				{Provider: "gemini", Model: "gemini-flash-latest", APIKeyEnv: "GEMINI_API_KEY"},
				{Provider: "gemini", Model: "gemini-flash-lite-latest", APIKeyEnv: "GEMINI_API_KEY"},
			},
		},
		Persona: Persona{
			Enabled:           true,
			DecayHalfLifeDays: 14,
			AdoptThreshold:    0.3,
			StyleCardRefreshN: 20,
		},
		Services: Services{
			Google: GoogleService{
				Credentials: "~/.config/pylon/google-credentials.json",
				CalendarID:  "primary",
			},
			Spotify: SpotifyService{
				RedirectPort: 8888,
			},
		},
		Briefing: Briefing{
			Time:     "08:00",
			Timezone: "Europe/Istanbul",
		},
		Work: Work{
			DailyGoalHours: 4,
			TrackedApps:    []string{"code", "cs2", "steam"},
		},
		WatchProcesses: []WatchProcess{
			{Name: "code", TasksOnExit: true},
			{Name: "cs2", TasksOnExit: true},
		},
		Paths: Paths{
			Socket: ipc.DefaultSocketPath(),
			PID:    ipc.DefaultPIDPath(),
			DB:     defaultDBPath(),
		},
	}
}

// Load reads config from path, applying defaults for anything omitted. A missing
// file is not an error — defaults are returned. ${ENV} references are expanded.
func Load(path string) (Config, error) {
	cfg := Default()

	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return cfg, fmt.Errorf("read config %s: %w", path, err)
	}

	expanded := os.ExpandEnv(string(raw))
	if err := yaml.Unmarshal([]byte(expanded), &cfg); err != nil {
		return cfg, fmt.Errorf("parse config %s: %w", path, err)
	}

	if err := cfg.Validate(); err != nil {
		return cfg, err
	}
	return cfg, nil
}

// Validate checks for obviously broken values.
func (c Config) Validate() error {
	if c.Intent.RouterThreshold < 0 || c.Intent.RouterThreshold > 1 {
		return fmt.Errorf("intent.router_threshold must be in [0,1], got %v", c.Intent.RouterThreshold)
	}
	if c.Persona.AdoptThreshold < 0 || c.Persona.AdoptThreshold > 1 {
		return fmt.Errorf("persona.adopt_threshold must be in [0,1], got %v", c.Persona.AdoptThreshold)
	}
	if c.Persona.DecayHalfLifeDays <= 0 {
		return fmt.Errorf("persona.decay_half_life_days must be > 0, got %v", c.Persona.DecayHalfLifeDays)
	}
	if _, err := time.LoadLocation(c.Briefing.Timezone); err != nil {
		return fmt.Errorf("briefing.timezone %q invalid: %w", c.Briefing.Timezone, err)
	}
	if c.Paths.Socket == "" {
		return fmt.Errorf("paths.socket must not be empty")
	}
	return nil
}

// defaultDBPath returns ~/.local/share/pylon/pylon.db, falling back to /tmp.
func defaultDBPath() string {
	if dir, err := os.UserConfigDir(); err == nil {
		return filepath.Join(dir, "pylon", "pylon.db")
	}
	// No user config dir (stripped account, no profile): fall back to the
	// platform temp dir rather than a hardcoded /tmp, which Windows lacks.
	return filepath.Join(os.TempDir(), "pylon.db")
}
