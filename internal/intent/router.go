package intent

// Router resolves frequent commands locally, with no API call. It matches a
// transcript against per-intent trigger phrases (Turkish + English) using
// token-level fuzzy matching, so word order, casing, and small typos/ASR slips
// don't break recognition.
type Router struct {
	threshold float64
	intents   []intentSpec
}

type intentSpec struct {
	action  Action
	phrases []string // raw trigger phrases; normalized at construction
}

// fuzzyTokenThreshold is the per-word similarity above which two tokens count
// as the "same" word during phrase scoring.
const fuzzyTokenThreshold = 0.8

// NewRouter builds a Router. threshold is the minimum phrase score (0..1) for a
// match to count as resolved; values <= 0 default to 0.8.
func NewRouter(threshold float64) *Router {
	if threshold <= 0 {
		threshold = 0.8
	}
	return &Router{
		threshold: threshold,
		intents:   defaultIntents(),
	}
}

// defaultIntents is the built-in command vocabulary. Phrases are intentionally
// multi-word where possible to avoid trigger-happy single-token matches.
func defaultIntents() []intentSpec {
	return []intentSpec{
		{ActionLockScreen, []string{"ekranı kilitle", "ekran kilitle", "lock screen", "lock the screen"}},
		{ActionMediaPlay, []string{"müzik çal", "şarkı çal", "devam et", "oynat", "play", "resume"}},
		{ActionMediaPause, []string{"durdur", "duraklat", "müziği durdur", "pause", "stop music"}},
		{ActionMediaNext, []string{"sonraki şarkı", "sonraki parça", "geç", "next", "next song", "skip"}},
		{ActionMediaPrev, []string{"önceki şarkı", "önceki parça", "previous", "previous song"}},
		{ActionVolumeUp, []string{"sesi aç", "sesi yükselt", "sesi artır", "volume up", "louder"}},
		{ActionVolumeDown, []string{"sesi kıs", "sesi azalt", "sesi düşür", "volume down", "quieter"}},
		{ActionMute, []string{"sesi kapat", "sustur", "mute"}},
	}
}

// Resolve interprets transcript. If a phrase scores at or above the threshold it
// returns a resolved Command; otherwise it returns ActionUnknown so the caller
// can fall back to Gemini. Parameterized commands (e.g. remind-on-exit) are
// tried first via dedicated matchers.
func (r *Router) Resolve(transcript string) Command {
	norm := normalize(transcript)
	if norm == "" {
		return Command{Action: ActionUnknown}
	}

	// Parameterized matchers take precedence over the phrase table.
	if cmd, ok := matchRemindOnExit(norm); ok {
		return cmd
	}

	tTokens := tokens(norm)
	best := Command{Action: ActionUnknown}
	for _, spec := range r.intents {
		score := spec.score(tTokens)
		if score > best.Confidence {
			best = Command{Action: spec.action, Confidence: score}
		}
	}
	if best.Confidence < r.threshold {
		return Command{Action: ActionUnknown, Confidence: best.Confidence}
	}
	return best
}

// score returns the best phrase score for this intent against the transcript
// tokens: for each phrase, the mean over its tokens of the best fuzzy match
// found in the transcript. Extra transcript words are harmless.
func (s intentSpec) score(tTokens []string) float64 {
	var best float64
	for _, phrase := range s.phrases {
		pTokens := tokens(normalize(phrase))
		if len(pTokens) == 0 {
			continue
		}
		var sum float64
		for _, pt := range pTokens {
			var bestTok float64
			for _, tt := range tTokens {
				if sim := similar(pt, tt); sim > bestTok {
					bestTok = sim
				}
			}
			if bestTok >= fuzzyTokenThreshold {
				sum += bestTok
			}
			// tokens below the per-word threshold contribute 0
		}
		if sc := sum / float64(len(pTokens)); sc > best {
			best = sc
		}
	}
	return best
}
