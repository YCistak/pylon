package profile

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/YCistak/pylon/internal/db"
)

// Engine learns and serves the user's speaking style. It is safe for concurrent
// use by the daemon's single intent handler.
type Engine struct {
	db       *db.DB
	halfLife time.Duration // exponential-decay half-life for signal weights
	adopt    float64       // min decayed weight before a trait is used
	refreshN int           // rebuild the style card every N observations

	mu   sync.Mutex
	seen int    // observations since the last card rebuild
	card string // cached style card
}

// NewEngine builds a persona Engine. halfLifeDays and adopt come from config;
// refreshN batches style-card rebuilds so it (and any prompt cache) stays warm.
func NewEngine(store *db.DB, halfLifeDays, adopt float64, refreshN int) *Engine {
	if halfLifeDays <= 0 {
		halfLifeDays = 14
	}
	if refreshN <= 0 {
		refreshN = 20
	}
	e := &Engine{
		db:       store,
		halfLife: time.Duration(halfLifeDays * float64(24*time.Hour)),
		adopt:    adopt,
		refreshN: refreshN,
	}
	e.card = e.buildCard() // warm the cache from whatever is already stored
	return e
}

// Observe folds one transcript's style signals into the persona profile: each
// signal's weight is decayed to now, incremented by its occurrence count, and
// stored. The style card is rebuilt every refreshN observations.
func (e *Engine) Observe(text string) error {
	sigs := extractSignals(text)
	if len(sigs) == 0 {
		return nil
	}

	stored, err := e.db.PersonaSignals()
	if err != nil {
		return err
	}
	now := time.Now()
	prev := make(map[string]db.Signal, len(stored))
	for _, s := range stored {
		prev[s.Signal] = s
	}

	for key, o := range sigs {
		base := 0.0
		if old, ok := prev[key]; ok {
			base = decay(old.Weight, now.Sub(old.UpdatedAt), e.halfLife)
		}
		if err := e.db.UpsertPersonaSignal(key, o.value, base+float64(o.count)); err != nil {
			return err
		}
	}

	e.mu.Lock()
	e.seen++
	rebuild := e.seen >= e.refreshN
	if rebuild {
		e.seen = 0
	}
	e.mu.Unlock()
	if rebuild {
		card := e.buildCard()
		e.mu.Lock()
		e.card = card
		e.mu.Unlock()
	}
	return nil
}

// StyleCard returns the cached compact style description for the LLM system
// prompt. Empty until enough signals cross the adoption threshold.
func (e *Engine) StyleCard() string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.card
}

// buildCard reads all signals, decays them to now, picks the dominant adopted
// value per category, and renders a short Turkish style description.
func (e *Engine) buildCard() string {
	stored, err := e.db.PersonaSignals()
	if err != nil || len(stored) == 0 {
		return ""
	}
	now := time.Now()

	// Best decayed-weight value per category, gated by the adoption threshold.
	type pick struct {
		value  string
		weight float64
	}
	best := map[string]pick{}
	for _, s := range stored {
		cat, val, ok := splitKey(s.Signal)
		if !ok {
			continue
		}
		w := decay(s.Weight, now.Sub(s.UpdatedAt), e.halfLife)
		if w < e.adopt {
			continue
		}
		if cur, seen := best[cat]; !seen || w > cur.weight {
			best[cat] = pick{value: val, weight: w}
		}
	}
	if len(best) == 0 {
		return ""
	}

	var parts []string
	if p, ok := best["address"]; ok {
		parts = append(parts, fmt.Sprintf("sana %q diye hitap ediyor", p.value))
	}
	if p, ok := best["formality"]; ok {
		if p.value == "formal" {
			parts = append(parts, "resmi (siz dili) konuşuyor")
		} else {
			parts = append(parts, "samimi (sen dili) konuşuyor")
		}
	}
	if fillers := adoptedFillers(stored, now, e.halfLife, e.adopt); len(fillers) > 0 {
		parts = append(parts, "sık kullandığı dolgu sözcükleri: "+strings.Join(fillers, ", "))
	}
	if p, ok := best["verbosity"]; ok {
		switch p.value {
		case "short":
			parts = append(parts, "kısa cümleler kuruyor")
		case "long":
			parts = append(parts, "uzun, detaylı cümleler kuruyor")
		}
	}
	if len(parts) == 0 {
		return ""
	}
	return "Kullanıcının konuşma tarzı: " + strings.Join(parts, "; ") + ". Cevaplarında bu tarzı doğal biçimde yansıt."
}

// adoptedFillers returns up to three most-used filler words above threshold.
func adoptedFillers(stored []db.Signal, now time.Time, halfLife time.Duration, adopt float64) []string {
	type fw struct {
		word   string
		weight float64
	}
	var fillers []fw
	for _, s := range stored {
		cat, val, ok := splitKey(s.Signal)
		if !ok || cat != "filler" {
			continue
		}
		if w := decay(s.Weight, now.Sub(s.UpdatedAt), halfLife); w >= adopt {
			fillers = append(fillers, fw{val, w})
		}
	}
	sort.Slice(fillers, func(i, j int) bool { return fillers[i].weight > fillers[j].weight })
	var out []string
	for i, f := range fillers {
		if i == 3 {
			break
		}
		out = append(out, f.word)
	}
	return out
}

// decay applies exponential decay: weight halves every halfLife of elapsed time.
func decay(weight float64, elapsed, halfLife time.Duration) float64 {
	if halfLife <= 0 || elapsed <= 0 {
		return weight
	}
	return weight * math.Pow(0.5, elapsed.Seconds()/halfLife.Seconds())
}

func splitKey(key string) (category, value string, ok bool) {
	i := strings.IndexByte(key, ':')
	if i <= 0 || i == len(key)-1 {
		return "", "", false
	}
	return key[:i], key[i+1:], true
}
