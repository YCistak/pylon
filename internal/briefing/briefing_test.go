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

func TestHelloWordByTimeOfDay(t *testing.T) {
	cases := map[int]string{2: "İyi geceler", 9: "Günaydın", 14: "İyi günler", 21: "İyi akşamlar"}
	for hour, want := range cases {
		if g := helloWord(time.Date(2026, time.July, 13, hour, 0, 0, 0, time.UTC)); g != want {
			t.Errorf("hour %d: got %q, want %q", hour, g, want)
		}
	}
}

func TestNotificationSplitsTitleAndBody(t *testing.T) {
	svc := New()
	svc.now = fixedClock(mondayMorning)
	svc.SetDispatcher(fakeDispatch{
		"freshrss.unread_count": {text: "6069 okunmamış haberin var.", ok: true},
	})
	title, body := svc.Notification(context.Background())
	if title != "Günaydın" {
		t.Fatalf("title %q", title)
	}
	if body != "Bugün 13 Temmuz Pazartesi. 6069 okunmamış haberin var." {
		t.Fatalf("body %q", body)
	}
	// Build stitches them back with a period so the spoken form is unchanged.
	if got := svc.Build(context.Background()); got != title+". "+body {
		t.Fatalf("Build %q not title+body", got)
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
