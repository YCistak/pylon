package profile

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/YCistak/pylon/internal/db"
)

func testDB(t *testing.T) *db.DB {
	t.Helper()
	d, err := db.Open(filepath.Join(t.TempDir(), "p.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { d.Close() })
	return d
}

func TestExtractSignals(t *testing.T) {
	got := extractSignals("kanka yani bu işi nasıl yapıyorsun sen")
	if _, ok := got["address:kanka"]; !ok {
		t.Fatalf("address term not extracted: %v", got)
	}
	if _, ok := got["filler:yani"]; !ok {
		t.Fatalf("filler not extracted: %v", got)
	}
	if _, ok := got["formality:informal"]; !ok {
		t.Fatalf("informal register not detected: %v", got)
	}
}

func TestFormalRegisterDetected(t *testing.T) {
	got := extractSignals("merhaba, bunu yapabilir misiniz lütfen")
	if _, ok := got["formality:formal"]; !ok {
		t.Fatalf("formal register not detected: %v", got)
	}
}

func TestObserveBuildsStyleCard(t *testing.T) {
	// refreshN=1 rebuilds the card every observation; low adopt adopts quickly.
	e := NewEngine(testDB(t), 14, 0.3, 1)
	if e.StyleCard() != "" {
		t.Fatal("card should start empty")
	}
	if err := e.Observe("kanka bu kod neden çalışmıyor ya"); err != nil {
		t.Fatalf("observe: %v", err)
	}
	card := e.StyleCard()
	if !strings.Contains(card, "kanka") {
		t.Fatalf("style card missing adopted address term: %q", card)
	}
	if !strings.Contains(card, "samimi") {
		t.Fatalf("style card missing informal register: %q", card)
	}
}

func TestAdoptionThresholdGates(t *testing.T) {
	// High threshold: a single use stays below it; repeated use crosses it.
	e := NewEngine(testDB(t), 14, 2.5, 1)
	if err := e.Observe("reis"); err != nil {
		t.Fatalf("observe: %v", err)
	}
	if strings.Contains(e.StyleCard(), "reis") {
		t.Fatalf("trait adopted too early: %q", e.StyleCard())
	}
	for i := 0; i < 3; i++ {
		if err := e.Observe("reis"); err != nil {
			t.Fatalf("observe: %v", err)
		}
	}
	if !strings.Contains(e.StyleCard(), "reis") {
		t.Fatalf("trait should be adopted after repeated use: %q", e.StyleCard())
	}
}

func TestDecayHalvesAtHalfLife(t *testing.T) {
	hl := 24 * time.Hour
	if got := decay(4, hl, hl); got < 1.99 || got > 2.01 {
		t.Fatalf("decay at one half-life = %v, want ~2", got)
	}
	if got := decay(4, 0, hl); got != 4 {
		t.Fatalf("no elapsed should not decay, got %v", got)
	}
}
