package i18n

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPrefRoundTrips(t *testing.T) {
	dir := t.TempDir()

	if got := LoadPref(dir); got != "" {
		t.Fatalf("LoadPref on a fresh directory = %q, want empty", got)
	}
	if err := SavePref(dir, "de"); err != nil {
		t.Fatalf("SavePref: %v", err)
	}
	if got := LoadPref(dir); got != "de" {
		t.Fatalf("LoadPref after saving = %q, want de", got)
	}
}

// An empty language is how the interface says "follow the system", so it has to
// leave nothing behind — a file holding "" would be read back as a choice.
func TestEmptyPrefRemovesTheFile(t *testing.T) {
	dir := t.TempDir()

	if err := SavePref(dir, "ru"); err != nil {
		t.Fatalf("SavePref: %v", err)
	}
	if err := SavePref(dir, ""); err != nil {
		t.Fatalf("SavePref(empty): %v", err)
	}
	if _, err := os.Stat(PrefPath(dir)); !os.IsNotExist(err) {
		t.Fatalf("the preference file survived clearing: %v", err)
	}
	if got := LoadPref(dir); got != "" {
		t.Fatalf("LoadPref after clearing = %q, want empty", got)
	}
	// Clearing twice is what happens when the user picks "system" while already
	// on it, and must not be an error.
	if err := SavePref(dir, ""); err != nil {
		t.Fatalf("clearing an absent preference: %v", err)
	}
}

// The file is written by Pylon but sits in a directory people edit by hand.
// Whatever ends up in it, the answer is the configured language, not a crash
// and not a language nobody asked for.
func TestGarbageInThePrefFileIsIgnored(t *testing.T) {
	dir := t.TempDir()
	for _, content := range []string{"", "  \n", "klingon", "tr; rm -rf /", "C"} {
		if err := os.WriteFile(PrefPath(dir), []byte(content), 0o600); err != nil {
			t.Fatalf("write: %v", err)
		}
		if got := LoadPref(dir); got != "" {
			t.Errorf("LoadPref(%q) = %q, want empty", content, got)
		}
	}
}

// The same locale shapes SetLanguage accepts, since the value can also arrive
// from a command line: pylon lang pt_BR.
func TestPrefNormalizesLocaleTags(t *testing.T) {
	dir := t.TempDir()
	for tag, want := range map[string]string{
		"pt-BR":       "pt",
		"tr_TR.UTF-8": "tr",
		"EN":          "en",
	} {
		if err := SavePref(dir, tag); err != nil {
			t.Fatalf("SavePref(%q): %v", tag, err)
		}
		if got := LoadPref(dir); got != want {
			t.Errorf("LoadPref after SavePref(%q) = %q, want %q", tag, got, want)
		}
	}
}

func TestSavePrefRejectsALanguageWeDoNotHave(t *testing.T) {
	dir := t.TempDir()
	if err := SavePref(dir, "klingon"); err == nil {
		t.Fatal("SavePref accepted a language with no catalog")
	}
	if _, err := os.Stat(PrefPath(dir)); !os.IsNotExist(err) {
		t.Fatal("a rejected language still wrote a file")
	}
}

func TestSavePrefCreatesTheDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "pylon")
	if err := SavePref(dir, "fr"); err != nil {
		t.Fatalf("SavePref into a missing directory: %v", err)
	}
	if got := LoadPref(dir); got != "fr" {
		t.Fatalf("LoadPref = %q, want fr", got)
	}
}

func TestIsSupportedMatchesSetLanguage(t *testing.T) {
	prev := Language()
	t.Cleanup(func() { SetLanguage(prev) })

	for _, lang := range Supported {
		if !IsSupported(lang) {
			t.Errorf("IsSupported(%q) = false for a shipped language", lang)
		}
	}
	for _, tag := range []string{"", "klingon", "C", "zz"} {
		if IsSupported(tag) {
			t.Errorf("IsSupported(%q) = true", tag)
		}
	}
}

// The picker shows every shipped language, so a missing native name would be a
// row labelled "pt".
func TestEveryLanguageNamesItself(t *testing.T) {
	for _, lang := range Supported {
		name := NativeName(lang)
		if name == "" || name == lang {
			t.Errorf("NativeName(%q) = %q, want the language's own name", lang, name)
		}
	}
}
