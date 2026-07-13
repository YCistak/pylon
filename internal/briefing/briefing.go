// Package briefing composes Pylon's morning briefing: a short dated greeting
// followed by one line from each available data source — today's calendar, open
// GitHub items, unread news. It reuses each service's own action reply through
// the registry (a Dispatcher), so it never re-implements service formatting; a
// section whose service is unconfigured or errors is simply skipped.
//
// The briefing is itself a Service (action "briefing.today"), so it is reachable
// three ways: the daily scheduler speaks it every morning, the intent engine
// resolves "brifing ver", and the GUI can show it as a widget.
package briefing

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/YCistak/pylon/internal/intent"
)

// ActionToday builds and returns the full morning briefing.
const ActionToday intent.Action = "briefing.today"

// Dispatcher runs a resolved command against the service registry.
// *services.Registry satisfies this; tests pass a fake.
type Dispatcher interface {
	Dispatch(ctx context.Context, cmd intent.Command) (text string, ok bool, err error)
}

// Section is one line of the briefing: a service action to run, in speaking order.
type Section struct {
	Action intent.Action
	Args   map[string]string
}

// DefaultSections are the briefing's data sources, in the order they are spoken.
// Each is dropped silently if its service is not configured.
func DefaultSections() []Section {
	return []Section{
		{Action: "calendar.list_today"},
		{Action: "github.list_prs"},
		{Action: "freshrss.unread_count"},
	}
}

// Service is the briefing Service. Its Dispatcher is the registry it lives in, so
// it is wired after the registry is built (SetDispatcher).
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
			Desc: `"briefing.today": the user's morning briefing — today's date plus calendar, open GitHub items and unread news. No args. Use for "brifing ver", "günü özetle", "bugün ne var".`,
		},
	}
}

func (s *Service) Execute(ctx context.Context, action intent.Action, _ map[string]string) (string, error) {
	switch action {
	case ActionToday:
		return s.Build(ctx), nil
	default:
		return "", fmt.Errorf("briefing: bilinmeyen aksiyon %q", action)
	}
}

// Build assembles the full spoken briefing. Sections whose service is
// unavailable or errors are dropped; if every section drops, only the dated
// greeting remains.
func (s *Service) Build(ctx context.Context) string {
	title, body := s.compose(ctx)
	return title + ". " + body
}

// Notification splits the briefing for a desktop notification: the greeting is
// the title ("Günaydın") and the dated summary is the body ("Bugün 13 Temmuz
// Pazartesi. 6069 okunmamış haberin var.").
func (s *Service) Notification(ctx context.Context) (title, body string) {
	return s.compose(ctx)
}

// compose builds the two halves of the briefing: the time-of-day greeting word
// and the dated body with each available section appended.
func (s *Service) compose(ctx context.Context) (hello, body string) {
	now := time.Now
	if s.now != nil {
		now = s.now
	}
	t := now()
	parts := []string{fmt.Sprintf("Bugün %d %s %s.", t.Day(), trMonths[t.Month()], trDays[t.Weekday()])}
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
	return helloWord(t), strings.Join(parts, " ")
}

// helloWord is the bare time-of-day greeting (no trailing punctuation), used as
// the notification title and the spoken opener.
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
	time.Monday:    "Pazartesi",
	time.Tuesday:   "Salı",
	time.Wednesday: "Çarşamba",
	time.Thursday:  "Perşembe",
	time.Friday:    "Cuma",
	time.Saturday:  "Cumartesi",
	time.Sunday:    "Pazar",
}

var trMonths = map[time.Month]string{
	time.January:   "Ocak",
	time.February:  "Şubat",
	time.March:     "Mart",
	time.April:     "Nisan",
	time.May:       "Mayıs",
	time.June:      "Haziran",
	time.July:      "Temmuz",
	time.August:    "Ağustos",
	time.September: "Eylül",
	time.October:   "Ekim",
	time.November:  "Kasım",
	time.December:  "Aralık",
}
