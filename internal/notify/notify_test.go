package notify

import (
	"context"
	"errors"
	"testing"
)

type capture struct {
	name string
	args []string
	env  []string
	err  error
}

func (c *capture) run(_ context.Context, name string, args, env []string) error {
	c.name = name
	c.args = append([]string(nil), args...)
	c.env = append([]string(nil), env...)
	return c.err
}

func TestNotifySubstitutesTitleAndBody(t *testing.T) {
	cap := &capture{}
	n := &cmdNotifier{cmd: []string{"notify-send", "-a", "Pylon", "{title}", "{body}"}, run: cap.run}
	if err := n.Notify(context.Background(), "Günaydın", "Bugün 2 etkinliğin var."); err != nil {
		t.Fatal(err)
	}
	if cap.name != "notify-send" {
		t.Fatalf("name %q", cap.name)
	}
	want := []string{"-a", "Pylon", "Günaydın", "Bugün 2 etkinliğin var."}
	if len(cap.args) != len(want) {
		t.Fatalf("args %v", cap.args)
	}
	for i := range want {
		if cap.args[i] != want[i] {
			t.Fatalf("arg %d = %q, want %q", i, cap.args[i], want[i])
		}
	}
}

// TestNotifyExportsTitleBodyEnv verifies title/body reach the command via
// PYLON_TITLE/PYLON_BODY environment variables — the safe channel for
// templates (like the macOS osascript default) that would otherwise need to
// embed untrusted content inside a scripted string.
func TestNotifyExportsTitleBodyEnv(t *testing.T) {
	cap := &capture{}
	n := &cmdNotifier{
		cmd: []string{"osascript", "-e", `display notification (system attribute "PYLON_BODY") with title (system attribute "PYLON_TITLE")`},
		run: cap.run,
	}
	title := `Pylon" -- injected`
	body := `"; do shell script "touch /tmp/pwned`
	if err := n.Notify(context.Background(), title, body); err != nil {
		t.Fatal(err)
	}
	wantTitleEnv, wantBodyEnv := "PYLON_TITLE="+title, "PYLON_BODY="+body
	var gotTitle, gotBody bool
	for _, e := range cap.env {
		if e == wantTitleEnv {
			gotTitle = true
		}
		if e == wantBodyEnv {
			gotBody = true
		}
	}
	if !gotTitle || !gotBody {
		t.Fatalf("env %v missing PYLON_TITLE/PYLON_BODY with malicious values", cap.env)
	}
	// The malicious content must never be substituted into the script arg —
	// only read out-of-band via `system attribute`.
	scriptArg := cap.args[len(cap.args)-1]
	if scriptArg != `display notification (system attribute "PYLON_BODY") with title (system attribute "PYLON_TITLE")` {
		t.Fatalf("script arg was mutated with untrusted content: %q", scriptArg)
	}
}

func TestNotifyEmptyCmdErrors(t *testing.T) {
	n := &cmdNotifier{cmd: nil, run: (&capture{}).run}
	if err := n.Notify(context.Background(), "t", "b"); err == nil {
		t.Fatal("expected error for empty command")
	}
}

func TestNotifyPropagatesRunError(t *testing.T) {
	cap := &capture{err: errors.New("boom")}
	n := &cmdNotifier{cmd: []string{"notify-send", "{title}", "{body}"}, run: cap.run}
	if err := n.Notify(context.Background(), "t", "b"); err == nil {
		t.Fatal("expected run error to propagate")
	}
}

func TestNewUsesDefaultWhenEmpty(t *testing.T) {
	// On the build platform, New("") should yield a usable notifier (non-nil).
	if New(nil) == nil {
		t.Fatal("New(nil) returned nil")
	}
}
