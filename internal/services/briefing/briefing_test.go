package briefing

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/YCistak/pylon/internal/services/weather"
)

type fakeWx struct {
	f   weather.Forecast
	err error
}

func (f fakeWx) Today(context.Context) (weather.Forecast, error) { return f.f, f.err }

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

func briefingAt(t time.Time, wx WeatherSource, cal CalendarSource, news NewsSource) *Service {
	s := &Service{now: func() time.Time { return t }}
	s.SetSources(wx, cal, news)
	return s
}

func TestComposeJoinsClausesAfterGreeting(t *testing.T) {
	morning := time.Date(2026, 7, 19, 8, 0, 0, 0, time.UTC) // a Sunday
	s := briefingAt(morning, nil, fakeCal{n: 3}, fakeNews{n: 12})

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
	s := briefingAt(morning, nil, fakeCal{err: errors.New("down")}, nil) // cal errors, no news

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
	s := briefingAt(morning, nil, fakeCal{n: 0}, fakeNews{n: 0})

	out, _ := s.Execute(context.Background(), ActionToday, nil)
	if !strings.Contains(out, "Takvim boş") {
		t.Errorf("empty calendar not stated: %q", out)
	}
	if strings.Contains(out, "okunmamış") {
		t.Errorf("zero news should be dropped: %q", out)
	}
}

// Weather leads the clauses, and its numbers are phrased by the briefing rather
// than quoted from the weather service's own sentence.
func TestComposeWeatherLeadsAndIsShort(t *testing.T) {
	morning := time.Date(2026, 7, 19, 8, 0, 0, 0, time.UTC)
	wx := fakeWx{f: weather.Forecast{TempNow: 21.4, Code: 1, High: 28.6, Low: 17, RainPct: 40, HaveDay: true}}
	s := briefingAt(morning, wx, fakeCal{n: 2}, nil)

	out, _ := s.Execute(context.Background(), ActionToday, nil)
	for _, want := range []string{"Hava az bulutlu", "şu an 21, en yüksek 29 derece", "Yağış ihtimali %40"} {
		if !strings.Contains(out, want) {
			t.Errorf("briefing %q missing %q", out, want)
		}
	}
	if strings.Index(out, "Hava") > strings.Index(out, "Takvimde") {
		t.Errorf("weather should come before the calendar: %q", out)
	}
	// weather.Service phrases its own line with the place name; the briefing must
	// not be quoting that sentence.
	if strings.Contains(out, "'da hava") {
		t.Errorf("briefing quoted the weather service's sentence: %q", out)
	}
}

// An unlikely shower isn't worth a clause, and a failed fetch drops the whole
// line rather than saying the weather is unavailable.
func TestComposeWeatherQuietOnLowRainAndFailure(t *testing.T) {
	morning := time.Date(2026, 7, 19, 8, 0, 0, 0, time.UTC)

	dry := fakeWx{f: weather.Forecast{TempNow: 24, Code: 0, High: 30, Low: 19, RainPct: 15, HaveDay: true}}
	out, _ := briefingAt(morning, dry, nil, nil).Execute(context.Background(), ActionToday, nil)
	if strings.Contains(out, "Yağış") {
		t.Errorf("15%% rain should stay unsaid: %q", out)
	}

	down := fakeWx{err: errors.New("open-meteo down")}
	out, _ = briefingAt(morning, down, fakeCal{n: 1}, nil).Execute(context.Background(), ActionToday, nil)
	if strings.Contains(out, "Hava") {
		t.Errorf("failed forecast should drop its clause: %q", out)
	}
	if !strings.Contains(out, "Takvimde 1 etkinlik") {
		t.Errorf("other clauses must survive a weather failure: %q", out)
	}
}

// Without the daily block only the current temperature is stated — no "en
// yüksek 0 derece".
func TestComposeWeatherWithoutDailyFields(t *testing.T) {
	morning := time.Date(2026, 7, 19, 8, 0, 0, 0, time.UTC)
	wx := fakeWx{f: weather.Forecast{TempNow: 19, Code: 3}} // HaveDay false
	out, _ := briefingAt(morning, wx, nil, nil).Execute(context.Background(), ActionToday, nil)

	if !strings.Contains(out, "Hava çok bulutlu, şu an 19 derece.") {
		t.Errorf("current conditions missing: %q", out)
	}
	if strings.Contains(out, "en yüksek") {
		t.Errorf("daily fields absent but reported anyway: %q", out)
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
