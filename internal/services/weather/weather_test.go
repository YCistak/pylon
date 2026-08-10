package weather

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type fakeForecaster struct {
	f   Forecast
	err error
}

func (f fakeForecaster) forecast(context.Context, float64, float64) (Forecast, error) {
	return f.f, f.err
}

func withAPI(place string, api forecaster) *Service {
	return &Service{lat: 1, lon: 1, place: place, located: true, api: api}
}

func TestSpeakFullForecast(t *testing.T) {
	s := withAPI("İstanbul", fakeForecaster{f: Forecast{
		TempNow: 24, Code: 1, High: 27, Low: 18, RainPct: 40, HaveDay: true,
	}})
	out, err := s.Execute(context.Background(), ActionToday, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"İstanbul", "mostly clear", "24 degrees", "high is 27", "low 18", "40%"} {
		if !strings.Contains(out, want) {
			t.Errorf("forecast %q missing %q", out, want)
		}
	}
}

// No rain probability must not speak "%0" — silence is the honest rendering of
// a dry outlook.
func TestSpeakDropsZeroRain(t *testing.T) {
	s := withAPI("Ankara", fakeForecaster{})
	out := s.speak(Forecast{TempNow: 10, Code: 0, High: 12, Low: 3, RainPct: 0, HaveDay: true})
	if strings.Contains(out, "rain") {
		t.Errorf("spoke a zero rain chance: %q", out)
	}
	if !strings.Contains(out, "clear") {
		t.Errorf("missing condition: %q", out)
	}
}

func TestExecuteErrorIsGraceful(t *testing.T) {
	s := withAPI("İstanbul", fakeForecaster{err: context.DeadlineExceeded})
	out, err := s.Execute(context.Background(), ActionToday, nil)
	if err != nil {
		t.Fatalf("Execute should not return an error to the user: %v", err)
	}
	if !strings.Contains(out, "can't reach") {
		t.Errorf("expected a graceful message, got %q", out)
	}
}

func TestDescribeKnownAndUnknown(t *testing.T) {
	if Describe(0) != "clear" || Describe(65) != "rainy" || Describe(75) != "snowy" {
		t.Error("known WMO codes mis-described")
	}
	if Describe(1234) != "changeable" {
		t.Errorf("unknown code should fall back, got %q", Describe(1234))
	}
}

// Exercise the real JSON decode path against an Open-Meteo-shaped fixture,
// served locally so no network is touched.
func TestHTTPForecasterParsesResponse(t *testing.T) {
	const body = `{"current":{"temperature_2m":24.0,"weather_code":3},
		"daily":{"temperature_2m_max":[27.5],"temperature_2m_min":[18.1],"precipitation_probability_max":[55]}}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	h := &httpForecaster{client: srv.Client(), baseURL: srv.URL}
	f, err := h.forecast(context.Background(), 41, 29)
	if err != nil {
		t.Fatal(err)
	}
	if f.TempNow != 24 || f.Code != 3 || f.High != 27.5 || f.Low != 18.1 || f.RainPct != 55 || !f.HaveDay {
		t.Errorf("parsed forecast wrong: %+v", f)
	}
}

// With no coordinates the service says so instead of reporting the weather
// somewhere the user has never been. It used to default to İstanbul, which was
// right for one person and wrong for everyone who installs Pylon.
func TestNoCoordinatesIsStatedNotGuessed(t *testing.T) {
	s := New(0, 0, "")
	if s.located {
		t.Fatal("a zero location should not count as configured")
	}

	if _, err := s.Today(context.Background()); !errors.Is(err, ErrNoLocation) {
		t.Errorf("Today err = %v, want ErrNoLocation", err)
	}

	out, err := s.Execute(context.Background(), ActionToday, nil)
	if err != nil {
		t.Fatalf("Execute should answer, not error: %v", err)
	}
	if !strings.Contains(out, "No location") {
		t.Errorf("reply %q should say the location is missing", out)
	}
}

// A configured location is used as given, including a place name for the reply.
func TestCoordinatesMakeItLocated(t *testing.T) {
	s := New(38.42, 27.14, "İzmir")
	if !s.located || s.place != "İzmir" {
		t.Errorf("located=%v place=%q", s.located, s.place)
	}
}
