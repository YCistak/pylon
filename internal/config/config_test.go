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
	if len(cfg.Intent.Models) == 0 || cfg.Intent.Models[0].Model != "gemini-flash-lite-latest" {
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
