package db

import "testing"

func TestPersonaSignalUpsert(t *testing.T) {
	d := openTestDB(t)

	if err := d.UpsertPersonaSignal("address:kanka", "kanka", 1.5); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	// Same key updates in place rather than duplicating.
	if err := d.UpsertPersonaSignal("address:kanka", "kanka", 2.25); err != nil {
		t.Fatalf("upsert 2: %v", err)
	}
	if err := d.UpsertPersonaSignal("formality:informal", "informal", 3); err != nil {
		t.Fatalf("upsert 3: %v", err)
	}

	sigs, err := d.PersonaSignals()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(sigs) != 2 {
		t.Fatalf("want 2 signals, got %d: %+v", len(sigs), sigs)
	}
	byKey := map[string]Signal{}
	for _, s := range sigs {
		byKey[s.Signal] = s
	}
	if got := byKey["address:kanka"]; got.Weight != 2.25 || got.Value != "kanka" {
		t.Fatalf("address signal = %+v", got)
	}
	if byKey["address:kanka"].UpdatedAt.IsZero() {
		t.Fatal("updated_at not set")
	}
}

func TestContextStore(t *testing.T) {
	d := openTestDB(t)

	if _, ok, _ := d.GetContext("missing"); ok {
		t.Fatal("missing key should not be found")
	}

	if err := d.SetContext("last_topic", "go generics"); err != nil {
		t.Fatalf("set: %v", err)
	}
	if err := d.SetContext("last_topic", "sqlite migrations"); err != nil {
		t.Fatalf("update: %v", err)
	}
	val, ok, err := d.GetContext("last_topic")
	if err != nil || !ok {
		t.Fatalf("get: ok=%v err=%v", ok, err)
	}
	if val != "sqlite migrations" {
		t.Fatalf("value = %q, want updated", val)
	}

	if err := d.SetContext("mood", "focused"); err != nil {
		t.Fatalf("set mood: %v", err)
	}
	recent, err := d.RecentContext(5)
	if err != nil {
		t.Fatalf("recent: %v", err)
	}
	if len(recent) != 2 {
		t.Fatalf("want 2 entries, got %d", len(recent))
	}
	// Newest first: "mood" was written last.
	if recent[0].Key != "mood" {
		t.Fatalf("recent order wrong: %+v", recent)
	}
}
