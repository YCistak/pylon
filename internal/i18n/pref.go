package i18n

// The language a user picked in the interface, remembered between runs.
//
// It is a one-line file rather than a row in the database or a field written
// back into pylon.yaml, and both alternatives were rejected for concrete
// reasons. Rewriting pylon.yaml would strip the comments that make the shipped
// config readable, and it means writing to a file the user owns and edits by
// hand. The database is only ever opened by the daemon, so every CLI process —
// which never opens it — would keep answering in the configured language while
// the GUI spoke the chosen one.
//
// A file that every process can read with one syscall, before anything else is
// set up, is what this setting actually needs.

import (
	"os"
	"path/filepath"
	"strings"
)

// prefFile is the name of the file, kept next to pylon.yaml.
const prefFile = "language"

// nativeNames are how the languages name themselves. A language picker shows
// each option in its own language and script — someone who has ended up in the
// wrong one has to be able to find their way out, and "Russian" is no help to a
// reader who only recognises "Русский".
var nativeNames = map[string]string{
	"en": "English", "de": "Deutsch", "es": "Español", "fr": "Français",
	"pt": "Português", "ru": "Русский", "tr": "Türkçe",
}

// NativeName names a language in itself, for a language picker. An unknown tag
// returns the tag, which at least identifies the row.
func NativeName(lang string) string {
	if n, ok := nativeNames[lang]; ok {
		return n
	}
	return lang
}

// IsSupported reports whether Pylon ships a catalog for a locale tag. It
// accepts the same shapes SetLanguage does ("pt-BR" counts as Portuguese).
//
// Callers that act on a person's choice should use this first: SetLanguage
// silently falls back to English, which is right for a typo in a config file
// and wrong for a button press that must either work or say why it did not.
func IsSupported(tag string) bool {
	return isSupported(base(tag))
}

// Normalize reduces a locale tag to the language code Pylon stores and reports:
// "pt-BR" and "tr_TR.UTF-8" become "pt" and "tr". An unsupported tag returns "",
// so a caller can tell "not a language we have" from "English".
func Normalize(tag string) string {
	lang := base(tag)
	if !isSupported(lang) {
		return ""
	}
	return lang
}

// PrefPath is the file a chosen language is remembered in, inside dir.
func PrefPath(dir string) string {
	return filepath.Join(dir, prefFile)
}

// LoadPref reads the language chosen in the interface, or "" if none was — no
// file, an unreadable one, or content naming a language Pylon does not have.
//
// It returns no error on purpose. A preference that cannot be read is a reason
// to fall back to the configured language, never a reason to refuse to start.
func LoadPref(dir string) string {
	b, err := os.ReadFile(PrefPath(dir))
	if err != nil {
		return ""
	}
	lang := base(strings.TrimSpace(string(b)))
	if !isSupported(lang) {
		return ""
	}
	return lang
}

// SavePref remembers a chosen language in dir. An empty lang removes the file,
// which is how "follow the system language" is stored: the absence of a choice,
// not a choice named "auto" that later has to be special-cased everywhere.
func SavePref(dir, lang string) error {
	if strings.TrimSpace(lang) == "" {
		if err := os.Remove(PrefPath(dir)); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	if !IsSupported(lang) {
		return &UnsupportedError{Tag: lang}
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(PrefPath(dir), []byte(base(lang)+"\n"), 0o600)
}

// UnsupportedError names a language Pylon has no catalog for.
type UnsupportedError struct{ Tag string }

func (e *UnsupportedError) Error() string {
	return "unsupported language: " + e.Tag + " (have: " + strings.Join(Supported, ", ") + ")"
}
