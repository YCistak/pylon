package briefing

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/YCistak/pylon/internal/intent"
)

// fakeDispatch answers a fixed reply/ok/err per action.
type fakeDispatch map[intent.Action]struct {
	text string
	ok   bool
	err  error
}

func (f fakeDispatch) Dispatch(_ context.Context, cmd intent.Command) (string, bool, error) {
	r, found := f[cmd.Action]
	if !found {
		return "", false, nil // no service owns it
	}
	return r.text, r.ok, r.err
}

func fixedClock(t time.Time) func() time.Time { return func() time.Time { return t } }

// A Monday morning in July.
var mondayMorning = time.Date(2026, time.July, 13, 8, 0, 0, 0, time.UTC)

func TestBuildComposesAvailableSections(t *testing.T) {
	svc := New()
	svc.now = fixedClock(mondayMorning)
	svc.SetDispatcher(fakeDispatch{
		"calendar.list_today":  {text: "Bugün 2 etkinliğin var.", ok: true},
		"github.list_prs":      {text: "3 açık PR var.", ok: true},
		"freshrss.unread_count": {text: "5 okunmamış haberin var.", ok: true},
	})

	got := svc.Build(context.Background())
	want := "Günaydın. Bugün 13 Temmuz Pazartesi. Bugün 2 etkinliğin var. 3 açık PR var. 5 okunmamış haberin var."
	if got != want {
		t.Fatalf("briefing text\n got: %q\nwant: %q", got, want)
	}
}

func TestBuildSkipsUnavailableAndErroredSections(t *testing.T) {
	svc := New()
	svc.now = fixedClock(mondayMorning)
	svc.SetDispatcher(fakeDispatch{
		// calendar not configured → ok=false, dropped
		"github.list_prs":       {text: "3 açık PR var.", ok: true},
		"freshrss.unread_count": {text: "", ok: true, err: errors.New("boom")}, // errored → dropped
	})

	got := svc.Build(context.Background())
	want := "Günaydın. Bugün 13 Temmuz Pazartesi. 3 açık PR var."
	if got != want {
		t.Fatalf("briefing text\n got: %q\nwant: %q", got, want)
	}
}

func TestBuildGreetingOnlyWhenNothingAvailable(t *testing.T) {
	svc := New()
	svc.now = fixedClock(mondayMorning)
	svc.SetDispatcher(fakeDispatch{}) // nothing owns any action

	got := svc.Build(context.Background())
	if !strings.HasPrefix(got, "Günaydın.") || strings.Count(got, ".") != 2 {
		t.Fatalf("expected greeting-only, got %q", got)
	}
}

func TestGreetingByTimeOfDay(t *testing.T) {
	cases := map[int]string{2: "İyi geceler.", 9: "Günaydın.", 14: "İyi günler.", 21: "İyi akşamlar."}
	for hour, prefix := range cases {
		g := greeting(time.Date(2026, time.July, 13, hour, 0, 0, 0, time.UTC))
		if !strings.HasPrefix(g, prefix) {
			t.Errorf("hour %d: got %q, want prefix %q", hour, g, prefix)
		}
	}
}

func TestExecuteRunsBuild(t *testing.T) {
	svc := New()
	svc.now = fixedClock(mondayMorning)
	svc.SetDispatcher(fakeDispatch{})
	out, err := svc.Execute(context.Background(), ActionToday, nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.HasPrefix(out, "Günaydın.") {
		t.Fatalf("Execute output %q", out)
	}
	if _, err := svc.Execute(context.Background(), "briefing.bogus", nil); err == nil {
		t.Fatal("expected error for unknown action")
	}
}
