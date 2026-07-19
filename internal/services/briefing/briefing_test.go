package briefing

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

type fakeCal struct {
	n   int
	err error
}

func (f fakeCal) TodayCount(context.Context) (int, error) { return f.n, f.err }

type fakeNews struct {
	n   int
	err error
}

func (f fakeNews) UnreadCount(context.Context) (int, error) { return f.n, f.err }

func briefingAt(t time.Time, cal CalendarSource, news NewsSource) *Service {
	s := &Service{now: func() time.Time { return t }}
	s.SetSources(cal, news)
	return s
}

func TestComposeJoinsClausesAfterGreeting(t *testing.T) {
	morning := time.Date(2026, 7, 19, 8, 0, 0, 0, time.UTC) // a Sunday
	s := briefingAt(morning, fakeCal{n: 3}, fakeNews{n: 12})

	out, err := s.Execute(context.Background(), ActionToday, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Günaydın", "19 Temmuz Pazar", "Takvimde 3 etkinlik var", "12 okunmamış haber var"} {
		if !strings.Contains(out, want) {
			t.Errorf("briefing %q missing %q", out, want)
		}
	}
	// Greeting first; calendar before news.
	if strings.Index(out, "Günaydın") != 0 {
		t.Errorf("greeting not first: %q", out)
	}
	if strings.Index(out, "Takvimde") > strings.Index(out, "okunmamış") {
		t.Errorf("clauses out of order: %q", out)
	}
	// The date's "Bugün" must not be echoed by a clause.
	if strings.Count(out, "Bugün") != 1 {
		t.Errorf("Bugün should appear once, got %q", out)
	}
}

// A nil source and an erroring source both drop their clause, and the briefing
// still reads as a whole — a missing source must never blank the greeting.
func TestComposeSkipsUnavailableSources(t *testing.T) {
	morning := time.Date(2026, 7, 19, 8, 0, 0, 0, time.UTC)
	s := briefingAt(morning, fakeCal{err: errors.New("down")}, nil) // cal errors, no news

	out, _ := s.Execute(context.Background(), ActionToday, nil)
	if !strings.Contains(out, "Günaydın") || !strings.Contains(out, "19 Temmuz Pazar") {
		t.Errorf("greeting missing: %q", out)
	}
	if strings.Contains(out, "Takvim") || strings.Contains(out, "haber") {
		t.Errorf("unavailable clause leaked in: %q", out)
	}
}

// An empty calendar is stated; zero unread news is not (nothing worth saying).
func TestComposeEmptyCalendarAndZeroNews(t *testing.T) {
	morning := time.Date(2026, 7, 19, 8, 0, 0, 0, time.UTC)
	s := briefingAt(morning, fakeCal{n: 0}, fakeNews{n: 0})

	out, _ := s.Execute(context.Background(), ActionToday, nil)
	if !strings.Contains(out, "Takvim boş") {
		t.Errorf("empty calendar not stated: %q", out)
	}
	if strings.Contains(out, "okunmamış") {
		t.Errorf("zero news should be dropped: %q", out)
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
