package intent

import "testing"

func TestRouterResolvesFrequentCommands(t *testing.T) {
	r := NewRouter(0.8)
	cases := map[string]Action{
		"ekranı kilitle":         ActionLockScreen,
		"Ekranı Kilitle":         ActionLockScreen, // casing
		"lock the screen please": ActionLockScreen,
		"müzik çal":              ActionMediaPlay,
		"şarkıyı durdur":         ActionMediaPause, // extra word + suffix
		"sonraki şarkı":          ActionMediaNext,
		"sesi aç":                ActionVolumeUp,
		"sesi biraz kıs":         ActionVolumeDown, // extra word in between
		"sustur":                 ActionMute,
		"next song":              ActionMediaNext,
	}
	for input, want := range cases {
		t.Run(input, func(t *testing.T) {
			cmd := r.Resolve(input)
			if cmd.Action != want {
				t.Fatalf("Resolve(%q) = %q (conf %.2f), want %q", input, cmd.Action, cmd.Confidence, want)
			}
			if !cmd.Resolved() {
				t.Fatalf("expected resolved command for %q", input)
			}
		})
	}
}

func TestRouterToleratesTypos(t *testing.T) {
	r := NewRouter(0.8)
	// ASR slips: missing/extra letters should still resolve.
	cmd := r.Resolve("ekranı kilitlee")
	if cmd.Action != ActionLockScreen {
		t.Fatalf("typo not tolerated: got %q (conf %.2f)", cmd.Action, cmd.Confidence)
	}
}

func TestRouterDefersUnknown(t *testing.T) {
	r := NewRouter(0.8)
	for _, input := range []string{
		"bugün hava nasıl",
		"dolar kaç para",
		"bana bir fıkra anlat",
		"",
	} {
		if cmd := r.Resolve(input); cmd.Resolved() {
			t.Fatalf("Resolve(%q) should defer, got %q (conf %.2f)", input, cmd.Action, cmd.Confidence)
		}
	}
}

func TestRemindOnExit(t *testing.T) {
	r := NewRouter(0.8)
	cases := []struct {
		input   string
		process string
		content string
	}{
		{"kod kapanınca hocaya mesaj at", "code", "hocaya mesaj at"},
		{"vscode kapatınca bana spotify aç hatırlat", "code", "spotify aç"},
		{"cs2 kapandığında dinlenmemi hatırlat", "cs2", "dinlenmemi"},
	}
	for _, c := range cases {
		t.Run(c.input, func(t *testing.T) {
			cmd := r.Resolve(c.input)
			if cmd.Action != ActionRemindOnExit {
				t.Fatalf("Resolve(%q) = %q, want remind_on_exit", c.input, cmd.Action)
			}
			if got := cmd.arg("process"); got != c.process {
				t.Fatalf("process = %q, want %q", got, c.process)
			}
			if got := cmd.arg("content"); got != c.content {
				t.Fatalf("content = %q, want %q", got, c.content)
			}
		})
	}
}

func TestRemindOnExitDefersWhenIncomplete(t *testing.T) {
	r := NewRouter(0.8)
	// Close trigger but no content / no process → defer to Gemini.
	for _, input := range []string{
		"kod kapandı mı",
		"spotify kapatınca",
	} {
		if cmd := r.Resolve(input); cmd.Action == ActionRemindOnExit {
			t.Fatalf("Resolve(%q) should not be a complete remind, got %+v", input, cmd)
		}
	}
}

func TestNormalizeTurkish(t *testing.T) {
	got := normalize("EKRANI Kİlİtle!!!")
	want := "ekranı kilitle"
	if got != want {
		t.Fatalf("normalize = %q, want %q", got, want)
	}
}
