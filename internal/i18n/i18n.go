// Package i18n holds every string Pylon says to a person, in every language it
// says them in.
//
// Pylon was written in Turkish, with the sentences inline where they were
// spoken. That made one language easy and a second one impossible: the wording
// lived in 32 files, and half of it was assembled by fmt.Sprintf in the middle
// of business logic. The catalogs here are the same sentences, keyed and
// embedded, so adding a language is adding one JSON file.
//
// Three rules keep the callers honest:
//
//   - A missing key returns the key itself, never an empty string. A gap in a
//     translation shows up as "work.today.summary" on screen — ugly, findable,
//     and impossible to mistake for a working feature.
//   - A missing translation falls back to English, not to nothing. A half
//     translated language stays usable.
//   - Plural forms are per-language and picked by the catalog, not by the call
//     site: "1 saat" needs no plural in Turkish, "1 hour"/"2 hours" needs two in
//     English, and Russian needs four.
//
// The active language is process-wide because the alternative — threading a
// locale through every service method — buys nothing for a daemon that serves
// one person at a time. Tests set it explicitly (SetLanguage) and get
// deterministic output; the default is English, so a test that forgets is
// wrong in an obvious way rather than a locale-dependent one.
package i18n

import (
	"embed"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
)

//go:embed locales/*.json
var catalogs embed.FS

// Default is the language used when none is configured and none can be guessed,
// and the one every other language falls back to key by key.
const Default = "en"

// Supported lists the languages shipped with Pylon, in the order the docs and
// the settings screen show them: the default first, then alphabetically.
var Supported = []string{"en", "de", "es", "fr", "pt", "ru", "tr"}

// message is one catalog entry: either a plain string, or a set of plural forms
// keyed by CLDR category ("one", "few", "many", "other").
type message struct {
	one    string
	other  string
	few    string
	many   string
	plural bool
}

var (
	mu      sync.RWMutex
	current = Default
	loaded  = map[string]map[string]message{}
)

func init() {
	// English is always in memory: it is the fallback for every other language,
	// so a lazy load would just move the work to the first missing key.
	if err := load(Default); err != nil {
		panic("i18n: the English catalog must be loadable: " + err.Error())
	}
}

// SetLanguage switches the active language. An empty or unknown tag falls back
// to English rather than failing: a typo in a config file must not take the
// daemon down, and English text is still text.
//
// It accepts what people actually write in config files and environments —
// "tr", "tr_TR", "tr_TR.UTF-8", "pt-BR" — and keeps the base language.
func SetLanguage(tag string) string {
	lang := base(tag)
	if lang == "" || !isSupported(lang) {
		lang = Default
	}
	if err := load(lang); err != nil {
		lang = Default
	}
	mu.Lock()
	current = lang
	mu.Unlock()
	return lang
}

// Language reports the active language.
func Language() string {
	mu.RLock()
	defer mu.RUnlock()
	return current
}

// FromEnv guesses the language from the environment the way every other
// command-line program does, so a fresh install speaks the desktop's language
// without being configured. LC_ALL wins over LC_MESSAGES over LANG, matching
// POSIX precedence. It returns "" when there is nothing to go on — Windows sets
// none of these, and guessing wrong is worse than defaulting to English.
func FromEnv() string {
	for _, key := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
		if v := base(os.Getenv(key)); v != "" && isSupported(v) {
			return v
		}
	}
	return ""
}

// T returns the message for key in the active language, formatted with args.
// A key with no translation anywhere returns the key itself.
func T(key string, args ...any) string {
	return format(lookup(key).other, key, args...)
}

// N returns the plural form of key that fits n, formatted with n followed by
// args. The count is always the first verb, because every plural sentence in
// the catalogs states its number.
//
//	i18n.N("work.hours", 2)  → "2 hours" / "2 saat" / "2 часа"
func N(key string, n int, args ...any) string {
	m := lookup(key)
	form := m.other
	if m.plural {
		switch pluralCategory(Language(), n) {
		case "one":
			form = firstNonEmpty(m.one, m.other)
		case "few":
			form = firstNonEmpty(m.few, m.other)
		case "many":
			form = firstNonEmpty(m.many, m.other)
		}
	}
	return format(form, key, append([]any{n}, args...)...)
}

// englishNames are how the languages are named to an LLM. English on purpose:
// a model follows "write it in Portuguese" more reliably than "escreve em
// português", and the instruction is the one part of the prompt the user never
// sees.
var englishNames = map[string]string{
	"en": "English", "de": "German", "es": "Spanish", "fr": "French",
	"pt": "Portuguese", "ru": "Russian", "tr": "Turkish",
}

// EnglishName names a language in English, for prompts. An unknown tag returns
// the tag, which is still a usable instruction.
func EnglishName(lang string) string {
	if n, ok := englishNames[lang]; ok {
		return n
	}
	return lang
}

// Form picks the plural form that agrees with n but does not print n — for
// names that stand next to a number the sentence states in its own way: "1
// dollar" and "34.12 dollars" share one amount but not one form, and the amount
// is formatted as money, not as a count.
func Form(key string, n int) string {
	m := lookup(key)
	if m.other == "" {
		return key
	}
	if !m.plural {
		return m.other
	}
	switch pluralCategory(Language(), n) {
	case "one":
		return firstNonEmpty(m.one, m.other)
	case "few":
		return firstNonEmpty(m.few, m.other)
	case "many":
		return firstNonEmpty(m.many, m.other)
	}
	return m.other
}

// Has reports whether key exists in any loaded catalog. It exists for tests
// that assert every key a package uses is actually shipped.
func Has(key string) bool {
	mu.RLock()
	defer mu.RUnlock()
	if _, ok := loaded[current][key]; ok {
		return true
	}
	_, ok := loaded[Default][key]
	return ok
}

// lookup finds key in the active language, then in English. The zero message
// means "not found anywhere"; callers turn that into the key itself.
func lookup(key string) message {
	mu.RLock()
	defer mu.RUnlock()
	if m, ok := loaded[current][key]; ok {
		return m
	}
	if m, ok := loaded[Default][key]; ok {
		return m
	}
	return message{}
}

// format applies args, falling back to the key when the message is missing.
// Sprintf errors (a catalog verb that does not match the call) are left in the
// output on purpose: "%!s(MISSING)" in a sentence is a bug report, and hiding
// it would make the wrong text look intentional.
func format(msg, key string, args ...any) string {
	if msg == "" {
		return key
	}
	if len(args) == 0 {
		return msg
	}
	return fmt.Sprintf(msg, args...)
}

// load reads and caches one catalog. Already-loaded languages are a no-op, so
// callers may call it freely.
func load(lang string) error {
	mu.RLock()
	_, done := loaded[lang]
	mu.RUnlock()
	if done {
		return nil
	}

	data, err := catalogs.ReadFile("locales/" + lang + ".json")
	if err != nil {
		return err
	}
	// A catalog value is either a string or an object of plural forms, so it is
	// decoded loosely and narrowed here.
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("i18n: %s.json: %w", lang, err)
	}

	cat := make(map[string]message, len(raw))
	for key, val := range raw {
		var s string
		if err := json.Unmarshal(val, &s); err == nil {
			cat[key] = message{other: s}
			continue
		}
		var forms struct {
			One   string `json:"one"`
			Few   string `json:"few"`
			Many  string `json:"many"`
			Other string `json:"other"`
		}
		if err := json.Unmarshal(val, &forms); err != nil {
			return fmt.Errorf("i18n: %s.json: key %q is neither a string nor plural forms", lang, key)
		}
		cat[key] = message{
			one: forms.One, few: forms.Few, many: forms.Many,
			other: forms.Other, plural: true,
		}
	}

	mu.Lock()
	loaded[lang] = cat
	mu.Unlock()
	return nil
}

// pluralCategory implements the CLDR plural rules for the shipped languages.
// Only the categories those languages actually use are computed; anything else
// falls to "other", which every catalog entry defines.
func pluralCategory(lang string, n int) string {
	if n < 0 {
		n = -n
	}
	switch lang {
	case "tr":
		// Turkish marks no plural after a number: "1 saat", "5 saat".
		return "other"
	case "fr", "pt":
		// 0 and 1 are singular in both.
		if n <= 1 {
			return "one"
		}
		return "other"
	case "ru":
		// 1, 21, 31… one; 2-4, 22-24… few; 0, 5-20, 25-30… many.
		switch {
		case n%10 == 1 && n%100 != 11:
			return "one"
		case n%10 >= 2 && n%10 <= 4 && (n%100 < 12 || n%100 > 14):
			return "few"
		default:
			return "many"
		}
	default: // en, de, es
		if n == 1 {
			return "one"
		}
		return "other"
	}
}

// base reduces a locale tag to its language: "tr_TR.UTF-8" → "tr", "pt-BR" →
// "pt". Empty and the POSIX no-op locales ("C", "POSIX") yield "".
func base(tag string) string {
	tag = strings.TrimSpace(tag)
	if i := strings.IndexAny(tag, ".@"); i >= 0 {
		tag = tag[:i]
	}
	if i := strings.IndexAny(tag, "_-"); i >= 0 {
		tag = tag[:i]
	}
	tag = strings.ToLower(tag)
	if tag == "c" || tag == "posix" {
		return ""
	}
	return tag
}

func isSupported(lang string) bool {
	for _, s := range Supported {
		if s == lang {
			return true
		}
	}
	return false
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
