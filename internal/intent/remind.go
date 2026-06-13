package intent

import "strings"

// processAliases maps spoken process names to their canonical executable name.
var processAliases = map[string]string{
	"vscode": "code",
	"kod":    "code",
	"vs":     "code",
	"kodu":   "code",
}

// closeStems / fillers drive the heuristic remind-on-exit parser.
var (
	closeWords = map[string]bool{"close": true, "closes": true, "closed": true, "quit": true, "quits": true, "exit": true, "exits": true}
	closeStems = []string{"kapan", "kapat"} // kapanınca, kapandığında, kapatınca, ...
	// Question particles (mı/mi/mu/mü) mark a question, not a command; treating
	// them as fillers makes "kod kapandı mı" fall through (empty content → defer).
	fillerWords = map[string]bool{
		"bana": true, "beni": true, "lütfen": true, "diye": true, "ki": true, "please": true,
		"mı": true, "mi": true, "mu": true, "mü": true,
	}
	remindStems = []string{"hatırlat", "hatırla", "remind"}
)

// matchRemindOnExit recognizes the dominant "PROCESS closes CONTENT" phrasing,
// e.g. "kod kapanınca hocaya mesaj at" → process=code, content="hocaya mesaj
// at". It only fires when a process precedes the close trigger and non-filler
// content follows; anything more tangled is left for the Gemini fallback.
func matchRemindOnExit(norm string) (Command, bool) {
	toks := tokens(norm)
	trigger := -1
	for i, t := range toks {
		if isCloseTrigger(t) {
			trigger = i
			break
		}
	}
	if trigger <= 0 || trigger == len(toks)-1 {
		return Command{}, false // no trigger, nothing before it, or nothing after
	}

	process := canonicalProcess(toks[trigger-1])

	// Content is what follows the trigger, minus filler and remind words.
	var content []string
	for _, t := range toks[trigger+1:] {
		if fillerWords[t] || isRemindWord(t) {
			continue
		}
		content = append(content, t)
	}
	if len(content) == 0 {
		return Command{}, false
	}

	return Command{
		Action:     ActionRemindOnExit,
		Confidence: 0.85,
		Args: map[string]string{
			"process": process,
			"content": strings.Join(content, " "),
		},
	}, true
}

func isCloseTrigger(t string) bool {
	if closeWords[t] {
		return true
	}
	for _, stem := range closeStems {
		if strings.HasPrefix(t, stem) {
			return true
		}
	}
	return false
}

func isRemindWord(t string) bool {
	for _, stem := range remindStems {
		if strings.HasPrefix(t, stem) {
			return true
		}
	}
	return false
}

func canonicalProcess(t string) string {
	if c, ok := processAliases[t]; ok {
		return c
	}
	return t
}
