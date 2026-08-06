package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadMissingFileReturnsDefaults(t *testing.T) {
	cfg, err := Load(filepath.Join(t.TempDir(), "nope.yaml"))
	if err != nil {
		t.Fatalf("missing file should not error: %v", err)
	}
	if len(cfg.Intent.Models) == 0 || cfg.Intent.Models[0].Model != "gemini-flash-latest" {
		t.Fatalf("expected default model chain, got %+v", cfg.Intent.Models)
	}
	if cfg.Persona.DecayHalfLifeDays != 14 {
		t.Fatalf("expected default decay 14, got %v", cfg.Persona.DecayHalfLifeDays)
	}
}

func TestLoadPartialOverlaysDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pylon.yaml")
	yaml := `
intent:
  router_threshold: 0.6
  models:
    - provider: openai
      model: gpt-4o-mini
      api_key_env: OPENAI_API_KEY
persona:
  adopt_threshold: 0.5
`
	if err := os.WriteFile(path, []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	// A models list in YAML replaces the default chain.
	if len(cfg.Intent.Models) != 1 || cfg.Intent.Models[0].Provider != "openai" {
		t.Fatalf("models override failed: %+v", cfg.Intent.Models)
	}
	if cfg.Intent.RouterThreshold != 0.6 {
		t.Fatalf("threshold override failed: %v", cfg.Intent.RouterThreshold)
	}
	if cfg.Persona.AdoptThreshold != 0.5 {
		t.Fatalf("adopt override failed: %v", cfg.Persona.AdoptThreshold)
	}
	// Untouched fields keep their defaults.
	if cfg.Persona.StyleCardRefreshN != 20 {
		t.Fatalf("default should remain, got %d", cfg.Persona.StyleCardRefreshN)
	}
}

func TestEnvExpansion(t *testing.T) {
	t.Setenv("PYLON_TEST_DB", "/var/lib/pylon/test.db")
	path := filepath.Join(t.TempDir(), "pylon.yaml")
	if err := os.WriteFile(path, []byte("paths:\n  db: ${PYLON_TEST_DB}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.Paths.DB != "/var/lib/pylon/test.db" {
		t.Fatalf("env not expanded: %q", cfg.Paths.DB)
	}
}

func TestValidateRejectsBadValues(t *testing.T) {
	cases := map[string]func(*Config){
		"threshold>1":  func(c *Config) { c.Intent.RouterThreshold = 1.5 },
		"adopt<0":      func(c *Config) { c.Persona.AdoptThreshold = -0.1 },
		"decay<=0":     func(c *Config) { c.Persona.DecayHalfLifeDays = 0 },
		"bad timezone": func(c *Config) { c.Briefing.Timezone = "Mars/Olympus" },
		"empty socket": func(c *Config) { c.Paths.Socket = "" },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			cfg := Default()
			mutate(&cfg)
			if err := cfg.Validate(); err == nil {
				t.Fatalf("%s: expected validation error", name)
			}
		})
	}
}

// The voice defaults must make push-to-talk end on silence, with record_seconds
// acting as a ceiling rather than a fixed wait.
func TestVoiceSilenceDefaults(t *testing.T) {
	cfg := Default()
	if !cfg.Voice.SilenceStop {
		t.Fatal("silence_stop should default on")
	}
	if cfg.Voice.SilenceSeconds != 1.0 {
		t.Fatalf("silence_seconds = %v", cfg.Voice.SilenceSeconds)
	}
	if cfg.Voice.RecordSeconds != 15 {
		t.Fatalf("record_seconds ceiling = %v", cfg.Voice.RecordSeconds)
	}
	if cfg.Voice.STTServer.Enabled() {
		t.Fatal("no warm server should be configured by default")
	}
}

// pylon.local.yaml sets only bin + port; host must default and the server must
// come out enabled.
func TestVoiceSTTServerPartialConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pylon.yaml")
	yaml := `
voice:
  stt_server:
    bin: /opt/whisper/whisper-server
    port: 8910
`
	if err := os.WriteFile(path, []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if !cfg.Voice.STTServer.Enabled() {
		t.Fatal("stt_server should be enabled when bin is set")
	}
	if got := cfg.Voice.STTServer.Addr(); got != "127.0.0.1:8910" {
		t.Fatalf("addr = %q", got)
	}
}

func TestSTTServerAddrDefaults(t *testing.T) {
	if got := (STTServer{Bin: "x"}).Addr(); got != "127.0.0.1:8910" {
		t.Fatalf("empty host/port addr = %q", got)
	}
	if got := (STTServer{Bin: "x", Host: "0.0.0.0", Port: 9000}).Addr(); got != "0.0.0.0:9000" {
		t.Fatalf("addr = %q", got)
	}
}

// silence_stop: false must survive the overlay onto defaults, which start true.
func TestVoiceSilenceStopCanBeDisabled(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pylon.yaml")
	if err := os.WriteFile(path, []byte("voice:\n  silence_stop: false\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.Voice.SilenceStop {
		t.Fatal("explicit silence_stop: false was overridden by the default")
	}
}
