// Package briefing composes Pylon's daily briefing: a dated greeting followed
// by one line from each configured source — today's weather, how many calendar
// events, unread news. It reuses each service's own action reply through the
// registry (a Dispatcher), so it never re-implements their formatting; a section
// whose service is unconfigured or errors is simply dropped.
//
// The briefing is itself a Service (action "briefing.today"), reachable through
// the same registry as any other: the GUI fetches it to show a banner, and the
// intent engine resolves "brifing ver".
package briefing

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/YCistak/pylon/internal/intent"
)

// ActionToday builds and returns the full briefing text.
const ActionToday intent.Action = "briefing.today"

// Dispatcher runs a resolved command against the service registry.
// *services.Registry satisfies this; tests pass a fake.
type Dispatcher interface {
	Dispatch(ctx context.Context, cmd intent.Command) (text string, ok bool, err error)
}

// Section is one line of the briefing: a service action, in speaking order.
type Section struct {
	Action intent.Action
	Args   map[string]string
}

// DefaultSections are the briefing's sources, in the order they are spoken:
// weather, then how busy today is, then unread news. Each drops silently if its
// service is not configured.
func DefaultSections() []Section {
	return []Section{
		{Action: "weather.today"},
		{Action: "calendar.count_today"},
		{Action: "freshrss.unread_count"},
	}
}

// Service is the briefing Service. Its Dispatcher is the registry it lives in,
// so it is wired after the registry is built (SetDispatcher).
type Service struct {
	dispatch Dispatcher
	now      func() time.Time // injectable clock; defaults to time.Now
	sections []Section
}

// New builds the briefing service. Call SetDispatcher with the registry before use.
func New() *Service {
	return &Service{now: time.Now, sections: DefaultSections()}
}

// SetDispatcher wires the registry the briefing dispatches its sections through.
func (s *Service) SetDispatcher(d Dispatcher) { s.dispatch = d }

func (s *Service) Name() string { return "briefing" }

func (s *Service) Actions() []intent.ActionSpec {
	return []intent.ActionSpec{
		{
			Name: ActionToday,
			Desc: `"briefing.today": give the daily briefing — a dated greeting, today's weather, how many calendar events, and unread news, in one short spoken paragraph. No args. Use for "brifing ver", "günü özetle", "bugün ne var".`,
		},
	}
}

func (s *Service) Execute(ctx context.Context, action intent.Action, _ map[string]string) (string, error) {
	switch action {
	case ActionToday:
		return s.compose(ctx), nil
	default:
		return "", fmt.Errorf("briefing: bilinmeyen aksiyon %q", action)
	}
}

// compose builds the briefing: a time-of-day greeting and dated opener, then
// each available section's own reply appended in order.
func (s *Service) compose(ctx context.Context) string {
	now := time.Now
	if s.now != nil {
		now = s.now
	}
	t := now()
	parts := []string{fmt.Sprintf("%s! Bugün %d %s %s.", helloWord(t), t.Day(), trMonths[t.Month()], trDays[t.Weekday()])}

	if s.dispatch != nil {
		for _, sec := range s.sections {
			text, ok, err := s.dispatch.Dispatch(ctx, intent.Command{Action: sec.Action, Args: sec.Args})
			if !ok || err != nil {
				continue
			}
			if text = strings.TrimSpace(text); text != "" {
				parts = append(parts, text)
			}
		}
	}
	return strings.Join(parts, " ")
}

// helloWord is the bare time-of-day greeting.
func helloWord(t time.Time) string {
	switch h := t.Hour(); {
	case h < 6:
		return "İyi geceler"
	case h < 12:
		return "Günaydın"
	case h < 18:
		return "İyi günler"
	default:
		return "İyi akşamlar"
	}
}

var trDays = map[time.Weekday]string{
	time.Monday: "Pazartesi", time.Tuesday: "Salı", time.Wednesday: "Çarşamba",
	time.Thursday: "Perşembe", time.Friday: "Cuma", time.Saturday: "Cumartesi",
	time.Sunday: "Pazar",
}

var trMonths = map[time.Month]string{
	time.January: "Ocak", time.February: "Şubat", time.March: "Mart",
	time.April: "Nisan", time.May: "Mayıs", time.June: "Haziran",
	time.July: "Temmuz", time.August: "Ağustos", time.September: "Eylül",
	time.October: "Ekim", time.November: "Kasım", time.December: "Aralık",
}
