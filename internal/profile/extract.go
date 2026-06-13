// Package profile is Pylon's persona engine: a local, deterministic statistics
// layer that learns how the user speaks (address terms, formality, fillers,
// verbosity) by counting — no ML, no API cost. It feeds a compact "style card"
// into the LLM system prompt so replies gradually mirror the user.
package profile

import (
	"strings"
	"unicode"
)

// observation is one extracted signal: a composite "category:value" key with how
// many times it appeared in the sentence.
type observation struct {
	value string
	count int
}

// addressTerms are informal Turkish terms of address. The matched term becomes
// the value so the engine learns which one the user prefers.
var addressTerms = map[string]bool{
	"kanka": true, "kanki": true, "abi": true, "abicim": true, "abla": true,
	"reis": true, "dostum": true, "kardeşim": true, "moruk": true, "birader": true,
	"koçum": true, "hocam": true, "usta": true, "kaptan": true, "lan": true,
}

// fillerWords are discourse fillers worth mirroring.
var fillerWords = map[string]bool{
	"yani": true, "işte": true, "hani": true, "falan": true, "filan": true,
	"aslında": true, "açıkçası": true, "şey": true,
}

// formalMarkers / informalMarkers are light heuristics for sen/siz register.
var (
	formalMarkers   = map[string]bool{"siz": true, "sizin": true, "size": true, "sizi": true, "lütfen": true, "rica": true}
	informalMarkers = map[string]bool{"sen": true, "senin": true, "seni": true, "sana": true}
)

// extractSignals counts the style signals present in one transcript, keyed by
// "category:value". Returns nil for empty input.
func extractSignals(text string) map[string]observation {
	toks := lowerTokens(text)
	if len(toks) == 0 {
		return nil
	}
	out := map[string]observation{}
	add := func(category, value string) {
		key := category + ":" + value
		o := out[key]
		o.value = value
		o.count++
		out[key] = o
	}

	var formal, informal int
	for _, t := range toks {
		switch {
		case addressTerms[t]:
			add("address", t)
		case fillerWords[t]:
			add("filler", t)
		}
		if formalMarkers[t] || hasFormalEnding(t) {
			formal++
		}
		if informalMarkers[t] || addressTerms[t] {
			informal++
		}
	}

	if formal > informal {
		add("formality", "formal")
	} else if informal > formal {
		add("formality", "informal")
	}

	// Verbosity bucket from sentence length, so a dominant cadence emerges.
	switch {
	case len(toks) <= 4:
		add("verbosity", "short")
	case len(toks) >= 12:
		add("verbosity", "long")
	default:
		add("verbosity", "medium")
	}
	return out
}

// hasFormalEnding detects the 2nd-person-plural/polite verb endings that signal
// the "siz" register (…sınız/…siniz/…sunuz/…sünüz).
func hasFormalEnding(t string) bool {
	for _, suf := range []string{"sınız", "siniz", "sunuz", "sünüz"} {
		if strings.HasSuffix(t, suf) {
			return true
		}
	}
	return false
}

// lowerTokens lower-cases (Turkish-aware) and splits text into word tokens,
// dropping punctuation.
func lowerTokens(s string) []string {
	s = turkishLower(s)
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		} else {
			b.WriteRune(' ')
		}
	}
	return strings.Fields(b.String())
}

func turkishLower(s string) string {
	return strings.ToLower(strings.NewReplacer("I", "ı", "İ", "i").Replace(s))
}
