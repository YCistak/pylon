// Package config loads and validates the Pylon configuration (pylon.yaml).
//
// Every field has a sensible default, so a missing or partial file still yields
// a usable Config. Values of the form ${ENV_VAR} are expanded from the
// environment at load time, keeping secrets (API keys, tokens) out of the file.
package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// Config is the fully-resolved Pylon configuration.
type Config struct {
	Voice   Voice   `yaml:"voice"`
	Intent  Intent  `yaml:"intent"`
	Persona Persona `yaml:"persona"`

	Briefing Briefing `yaml:"briefing"`
	Work     Work     `yaml:"work"`

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

// Briefing configures the morning briefing.
type Briefing struct {
	Time     string `yaml:"time"`     // HH:MM
	Timezone string `yaml:"timezone"` // IANA tz, e.g. Europe/Istanbul
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
			Socket: "/tmp/pylon.sock",
			PID:    "/tmp/pylon.pid",
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
		return dir + "/pylon/pylon.db"
	}
	return "/tmp/pylon.db"
}
