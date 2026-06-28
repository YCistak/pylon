package services

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/YCistak/pylon/internal/intent"
)

type fakeService struct {
	name    string
	actions []intent.Action
	reply   string
	err     error
	gotArgs map[string]string
}

func (f *fakeService) Name() string { return f.name }
func (f *fakeService) Actions() []intent.ActionSpec {
	var out []intent.ActionSpec
	for _, a := range f.actions {
		out = append(out, intent.ActionSpec{Name: a, Desc: string(a)})
	}
	return out
}
func (f *fakeService) Execute(_ context.Context, _ intent.Action, args map[string]string) (string, error) {
	f.gotArgs = args
	return f.reply, f.err
}

func TestRegistrySpecsAndDispatch(t *testing.T) {
	cal := &fakeService{name: "calendar", actions: []intent.Action{"calendar.list_today"}, reply: "boş"}
	gh := &fakeService{name: "github", actions: []intent.Action{"github.list_prs"}, reply: "2 PR"}
	r := NewRegistry(cal, gh, nil) // nil entry must be skipped

	if len(r.Specs()) != 2 {
		t.Fatalf("specs = %d, want 2", len(r.Specs()))
	}

	text, ok, err := r.Dispatch(context.Background(), intent.Command{
		Action: "github.list_prs", Args: map[string]string{"x": "y"},
	})
	if err != nil || !ok {
		t.Fatalf("dispatch: ok=%v err=%v", ok, err)
	}
	if text != "2 PR" {
		t.Fatalf("text = %q", text)
	}
	if gh.gotArgs["x"] != "y" {
		t.Fatalf("args not passed: %+v", gh.gotArgs)
	}

	// Unknown action → ok=false (caller falls back).
	if _, ok, _ := r.Dispatch(context.Background(), intent.Command{Action: "media.play"}); ok {
		t.Fatal("unowned action should not dispatch")
	}
}

func TestRegistryWrapsServiceError(t *testing.T) {
	bad := &fakeService{name: "calendar", actions: []intent.Action{"calendar.add_event"}, err: errors.New("api down")}
	r := NewRegistry(bad)
	_, ok, err := r.Dispatch(context.Background(), intent.Command{Action: "calendar.add_event"})
	if !ok || err == nil {
		t.Fatalf("expected owned+error, got ok=%v err=%v", ok, err)
	}
	if !strings.Contains(err.Error(), "calendar") {
		t.Fatalf("error should name the service: %v", err)
	}
}
