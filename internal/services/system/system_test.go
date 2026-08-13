package system

import (
	"context"
	"errors"
	"runtime"
	"strings"
	"testing"

	"github.com/YCistak/pylon/internal/intent"
)

// fakeRunner records commands and can fail selected ones.
type fakeRunner struct {
	calls  []string
	failIf func(name string, args []string) bool
	out    string // what output() returns when it is not made to fail
}

func (f *fakeRunner) run(_ context.Context, name string, args ...string) error {
	f.calls = append(f.calls, strings.TrimSpace(name+" "+strings.Join(args, " ")))
	if f.failIf != nil && f.failIf(name, args) {
		return errors.New("boom")
	}
	return nil
}

func (f *fakeRunner) output(_ context.Context, name string, args ...string) (string, error) {
	f.calls = append(f.calls, strings.TrimSpace(name+" "+strings.Join(args, " ")))
	if f.failIf != nil && f.failIf(name, args) {
		return "", errors.New("boom")
	}
	return f.out, nil
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
	if got != "Volume up." {
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
	if got := runAction(t, fr, intent.ActionVolumeDown, nil); got != "Volume down." {
		t.Fatalf("down reply %q", got)
	}
	if got := runAction(t, fr, intent.ActionMute, nil); got != "Muted." {
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
	if got := runAction(t, fr, intent.ActionLockScreen, nil); got != "Locking the screen." {
		t.Fatalf("lock reply %q", got)
	}
	if len(fr.calls) != 2 || !strings.HasPrefix(fr.calls[1], "xdg-screensaver") {
		t.Fatalf("expected fallback, calls=%v", fr.calls)
	}
}

func TestLockBothFail(t *testing.T) {
	fr := &fakeRunner{failIf: func(string, []string) bool { return true }}
	if got := runAction(t, fr, intent.ActionLockScreen, nil); got != "I couldn't lock the screen." {
		t.Fatalf("lock reply %q", got)
	}
}

func TestMediaControls(t *testing.T) {
	cases := []struct {
		action intent.Action
		verb   string
		reply  string
	}{
		{intent.ActionMediaPlay, "play", "Playing."},
		{intent.ActionMediaPause, "pause", "Paused."},
		{intent.ActionMediaNext, "next", "Next track."},
		{intent.ActionMediaPrev, "previous", "Previous track."},
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
	if got := runAction(t, fr, ActionClose, map[string]string{"app": "chrome"}); got != "chrome closed." {
		t.Fatalf("reply %q", got)
	}
	// -x, so the pattern has to match the whole process name.
	if fr.calls[0] != "pkill -x chrome" {
		t.Fatalf("cmd %q", fr.calls[0])
	}
}

func TestCloseGuards(t *testing.T) {
	fr := &fakeRunner{}
	if got := runAction(t, fr, ActionClose, nil); got != "What should I close?" {
		t.Errorf("empty app: %q", got)
	}
	if got := runAction(t, fr, ActionClose, map[string]string{"app": "Pylon"}); got != "I can't close myself." {
		t.Errorf("self-close guard: %q", got)
	}
	if len(fr.calls) != 0 {
		t.Fatalf("guards should not run any command, got %v", fr.calls)
	}
}

func TestCloseFailureGraceful(t *testing.T) {
	fr := &fakeRunner{failIf: func(string, []string) bool { return true }}
	got := runAction(t, fr, ActionClose, map[string]string{"app": "code"})
	if got != "code couldn't be closed, or wasn't running." {
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

// Now-playing reads whatever MPRIS player is running, so the cases that matter
// are the shapes a player can hand back — not any one application.
func TestNowPlaying(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("MPRIS is Linux-only; the other platforms have no backend yet")
	}
	cases := []struct {
		name string
		out  string
		fail bool
		want string
	}{
		{name: "playing", out: "Playing\tRadiohead\tWeird Fishes\n", want: "Radiohead — Weird Fishes is playing."},
		{name: "paused", out: "Paused\tRadiohead\tWeird Fishes\n", want: "Radiohead — Weird Fishes — paused."},
		// A browser tab or a stream often has no artist; "— Title" would read
		// as a missing word.
		{name: "no artist", out: "Playing\t\tSome Video\n", want: "Some Video is playing."},
		// A title containing a dash must survive: the format is tab-separated
		// for exactly this.
		{name: "dash in title", out: "Playing\tX\tA - B\n", want: "X — A - B is playing."},
		// playerctl exits non-zero when no player is running at all. That is an
		// answer to the question, not a failure to report.
		{name: "no player", fail: true, want: "Nothing is playing."},
		{name: "empty metadata", out: "Stopped\t\t\n", want: "Nothing is playing."},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fr := &fakeRunner{out: c.out}
			if c.fail {
				fr.failIf = func(name string, _ []string) bool { return name == "playerctl" }
			}
			if got := runAction(t, fr, intent.ActionNowPlaying, nil); got != c.want {
				t.Errorf("reply %q, want %q", got, c.want)
			}
		})
	}
}

// pkill takes an extended regular expression, not a literal, and the name comes
// from a language model. `pkill .` matched 479 of 480 processes on the machine
// where this was found — the user's whole session — and `-x` alone does not
// help, because `.*` still matches every name there is.
func TestCloseRefusesPatternsThatAreNotNames(t *testing.T) {
	for _, app := range []string{
		".", "..", ".*", "^.*$", "chrome|pylon", "a b", "code;rm", "$(id)",
		"*", "[a-z]+", "chr?me", "(chrome)", "chrome$", strings.Repeat("a", 65),
	} {
		fr := &fakeRunner{}
		got := runAction(t, fr, ActionClose, map[string]string{"app": app})
		if got != "That is not a program name I can act on." {
			t.Errorf("%q: reply %q", app, got)
		}
		if len(fr.calls) != 0 {
			t.Errorf("%q: ran %v — nothing should have been killed", app, fr.calls)
		}
	}
}

// Surrounding whitespace is the model's, not the user's intent, and is trimmed
// before the name is judged.
func TestCloseTrimsBeforeJudging(t *testing.T) {
	fr := &fakeRunner{}
	runAction(t, fr, ActionClose, map[string]string{"app": "  chrome  "})
	if len(fr.calls) != 1 || fr.calls[0] != "pkill -x chrome" {
		t.Fatalf("calls %v", fr.calls)
	}
}

// isSelf compares literals while pkill matches patterns, so the guard against
// Pylon killing itself was walked past by spelling the name as a pattern.
func TestCloseSelfGuardIsNotEvadedByAPattern(t *testing.T) {
	// Refused outright: they carry metacharacters isProcessName rejects.
	for _, app := range []string{"pylon|x", "pylon.*", "^pylon$", "pylo?n"} {
		fr := &fakeRunner{}
		runAction(t, fr, ActionClose, map[string]string{"app": app})
		if len(fr.calls) != 0 {
			t.Errorf("%q reached pkill: %v", app, fr.calls)
		}
	}
	// "pylo." looks like a name and passes every name check — the dot is the
	// one metacharacter that survives, because real names contain it. It is
	// defused by escaping instead: pkill -x 'pylo\.' matches a process
	// literally called "pylo.", which is nothing.
	fr := &fakeRunner{}
	runAction(t, fr, ActionClose, map[string]string{"app": "pylo."})
	if len(fr.calls) != 1 || fr.calls[0] != `pkill -x pylo\.` {
		t.Fatalf("calls %v", fr.calls)
	}
}

// Escaping has to leave real names intact — mount.ntfs-3g and python3.11 are on
// an ordinary desktop.
func TestCloseEscapesDotsInRealNames(t *testing.T) {
	fr := &fakeRunner{}
	runAction(t, fr, ActionClose, map[string]string{"app": "mount.ntfs-3g"})
	if len(fr.calls) != 1 || fr.calls[0] != `pkill -x mount\.ntfs-3g` {
		t.Fatalf("calls %v", fr.calls)
	}
}

// Real names have to keep working, or the guard has just removed the feature.
func TestCloseAllowsRealProcessNames(t *testing.T) {
	for _, app := range []string{"code", "chrome", "gnome-shell", "google_chrome", "Discord", "node20", "a.out"} {
		fr := &fakeRunner{}
		runAction(t, fr, ActionClose, map[string]string{"app": app})
		if len(fr.calls) != 1 || fr.calls[0] != "pkill -x "+killPattern(app) {
			t.Errorf("%q: calls %v", app, fr.calls)
		}
	}
}
