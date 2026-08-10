package briefing

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/YCistak/pylon/internal/i18n"
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
	for _, want := range []string{"Good morning", "Sunday, 19 July", "3 events in your calendar", "12 unread articles"} {
		if !strings.Contains(out, want) {
			t.Errorf("briefing %q missing %q", out, want)
		}
	}
	// Greeting first; calendar before news.
	if strings.Index(out, "Good morning") != 0 {
		t.Errorf("greeting not first: %q", out)
	}
	if strings.Index(out, "calendar") > strings.Index(out, "unread") {
		t.Errorf("clauses out of order: %q", out)
	}
	// The date's "Today" must not be echoed by a clause.
	if strings.Count(out, "Today") != 1 {
		t.Errorf("Today should appear once, got %q", out)
	}
}

// A nil source and an erroring source both drop their clause, and the briefing
// still reads as a whole — a missing source must never blank the greeting.
func TestComposeSkipsUnavailableSources(t *testing.T) {
	morning := time.Date(2026, 7, 19, 8, 0, 0, 0, time.UTC)
	s := briefingAt(morning, nil, fakeCal{err: errors.New("down")}, nil) // cal errors, no news

	out, _ := s.Execute(context.Background(), ActionToday, nil)
	if !strings.Contains(out, "Good morning") || !strings.Contains(out, "Sunday, 19 July") {
		t.Errorf("greeting missing: %q", out)
	}
	if strings.Contains(out, "calendar") || strings.Contains(out, "unread") {
		t.Errorf("unavailable clause leaked in: %q", out)
	}
}

// An empty calendar is stated; zero unread news is not (nothing worth saying).
func TestComposeEmptyCalendarAndZeroNews(t *testing.T) {
	morning := time.Date(2026, 7, 19, 8, 0, 0, 0, time.UTC)
	s := briefingAt(morning, nil, fakeCal{n: 0}, fakeNews{n: 0})

	out, _ := s.Execute(context.Background(), ActionToday, nil)
	if !strings.Contains(out, "calendar is empty") {
		t.Errorf("empty calendar not stated: %q", out)
	}
	if strings.Contains(out, "unread") {
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
	for _, want := range []string{"mostly clear", "21 degrees now and up to 29", "Chance of rain 40%"} {
		if !strings.Contains(out, want) {
			t.Errorf("briefing %q missing %q", out, want)
		}
	}
	if strings.Index(out, "mostly clear") > strings.Index(out, "calendar") {
		t.Errorf("weather should come before the calendar: %q", out)
	}
	// weather.Service phrases its own line with the place name; the briefing must
	// not be quoting that sentence.
	if strings.Contains(out, "degrees right now") {
		t.Errorf("briefing quoted the weather service's sentence: %q", out)
	}
}

// An unlikely shower isn't worth a clause, and a failed fetch drops the whole
// line rather than saying the weather is unavailable.
func TestComposeWeatherQuietOnLowRainAndFailure(t *testing.T) {
	morning := time.Date(2026, 7, 19, 8, 0, 0, 0, time.UTC)

	dry := fakeWx{f: weather.Forecast{TempNow: 24, Code: 0, High: 30, Low: 19, RainPct: 15, HaveDay: true}}
	out, _ := briefingAt(morning, dry, nil, nil).Execute(context.Background(), ActionToday, nil)
	if strings.Contains(out, "Chance of rain") {
		t.Errorf("15%% rain should stay unsaid: %q", out)
	}

	down := fakeWx{err: errors.New("open-meteo down")}
	out, _ = briefingAt(morning, down, fakeCal{n: 1}, nil).Execute(context.Background(), ActionToday, nil)
	if strings.Contains(out, "degrees") {
		t.Errorf("failed forecast should drop its clause: %q", out)
	}
	if !strings.Contains(out, "1 event in your calendar") {
		t.Errorf("other clauses must survive a weather failure: %q", out)
	}
}

// Without the daily block only the current temperature is stated — no "en
// yüksek 0 derece".
func TestComposeWeatherWithoutDailyFields(t *testing.T) {
	morning := time.Date(2026, 7, 19, 8, 0, 0, 0, time.UTC)
	wx := fakeWx{f: weather.Forecast{TempNow: 19, Code: 3}} // HaveDay false
	out, _ := briefingAt(morning, wx, nil, nil).Execute(context.Background(), ActionToday, nil)

	if !strings.Contains(out, "It is overcast, 19 degrees.") {
		t.Errorf("current conditions missing: %q", out)
	}
	if strings.Contains(out, "up to") {
		t.Errorf("daily fields absent but reported anyway: %q", out)
	}
}

func TestGreetingByHour(t *testing.T) {
	cases := map[int]string{3: "Good night", 9: "Good morning", 14: "Good afternoon", 21: "Good evening"}
	for h, want := range cases {
		got := helloWord(time.Date(2026, 7, 19, h, 0, 0, 0, time.UTC))
		if got != want {
			t.Errorf("hour %d: got %q, want %q", h, got, want)
		}
	}
}

// The whole briefing follows the active language, including the date — the one
// place where a half-translated sentence would show up as an English weekday in
// the middle of a Russian line.
func TestComposeSpeaksTheActiveLanguage(t *testing.T) {
	prev := i18n.Language()
	t.Cleanup(func() { i18n.SetLanguage(prev) })

	morning := time.Date(2026, 7, 19, 8, 0, 0, 0, time.UTC) // a Sunday
	s := briefingAt(morning, nil, fakeCal{n: 2}, nil)

	i18n.SetLanguage("tr")
	tr, _ := s.Execute(context.Background(), ActionToday, nil)
	for _, want := range []string{"Günaydın", "19 Temmuz Pazar", "Takvimde 2 etkinlik var"} {
		if !strings.Contains(tr, want) {
			t.Errorf("Turkish briefing %q missing %q", tr, want)
		}
	}

	i18n.SetLanguage("ru")
	ru, _ := s.Execute(context.Background(), ActionToday, nil)
	for _, want := range []string{"Доброе утро", "воскресенье", "19 июля", "В календаре 2 события"} {
		if !strings.Contains(ru, want) {
			t.Errorf("Russian briefing %q missing %q", ru, want)
		}
	}
	// Russian has a separate plural form for 2-4; 5 must not reuse it.
	five := briefingAt(morning, nil, fakeCal{n: 5}, nil)
	out, _ := five.Execute(context.Background(), ActionToday, nil)
	if !strings.Contains(out, "В календаре 5 событий") {
		t.Errorf("Russian many-form wrong: %q", out)
	}
}
