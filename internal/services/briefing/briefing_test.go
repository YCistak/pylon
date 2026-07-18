package briefing

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/YCistak/pylon/internal/intent"
)

// fakeDispatch returns a canned reply per action, or reports the action as
// unavailable (ok=false) when it has none — mirroring a service that is not
// configured.
type fakeDispatch struct {
	replies map[intent.Action]string
	errs    map[intent.Action]error
}

func (f fakeDispatch) Dispatch(_ context.Context, cmd intent.Command) (string, bool, error) {
	if err, ok := f.errs[cmd.Action]; ok {
		return "", true, err
	}
	if r, ok := f.replies[cmd.Action]; ok {
		return r, true, nil
	}
	return "", false, nil // unconfigured service
}

func briefingAt(t time.Time, d Dispatcher) *Service {
	return &Service{now: func() time.Time { return t }, sections: DefaultSections(), dispatch: d}
}

func TestComposeJoinsSectionsAfterGreeting(t *testing.T) {
	morning := time.Date(2026, 7, 19, 8, 0, 0, 0, time.UTC) // a Sunday
	s := briefingAt(morning, fakeDispatch{replies: map[intent.Action]string{
		"weather.today":        "İstanbul'da hava açık, şu an 24 derece.",
		"calendar.count_today": "Bugün 3 etkinliğin var.",
		"freshrss.unread_count": "12 okunmamış haberin var.",
	}})

	out, err := s.Execute(context.Background(), ActionToday, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Günaydın", "19 Temmuz Pazar", "24 derece", "3 etkinliğin", "12 okunmamış"} {
		if !strings.Contains(out, want) {
			t.Errorf("briefing %q missing %q", out, want)
		}
	}
	// Order: greeting first, then weather before news.
	if strings.Index(out, "Günaydın") != 0 {
		t.Errorf("greeting not first: %q", out)
	}
	if strings.Index(out, "derece") > strings.Index(out, "haberin") {
		t.Errorf("sections out of order: %q", out)
	}
}

// An unconfigured or failing section is dropped, and the briefing still reads as
// a whole sentence — a missing service must never blank the greeting.
func TestComposeSkipsUnavailableSections(t *testing.T) {
	morning := time.Date(2026, 7, 19, 8, 0, 0, 0, time.UTC)
	s := briefingAt(morning, fakeDispatch{
		replies: map[intent.Action]string{"weather.today": "Hava açık."},
		errs:    map[intent.Action]error{"freshrss.unread_count": errors.New("down")},
		// calendar.count_today absent → unconfigured
	})

	out, _ := s.Execute(context.Background(), ActionToday, nil)
	if !strings.Contains(out, "Günaydın") || !strings.Contains(out, "Hava açık") {
		t.Errorf("kept sections missing: %q", out)
	}
	if strings.Contains(out, "haberin") {
		t.Errorf("failed section leaked in: %q", out)
	}
}

func TestGreetingByHour(t *testing.T) {
	cases := map[int]string{3: "İyi geceler", 9: "Günaydın", 14: "İyi günler", 21: "İyi akşamlar"}
	for h, want := range cases {
		got := helloWord(time.Date(2026, 7, 19, h, 0, 0, 0, time.UTC))
		if got != want {
			t.Errorf("hour %d: got %q, want %q", h, got, want)
		}
	}
}
