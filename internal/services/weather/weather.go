// Package weather reports the local forecast via Open-Meteo — free, no API key,
// no quota. It speaks the current conditions plus today's high, low and rain
// chance for a configured location ("İstanbul'da hava az bulutlu, şu an 24
// derece…").
package weather

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/YCistak/pylon/internal/intent"
)

// ActionToday reports the current conditions and today's outlook.
const ActionToday intent.Action = "weather.today"

// Forecast is the slice of an Open-Meteo response Pylon speaks.
type Forecast struct {
	TempNow  float64 // current temperature, °C
	Code     int     // WMO weather code
	High     float64 // today's max, °C
	Low      float64 // today's min, °C
	RainPct  int     // today's max precipitation probability, %
	haveDay  bool    // whether the daily fields were present
}

// forecaster fetches a forecast for a location; the HTTP implementation is
// swapped for a fake in tests.
type forecaster interface {
	forecast(ctx context.Context, lat, lon float64) (Forecast, error)
}

// Service reports the weather for a fixed location. It needs no key, so it is
// always registered; the location defaults to İstanbul and is overridable.
type Service struct {
	lat, lon float64
	place    string // display name, e.g. "İstanbul"
	api      forecaster
}

// New builds the service. A zero lat/lon falls back to İstanbul so a fresh
// install still answers "hava nasıl" with something sensible.
func New(lat, lon float64, place string) *Service {
	if lat == 0 && lon == 0 {
		lat, lon, place = 41.0082, 28.9784, "İstanbul"
	}
	if strings.TrimSpace(place) == "" {
		place = "Konumun"
	}
	return &Service{lat: lat, lon: lon, place: place, api: &httpForecaster{
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

func (s *Service) Execute(ctx context.Context, action intent.Action, _ map[string]string) (string, error) {
	switch action {
	case ActionToday:
		f, err := s.api.forecast(ctx, s.lat, s.lon)
		if err != nil {
			return "Hava durumuna şu an ulaşamadım.", nil
		}
		return s.speak(f), nil
	default:
		return "", fmt.Errorf("weather: bilinmeyen aksiyon %q", action)
	}
}

// speak renders a forecast as one Turkish line.
func (s *Service) speak(f Forecast) string {
	out := fmt.Sprintf("%s'da hava %s, şu an %.0f derece.", s.place, describe(f.Code), f.TempNow)
	if f.haveDay {
		out += fmt.Sprintf(" Bugün en yüksek %.0f, en düşük %.0f derece.", f.High, f.Low)
		if f.RainPct > 0 {
			out += fmt.Sprintf(" Yağış ihtimali %%%d.", f.RainPct)
		}
	}
	return out
}

// describe maps a WMO weather code to a Turkish phrase. Codes group naturally
// (drizzle 51-57, rain 61-67, snow 71-77, showers 80-82); unknown codes get a
// neutral fallback rather than an empty string.
func describe(code int) string {
	switch code {
	case 0:
		return "açık"
	case 1:
		return "az bulutlu"
	case 2:
		return "parçalı bulutlu"
	case 3:
		return "çok bulutlu"
	case 45, 48:
		return "sisli"
	case 51, 53, 55, 56, 57:
		return "çisentili"
	case 61, 63, 65, 66, 67:
		return "yağmurlu"
	case 71, 73, 75, 77:
		return "karlı"
	case 80, 81, 82:
		return "sağanak yağışlı"
	case 85, 86:
		return "kar sağanaklı"
	case 95:
		return "gök gürültülü fırtınalı"
	case 96, 99:
		return "dolu ve gök gürültülü fırtınalı"
	default:
		return "değişken"
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
		f.haveDay = true
		if len(raw.Daily.Rain) > 0 {
			f.RainPct = raw.Daily.Rain[0]
		}
	}
	return f, nil
}
