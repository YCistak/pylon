package i18n

import (
	"encoding/json"
	"regexp"
	"strings"
	"testing"
)

// restore puts the process-wide language back after a test changes it, so the
// order tests run in cannot change what they assert.
func restore(t *testing.T) {
	t.Helper()
	prev := Language()
	t.Cleanup(func() { SetLanguage(prev) })
}

func TestTranslatesAndFormats(t *testing.T) {
	restore(t)

	SetLanguage("tr")
	if got := T("weather.code.clear"); got != "açık" {
		t.Errorf("tr clear = %q", got)
	}
	SetLanguage("en")
	if got := T("weather.code.clear"); got != "clear" {
		t.Errorf("en clear = %q", got)
	}
	if got := T("weather.rain", 40); got != "Chance of rain 40%." {
		t.Errorf("formatted = %q", got)
	}
}

// A key nobody translated must be visible as a bug, not silently blank: an
// empty string in a spoken reply would sound like the service simply had
// nothing to say.
func TestMissingKeyReturnsTheKey(t *testing.T) {
	if got := T("nope.not.a.key"); got != "nope.not.a.key" {
		t.Errorf("missing key = %q, want the key itself", got)
	}
	if Has("nope.not.a.key") {
		t.Error("Has should be false for a key no catalog defines")
	}
}

// A partially translated language stays usable: anything it is missing comes
// out in English rather than as a key.
func TestFallsBackToEnglish(t *testing.T) {
	restore(t)
	SetLanguage("de")

	mu.Lock()
	delete(loaded["de"], "weather.code.clear")
	mu.Unlock()

	if got := T("weather.code.clear"); got != "clear" {
		t.Errorf("fallback = %q, want the English text", got)
	}
}

// Config files and environments carry tags in every shape; only the language
// part decides, and anything unknown lands on English rather than failing.
func TestLanguageTagsAreNormalized(t *testing.T) {
	restore(t)

	cases := map[string]string{
		"tr": "tr", "tr_TR": "tr", "tr_TR.UTF-8": "tr", "pt-BR": "pt",
		"PT": "pt", "  de  ": "de",
		"": Default, "C": Default, "POSIX": Default, "klingon": Default,
	}
	for in, want := range cases {
		if got := SetLanguage(in); got != want {
			t.Errorf("SetLanguage(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestPluralCategories(t *testing.T) {
	cases := []struct {
		lang string
		n    int
		want string
	}{
		// Turkish never marks plural after a number.
		{"tr", 1, "other"}, {"tr", 5, "other"},
		// English/German/Spanish: one vs the rest.
		{"en", 1, "one"}, {"en", 0, "other"}, {"en", 2, "other"},
		{"de", 1, "one"}, {"es", 3, "other"},
		// French and Portuguese count zero as singular.
		{"fr", 0, "one"}, {"fr", 1, "one"}, {"fr", 2, "other"},
		{"pt", 0, "one"}, {"pt", 7, "other"},
		// Russian: 1/21 one, 2-4/22-24 few, 0/5-20 many.
		{"ru", 1, "one"}, {"ru", 21, "one"}, {"ru", 11, "many"},
		{"ru", 2, "few"}, {"ru", 24, "few"}, {"ru", 14, "many"},
		{"ru", 5, "many"}, {"ru", 0, "many"},
	}
	for _, c := range cases {
		if got := pluralCategory(c.lang, c.n); got != c.want {
			t.Errorf("pluralCategory(%s, %d) = %s, want %s", c.lang, c.n, got, c.want)
		}
	}
}

// Every shipped catalog must parse, and every key English defines must exist in
// the others — a language that silently drifts behind English is how a release
// ends up half-translated without anyone noticing.
func TestCatalogsAreCompleteAndConsistent(t *testing.T) {
	english := keysOf(t, Default)

	for _, lang := range Supported {
		if err := load(lang); err != nil {
			t.Fatalf("%s: %v", lang, err)
		}
		keys := keysOf(t, lang)

		for k := range english {
			if _, ok := keys[k]; !ok {
				t.Errorf("%s.json is missing key %q", lang, k)
			}
		}
		for k := range keys {
			if _, ok := english[k]; !ok {
				t.Errorf("%s.json has key %q that en.json does not", lang, k)
			}
		}
	}
}

// Format verbs must match across languages, or a translated sentence prints
// "%!d(string=…)" at the one moment someone is listening to it.
func TestFormatVerbsMatchEnglish(t *testing.T) {
	english := stringsOf(t, Default)

	for _, lang := range Supported {
		if lang == Default {
			continue
		}
		for key, text := range stringsOf(t, lang) {
			want, ok := english[key]
			if !ok {
				continue // reported by the completeness test
			}
			if a, b := verbs(want), verbs(text); a != b {
				t.Errorf("%s.json %q uses verbs %q, en.json uses %q", lang, key, b, a)
			}
		}
	}
}

// Go puts the argument index immediately before the verb: "%.0[3]f", never
// "%[3].0f". The wrong order parses, advances the argument counter, and then
// prints "%!f(BADINDEX)" — in a spoken sentence, at the moment someone is
// listening. Every catalog is checked because the mistake is invisible until a
// language with a reordered sentence is selected.
func TestIndexedVerbsPutTheIndexLast(t *testing.T) {
	bad := regexp.MustCompile(`%\[\d+\][^a-zA-Z%]`)

	for _, lang := range Supported {
		for key, text := range stringsOf(t, lang) {
			if m := bad.FindString(text); m != "" {
				t.Errorf("%s.json %q: %q puts flags after the index; write %%.0[n]f, not %%[n].0f",
					lang, key, m)
			}
		}
	}
}

func keysOf(t *testing.T, lang string) map[string]struct{} {
	t.Helper()
	out := map[string]struct{}{}
	for k := range rawCatalog(t, lang) {
		out[k] = struct{}{}
	}
	return out
}

// stringsOf returns the plain-string entries of a catalog. Plural entries are
// skipped: their forms are compared by the plural tests, not verb-by-verb.
func stringsOf(t *testing.T, lang string) map[string]string {
	t.Helper()
	out := map[string]string{}
	for k, v := range rawCatalog(t, lang) {
		var s string
		if err := json.Unmarshal(v, &s); err == nil {
			out[k] = s
		}
	}
	return out
}

func rawCatalog(t *testing.T, lang string) map[string]json.RawMessage {
	t.Helper()
	data, err := catalogs.ReadFile("locales/" + lang + ".json")
	if err != nil {
		t.Fatalf("read %s: %v", lang, err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("parse %s: %v", lang, err)
	}
	return raw
}

// verbs collects the format verbs in a message, sorted into a comparable
// string. Indexed verbs ("%[1]s") keep their index, since that is what lets a
// translation reorder the sentence.
func verbs(msg string) string {
	var found []string
	for i := 0; i < len(msg); i++ {
		if msg[i] != '%' {
			continue
		}
		if i+1 < len(msg) && msg[i+1] == '%' { // literal percent
			i++
			continue
		}
		j := i + 1
		for j < len(msg) && !strings.ContainsRune("bcdeEfFgGoqstTvxXU", rune(msg[j])) {
			j++
		}
		if j < len(msg) {
			found = append(found, msg[i:j+1])
			i = j
		}
	}
	// Sorted by construction of the callers' usage: compare as a set, since a
	// translation may legitimately reorder indexed verbs.
	return strings.Join(sorted(found), ",")
}

func sorted(in []string) []string {
	out := append([]string(nil), in...)
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j] < out[j-1]; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}

// Money is where fractional plurals actually bite: "0.87 dollars" is plural in
// English, "0,87 euro" singular in French, and the difference is a rule about
// the integer part, not about the fraction.
func TestFormFloatFollowsTheLanguagesRule(t *testing.T) {
	restore(t)

	cases := []struct {
		lang string
		v    float64
		want string
	}{
		{"en", 1, "dollar"}, {"en", 0.87, "dollars"}, {"en", 34.12, "dollars"},
		{"fr", 1, "dollar"}, {"fr", 0.87, "dollar"}, {"fr", 2.5, "dollars"},
		{"pt", 0.5, "dólar"}, {"pt", 3.2, "dólares"},
		{"tr", 1, "dolar"}, {"tr", 34.12, "dolar"},
		{"ru", 1, "доллар"}, {"ru", 3, "доллара"}, {"ru", 7, "долларов"},
	}
	for _, c := range cases {
		SetLanguage(c.lang)
		if got := FormFloat("currency.USD", c.v); got != c.want {
			t.Errorf("%s FormFloat(%v) = %q, want %q", c.lang, c.v, got, c.want)
		}
	}
}
