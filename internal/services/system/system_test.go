package system

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/YCistak/pylon/internal/intent"
)

// fakeRunner records commands and can fail selected ones.
type fakeRunner struct {
	calls  []string
	failIf func(name string, args []string) bool
}

func (f *fakeRunner) run(_ context.Context, name string, args ...string) error {
	f.calls = append(f.calls, strings.TrimSpace(name+" "+strings.Join(args, " ")))
	if f.failIf != nil && f.failIf(name, args) {
		return errors.New("boom")
	}
	return nil
}

func runAction(t *testing.T, fr *fakeRunner, action intent.Action, args map[string]string) string {
	t.Helper()
	s := &System{run: fr}
	out, err := s.Execute(context.Background(), action, args)
	if err != nil {
		t.Fatalf("Execute(%s): %v", action, err)
	}
	return out
}

func TestVolumeUpUnmutesThenRaises(t *testing.T) {
	fr := &fakeRunner{}
	got := runAction(t, fr, intent.ActionVolumeUp, nil)
	if got != "Sesi açtım." {
		t.Fatalf("reply %q", got)
	}
	want := []string{
		"pactl set-sink-mute @DEFAULT_SINK@ 0",
		"pactl set-sink-volume @DEFAULT_SINK@ +10%",
	}
	if strings.Join(fr.calls, "|") != strings.Join(want, "|") {
		t.Fatalf("calls = %v", fr.calls)
	}
}

func TestVolumeDownAndMute(t *testing.T) {
	fr := &fakeRunner{}
	if got := runAction(t, fr, intent.ActionVolumeDown, nil); got != "Sesi kıstım." {
		t.Fatalf("down reply %q", got)
	}
	if got := runAction(t, fr, intent.ActionMute, nil); got != "Sesi kapattım." {
		t.Fatalf("mute reply %q", got)
	}
	if fr.calls[0] != "pactl set-sink-volume @DEFAULT_SINK@ -10%" {
		t.Fatalf("down cmd %q", fr.calls[0])
	}
	if fr.calls[1] != "pactl set-sink-mute @DEFAULT_SINK@ 1" {
		t.Fatalf("mute cmd %q", fr.calls[1])
	}
}

func TestLockFallsBackToSecondCommand(t *testing.T) {
	// loginctl fails, xdg-screensaver succeeds → still locks.
	fr := &fakeRunner{failIf: func(name string, _ []string) bool { return name == "loginctl" }}
	if got := runAction(t, fr, intent.ActionLockScreen, nil); got != "Ekranı kilitliyorum." {
		t.Fatalf("lock reply %q", got)
	}
	if len(fr.calls) != 2 || !strings.HasPrefix(fr.calls[1], "xdg-screensaver") {
		t.Fatalf("expected fallback, calls=%v", fr.calls)
	}
}

func TestLockBothFail(t *testing.T) {
	fr := &fakeRunner{failIf: func(string, []string) bool { return true }}
	if got := runAction(t, fr, intent.ActionLockScreen, nil); got != "Ekranı kilitleyemedim." {
		t.Fatalf("lock reply %q", got)
	}
}

func TestMediaControls(t *testing.T) {
	cases := []struct {
		action intent.Action
		verb   string
		reply  string
	}{
		{intent.ActionMediaPlay, "play", "Oynatıyorum."},
		{intent.ActionMediaPause, "pause", "Durdurdum."},
		{intent.ActionMediaNext, "next", "Sonraki parçaya geçtim."},
		{intent.ActionMediaPrev, "previous", "Önceki parçaya döndüm."},
	}
	for _, c := range cases {
		fr := &fakeRunner{}
		if got := runAction(t, fr, c.action, nil); got != c.reply {
			t.Errorf("%s reply %q", c.action, got)
		}
		if fr.calls[0] != "playerctl "+c.verb {
			t.Errorf("%s cmd %q", c.action, fr.calls[0])
		}
	}
}

func TestCloseApp(t *testing.T) {
	fr := &fakeRunner{}
	if got := runAction(t, fr, ActionClose, map[string]string{"app": "chrome"}); got != "chrome kapatıldı." {
		t.Fatalf("reply %q", got)
	}
	if fr.calls[0] != "pkill chrome" {
		t.Fatalf("cmd %q", fr.calls[0])
	}
}

func TestCloseGuards(t *testing.T) {
	fr := &fakeRunner{}
	if got := runAction(t, fr, ActionClose, nil); got != "Neyi kapatayım?" {
		t.Errorf("empty app: %q", got)
	}
	if got := runAction(t, fr, ActionClose, map[string]string{"app": "Pylon"}); got != "Kendimi kapatamam." {
		t.Errorf("self-close guard: %q", got)
	}
	if len(fr.calls) != 0 {
		t.Fatalf("guards should not run any command, got %v", fr.calls)
	}
}

func TestCloseFailureGraceful(t *testing.T) {
	fr := &fakeRunner{failIf: func(string, []string) bool { return true }}
	got := runAction(t, fr, ActionClose, map[string]string{"app": "code"})
	if got != "code kapatılamadı ya da zaten kapalı." {
		t.Fatalf("reply %q", got)
	}
}

func TestUnknownAction(t *testing.T) {
	s := &System{run: &fakeRunner{}}
	if _, err := s.Execute(context.Background(), "system.bogus", nil); err == nil {
		t.Fatal("expected error")
	}
}

func TestActionsCoverRouterIntents(t *testing.T) {
	// The system service must own every media/lock action the local router emits,
	// or those commands would silently do nothing again.
	owned := map[intent.Action]bool{}
	for _, a := range New().Actions() {
		owned[a.Name] = true
	}
	for _, a := range []intent.Action{
		intent.ActionLockScreen, intent.ActionMediaPlay, intent.ActionMediaPause,
		intent.ActionMediaNext, intent.ActionMediaPrev,
		intent.ActionVolumeUp, intent.ActionVolumeDown, intent.ActionMute,
	} {
		if !owned[a] {
			t.Errorf("system service does not own router action %q", a)
		}
	}
}
