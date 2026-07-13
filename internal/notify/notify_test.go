package notify

import (
	"context"
	"errors"
	"testing"
)

type capture struct {
	name string
	args []string
	err  error
}

func (c *capture) run(_ context.Context, name string, args []string) error {
	c.name = name
	c.args = append([]string(nil), args...)
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

func TestNotifySubstitutesInsideSingleArg(t *testing.T) {
	cap := &capture{}
	n := &cmdNotifier{cmd: []string{"osascript", "-e", `display notification "{body}" with title "{title}"`}, run: cap.run}
	_ = n.Notify(context.Background(), "Pylon", "merhaba")
	if cap.args[1] != `display notification "merhaba" with title "Pylon"` {
		t.Fatalf("scripted arg %q", cap.args[1])
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
