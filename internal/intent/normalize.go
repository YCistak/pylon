package intent

import (
	"strings"
	"unicode"
)

// normalize lower-cases (Turkish-aware), strips punctuation, and collapses
// whitespace so phrase matching is robust to casing and spacing.
func normalize(s string) string {
	s = turkishLower(s)
	var b strings.Builder
	b.Grow(len(s))
	prevSpace := false
	for _, r := range s {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
			prevSpace = false
		case unicode.IsSpace(r) || unicode.IsPunct(r):
			if !prevSpace {
				b.WriteRune(' ')
				prevSpace = true
			}
		}
	}
	return strings.TrimSpace(b.String())
}

// turkishLower lower-cases handling the Turkish dotted/dotless I correctly
// (I→ı, İ→i) before applying the generic Unicode fold.
func turkishLower(s string) string {
	r := strings.NewReplacer("I", "ı", "İ", "i")
	return strings.ToLower(r.Replace(s))
}

// speechTail are trailing speech-marker words Gemini sometimes leaves on a
// reminder body ("anneni ara de", "annene söyle") — they tell Pylon to speak,
// not part of the reminder itself, so they are stripped before storage.
var speechTail = map[string]bool{
	"de": true, "diye": true, "söyle": true, "söyler": true,
	"der": true, "dersin": true, "demiştim": true,
}

// trimSpeechTail removes trailing speech-marker words from s, keeping at least
// one word so a reminder is never emptied.
func trimSpeechTail(s string) string {
	f := strings.Fields(s)
	for len(f) > 1 && speechTail[normalize(f[len(f)-1])] {
		f = f[:len(f)-1]
	}
	return strings.Join(f, " ")
}

// tokens splits normalized text into words.
func tokens(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Fields(s)
}

// firstToken returns the first whitespace-separated word of s, or "".
func firstToken(s string) string {
	if f := strings.Fields(s); len(f) > 0 {
		return f[0]
	}
	return ""
}

// levenshtein returns the edit distance between a and b.
func levenshtein(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	la, lb := len(ra), len(rb)
	if la == 0 {
		return lb
	}
	if lb == 0 {
		return la
	}
	prev := make([]int, lb+1)
	curr := make([]int, lb+1)
	for j := 0; j <= lb; j++ {
		prev[j] = j
	}
	for i := 1; i <= la; i++ {
		curr[0] = i
		for j := 1; j <= lb; j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			curr[j] = min3(curr[j-1]+1, prev[j]+1, prev[j-1]+cost)
		}
		prev, curr = curr, prev
	}
	return prev[lb]
}

// similar reports the similarity ratio (0..1) between two words via edit distance.
func similar(a, b string) float64 {
	if a == b {
		return 1
	}
	maxLen := len([]rune(a))
	if l := len([]rune(b)); l > maxLen {
		maxLen = l
	}
	if maxLen == 0 {
		return 1
	}
	return 1 - float64(levenshtein(a, b))/float64(maxLen)
}

func min3(a, b, c int) int {
	if b < a {
		a = b
	}
	if c < a {
		a = c
	}
	return a
}
