// Package weather reports the local forecast via Open-Meteo — free, no API key,
// no quota. It speaks the current conditions plus today's high, low and rain
// chance for a configured location ("İstanbul'da hava az bulutlu, şu an 24
// derece…").
package weather

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/YCistak/pylon/internal/i18n"
	"github.com/YCistak/pylon/internal/intent"
)

// ActionToday reports the current conditions and today's outlook.
const ActionToday intent.Action = "weather.today"

// ErrNoLocation means no coordinates were configured. Callers that compose
// their own text (the briefing) tell it apart from a network failure this way.
var ErrNoLocation = errors.New("weather: no location configured")

// Forecast is the slice of an Open-Meteo response Pylon speaks. It is exported
// whole because the briefing reads the raw numbers and phrases its own, shorter
// clause from them (see internal/services/briefing).
type Forecast struct {
	TempNow float64 // current temperature, °C
	Code    int     // WMO weather code
	High    float64 // today's max, °C
	Low     float64 // today's min, °C
	RainPct int     // today's max precipitation probability, %
	HaveDay bool    // whether the daily fields were present
}

// forecaster fetches a forecast for a location; the HTTP implementation is
// swapped for a fake in tests.
type forecaster interface {
	forecast(ctx context.Context, lat, lon float64) (Forecast, error)
}

// Service reports the weather for a fixed location. It needs no key, so it is
// always registered — but it does need coordinates, and says so when it has
// none.
type Service struct {
	lat, lon float64
	place    string // display name, e.g. "İstanbul"
	located  bool   // whether coordinates were configured
	api      forecaster
}

// New builds the service. A zero lat/lon leaves it unconfigured rather than
// guessing: the service used to fall back to İstanbul, which is a sensible
// default for exactly one user and a confusing one for everybody else — someone
// in Lisbon asking about the weather should be told to set a location, not
// handed a forecast for a city 3000 km away.
func New(lat, lon float64, place string) *Service {
	located := lat != 0 || lon != 0
	if strings.TrimSpace(place) == "" {
		place = i18n.T("weather.here")
	}
	return &Service{lat: lat, lon: lon, place: place, located: located, api: &httpForecaster{
		client: &http.Client{Timeout: 8 * time.Second},
	}}
}

func (s *Service) Name() string { return "weather" }

func (s *Service) Actions() []intent.ActionSpec {
	return []intent.ActionSpec{
		{
			Name: ActionToday,
			Desc: `"weather.today": report the local weather — current temperature and condition, plus today's high, low and rain chance. No args. Use for "hava nasıl", "bugün hava", "yağmur var mı", "dışarı çıkayım mı", "kaç derece".`,
		},
	}
}

// Today fetches the raw forecast for the configured location. The spoken action
// goes through it too; it exists separately so the briefing can read the numbers
// and word its own clause instead of quoting a whole sentence.
//
// With no coordinates it errors rather than fetching, which is what drops the
// briefing's weather clause instead of putting a stranger's city in it.
func (s *Service) Today(ctx context.Context) (Forecast, error) {
	if !s.located {
		return Forecast{}, ErrNoLocation
	}
	return s.api.forecast(ctx, s.lat, s.lon)
}

func (s *Service) Execute(ctx context.Context, action intent.Action, _ map[string]string) (string, error) {
	switch action {
	case ActionToday:
		f, err := s.Today(ctx)
		switch {
		case errors.Is(err, ErrNoLocation):
			return i18n.T("weather.no_location"), nil
		case err != nil:
			return i18n.T("weather.unavailable"), nil
		}
		return s.speak(f), nil
	default:
		return "", fmt.Errorf("weather: unknown action %q", action)
	}
}

// speak renders a forecast as one line in the active language.
func (s *Service) speak(f Forecast) string {
	out := i18n.T("weather.now", s.place, Describe(f.Code), f.TempNow)
	if f.HaveDay {
		out += " " + i18n.T("weather.today", f.High, f.Low)
		if f.RainPct > 0 {
			out += " " + i18n.T("weather.rain", f.RainPct)
		}
	}
	return out
}

// Describe maps a WMO weather code to a phrase in the active language. Codes
// group naturally (drizzle 51-57, rain 61-67, snow 71-77, showers 80-82);
// unknown codes get a neutral fallback rather than an empty string.
func Describe(code int) string {
	switch code {
	case 0:
		return i18n.T("weather.code.clear")
	case 1:
		return i18n.T("weather.code.mostly_clear")
	case 2:
		return i18n.T("weather.code.partly_cloudy")
	case 3:
		return i18n.T("weather.code.overcast")
	case 45, 48:
		return i18n.T("weather.code.fog")
	case 51, 53, 55, 56, 57:
		return i18n.T("weather.code.drizzle")
	case 61, 63, 65, 66, 67:
		return i18n.T("weather.code.rain")
	case 71, 73, 75, 77:
		return i18n.T("weather.code.snow")
	case 80, 81, 82:
		return i18n.T("weather.code.showers")
	case 85, 86:
		return i18n.T("weather.code.snow_showers")
	case 95:
		return i18n.T("weather.code.thunderstorm")
	case 96, 99:
		return i18n.T("weather.code.thunderstorm_hail")
	default:
		return i18n.T("weather.code.unknown")
	}
}

// httpForecaster is the production forecaster, hitting Open-Meteo.
type httpForecaster struct {
	client  *http.Client
	baseURL string // overridden in tests; empty means the real endpoint
}

func (h *httpForecaster) forecast(ctx context.Context, lat, lon float64) (Forecast, error) {
	base := h.baseURL
	if base == "" {
		base = "https://api.open-meteo.com/v1/forecast"
	}
	url := fmt.Sprintf("%s?latitude=%.4f&longitude=%.4f"+
		"&current=temperature_2m,weather_code"+
		"&daily=temperature_2m_max,temperature_2m_min,precipitation_probability_max"+
		"&timezone=auto&forecast_days=1", base, lat, lon)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return Forecast{}, err
	}
	resp, err := h.client.Do(req)
	if err != nil {
		return Forecast{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Forecast{}, fmt.Errorf("weather: open-meteo returned %s", resp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return Forecast{}, err
	}
	return parseForecast(body)
}

// parseForecast decodes an Open-Meteo response. Split out so the JSON shape is
// unit-tested without a network.
func parseForecast(body []byte) (Forecast, error) {
	var raw struct {
		Current struct {
			Temp float64 `json:"temperature_2m"`
			Code int     `json:"weather_code"`
		} `json:"current"`
		Daily struct {
			Max  []float64 `json:"temperature_2m_max"`
			Min  []float64 `json:"temperature_2m_min"`
			Rain []int     `json:"precipitation_probability_max"`
		} `json:"daily"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return Forecast{}, err
	}
	f := Forecast{TempNow: raw.Current.Temp, Code: raw.Current.Code}
	if len(raw.Daily.Max) > 0 && len(raw.Daily.Min) > 0 {
		f.High = raw.Daily.Max[0]
		f.Low = raw.Daily.Min[0]
		f.HaveDay = true
		if len(raw.Daily.Rain) > 0 {
			f.RainPct = raw.Daily.Rain[0]
		}
	}
	return f, nil
}
