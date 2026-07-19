package banner

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

func TestShowPipesTextAndSplitsCommand(t *testing.T) {
	var gotStdin []byte
	var gotName string
	var gotArgs []string
	p := &Presenter{
		cmd: []string{"python3", "scripts/briefing_banner.py", "--foo"},
		run: func(stdin []byte, name string, args []string) error {
			gotStdin, gotName, gotArgs = stdin, name, args
			return nil
		},
	}

	if err := p.Show(context.Background(), "  merhaba dünya  "); err != nil {
		t.Fatal(err)
	}
	if string(gotStdin) != "merhaba dünya" {
		t.Errorf("stdin = %q, want trimmed %q", gotStdin, "merhaba dünya")
	}
	if gotName != "python3" {
		t.Errorf("name = %q, want python3", gotName)
	}
	if want := []string{"scripts/briefing_banner.py", "--foo"}; !reflect.DeepEqual(gotArgs, want) {
		t.Errorf("args = %v, want %v", gotArgs, want)
	}
}

func TestShowNoOpWhenDisabledOrEmpty(t *testing.T) {
	called := false
	mark := func([]byte, string, []string) error { called = true; return nil }

	// Empty command → disabled.
	off := &Presenter{cmd: nil, run: mark}
	if err := off.Show(context.Background(), "text"); err != nil {
		t.Fatal(err)
	}
	// Blank text → nothing to show.
	on := &Presenter{cmd: []string{"echo"}, run: mark}
	if err := on.Show(context.Background(), "   "); err != nil {
		t.Fatal(err)
	}
	if called {
		t.Error("run should not be called for a disabled banner or empty text")
	}
}

func TestShowPropagatesRunError(t *testing.T) {
	p := &Presenter{
		cmd: []string{"nope"},
		run: func([]byte, string, []string) error { return errors.New("boom") },
	}
	if err := p.Show(context.Background(), "hi"); err == nil {
		t.Error("expected error from run to propagate")
	}
}
