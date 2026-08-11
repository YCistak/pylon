package i18n

// How a number is punctuated belongs to the language it is spoken in. "1 USD is
// 47,71 Turkish lira" does not read as a slightly foreign sentence to an English
// speaker — it reads as a different number, one-hundredth of the real one.
//
// Two services used to format numbers themselves, and both hard-coded Turkish
// punctuation (a point between thousands, a comma before the decimals) because
// Turkish was the only language Pylon had. Their assumption is the table below
// now, so a service asks for a number in the active language the same way it
// asks for a sentence.

import (
	"fmt"
	"strconv"
	"strings"
)

// nbsp groups thousands in the languages whose typography asks for a space. It
// is a no-break space rather than an ordinary one, so an amount never breaks
// across two lines in the interface; a speech engine reads both the same way.
// Named, because the two are indistinguishable in a source listing.
const nbsp = "\u00a0"

// separators is how each language groups thousands and marks the decimal.
// English is the odd one out — every other language Pylon speaks marks decimals
// with a comma.
var separators = map[string]struct{ group, decimal string }{
	"en": {",", "."},
	"de": {".", ","},
	"es": {".", ","},
	"fr": {nbsp, ","},
	"pt": {".", ","},
	"ru": {nbsp, ","},
	"tr": {".", ","},
}

// separatorsFor falls back to the default language rather than to empty
// strings: a number with no decimal separator at all is unreadable, while one
// punctuated as English is merely wrong for that reader.
func separatorsFor(lang string) (group, decimal string) {
	s, ok := separators[lang]
	if !ok {
		s = separators[Default]
	}
	return s.group, s.decimal
}

// Decimal writes v with places decimals in the active language, thousands
// ungrouped: "42,86" in Turkish, "42.86" in English. Grouping is left out
// because the caller that wants it wants an amount of money — see Money.
func Decimal(v float64, places int) string {
	_, decimal := separatorsFor(Language())
	return strings.Replace(strconv.FormatFloat(v, 'f', places, 64), ".", decimal, 1)
}

// Money writes v as an amount: two decimals, thousands grouped.
// "2.850.000,50" in Turkish, "2,850,000.50" in English.
func Money(v float64) string {
	group, decimal := separatorsFor(Language())
	negative := v < 0
	if negative {
		v = -v
	}
	whole := int64(v)
	frac := int64((v-float64(whole))*100 + 0.5)
	if frac == 100 { // rounding carried into the integer part
		whole++
		frac = 0
	}
	s := groupThousands(strconv.FormatInt(whole, 10), group) + decimal + fmt.Sprintf("%02d", frac)
	if negative {
		return "-" + s
	}
	return s
}

// groupThousands inserts sep every three digits from the right.
func groupThousands(digits, sep string) string {
	n := len(digits)
	if n <= 3 {
		return digits
	}
	var b strings.Builder
	lead := n % 3
	if lead > 0 {
		b.WriteString(digits[:lead])
		b.WriteString(sep)
	}
	for i := lead; i < n; i += 3 {
		b.WriteString(digits[i : i+3])
		if i+3 < n {
			b.WriteString(sep)
		}
	}
	return b.String()
}
