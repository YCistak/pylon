package i18n

import "testing"

func TestDecimal(t *testing.T) {
	cases := []struct {
		lang   string
		v      float64
		places int
		want   string
	}{
		{"en", 42.857142, 2, "42.86"}, // rounds
		{"tr", 42.857142, 2, "42,86"},
		{"en", 43.2, 2, "43.20"}, // trailing zero is kept: it is a place, not noise
		{"tr", 43.2, 2, "43,20"},
		{"de", 0.5, 1, "0,5"},
		{"ru", -7.25, 2, "-7,25"},
		{"fr", 1.5, 2, "1,50"},
		{"en", 42, 0, "42"},
		{"tr", 42, 0, "42"},
	}
	for _, c := range cases {
		SetLanguage(c.lang)
		if got := Decimal(c.v, c.places); got != c.want {
			t.Errorf("Decimal(%v, %d) in %s = %q, want %q", c.v, c.places, c.lang, got, c.want)
		}
	}
	SetLanguage(Default)
}

// These are the cases the exchange service used to own, when it grouped
// thousands with a point in every language.
func TestMoney(t *testing.T) {
	turkish := map[float64]string{
		34.125:     "34,13", // rounds
		0:          "0,00",
		999:        "999,00",
		1000:       "1.000,00",
		2850000.5:  "2.850.000,50",
		1234567.89: "1.234.567,89",
		-5.5:       "-5,50",
		0.999:      "1,00", // the fraction rounds into the whole part
	}
	english := map[float64]string{
		34.125:     "34.13",
		0:          "0.00",
		999:        "999.00",
		1000:       "1,000.00",
		2850000.5:  "2,850,000.50",
		1234567.89: "1,234,567.89",
		-5.5:       "-5.50",
		0.999:      "1.00",
	}
	for lang, cases := range map[string]map[float64]string{"tr": turkish, "en": english} {
		SetLanguage(lang)
		for in, want := range cases {
			if got := Money(in); got != want {
				t.Errorf("Money(%v) in %s = %q, want %q", in, lang, got, want)
			}
		}
	}
	SetLanguage(Default)
}

// French and Russian group with a space rather than a point, and a reader of
// either would misread "1.234" as a decimal. The space is a no-break one, so
// the test spells it out rather than trusting the source to show the
// difference.
func TestMoneyGroupsWithASpaceWhereTheLanguageDoes(t *testing.T) {
	const want = "1\u00a0234,50"
	for _, lang := range []string{"fr", "ru"} {
		SetLanguage(lang)
		if got := Money(1234.5); got != want {
			t.Errorf("Money(1234.5) in %s = %q, want %q", lang, got, want)
		}
	}
	SetLanguage(Default)
}

// An unknown tag is punctuated like the default language rather than with no
// separators at all: a number with nothing between the digits and the decimals
// is unreadable, while one punctuated as English is merely foreign.
func TestSeparatorsFallBackToTheDefaultLanguage(t *testing.T) {
	group, decimal := separatorsFor("ja")
	wantGroup, wantDecimal := separatorsFor(Default)
	if group != wantGroup || decimal != wantDecimal {
		t.Errorf("separatorsFor(ja) = %q/%q, want the default %q/%q", group, decimal, wantGroup, wantDecimal)
	}
}

// Every language Pylon ships has to know how to write a number, or a supported
// language silently gets English punctuation.
func TestEverySupportedLanguageHasSeparators(t *testing.T) {
	for _, lang := range Supported {
		if _, ok := separators[lang]; !ok {
			t.Errorf("no number separators for %q, which is in Supported", lang)
		}
	}
}
