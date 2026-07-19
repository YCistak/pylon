// Package briefing composes Pylon's daily briefing: a dated greeting followed by
// one short clause per configured source — today's weather, how many calendar
// events, unread news — in one flowing line.
//
// The briefing reads each source's raw value (a count, a forecast) and phrases
// the clause itself, so it controls the wording and the line reads as a whole
// rather than a list of each service's own sentence. A source that is not
// configured (nil) or that errors simply drops its clause; a missing source
// never blanks the greeting.
//
// The briefing is itself a Service (action "briefing.today"), reachable through
// the same registry as any other: the daemon fetches it to show the desktop
// banner, and the intent engine resolves "brifing ver".
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

// CalendarSource reports how many events today holds. *google.Calendar
// satisfies it; tests pass a fake.
type CalendarSource interface {
	TodayCount(ctx context.Context) (int, error)
}

// NewsSource reports how many unread items there are. *freshrss.FreshRSS
// satisfies it; tests pass a fake.
type NewsSource interface {
	UnreadCount(ctx context.Context) (int, error)
}

// Service is the briefing Service. Its sources are wired after construction
// (SetSources), since which ones exist depends on what the user has configured.
type Service struct {
	now  func() time.Time // injectable clock; defaults to time.Now
	cal  CalendarSource   // nil when calendar isn't authorized
	news NewsSource        // nil when no RSS is configured
}

// New builds the briefing service. Call SetSources with whatever is configured.
func New() *Service {
	return &Service{now: time.Now}
}

// SetSources wires the data sources the briefing reads. Pass nil for any source
// that isn't configured — its clause is then skipped.
func (s *Service) SetSources(cal CalendarSource, news NewsSource) {
	s.cal = cal
	s.news = news
}

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

// compose builds the briefing: a time-of-day greeting and dated opener, then one
// clause per available source in order (weather → calendar → news). Each clause
// is a full short sentence, so they join into a flowing line.
func (s *Service) compose(ctx context.Context) string {
	t := s.clock()
	parts := []string{fmt.Sprintf("%s! Bugün %d %s %s.", helloWord(t), t.Day(), trMonths[t.Month()], trDays[t.Weekday()])}

	// Weather slots in here once its source is wired.
	if line := s.calendarLine(ctx); line != "" {
		parts = append(parts, line)
	}
	if line := s.newsLine(ctx); line != "" {
		parts = append(parts, line)
	}
	return strings.Join(parts, " ")
}

// calendarLine phrases today's event count, or an empty-day note. It is dropped
// when calendar isn't configured or the fetch fails.
func (s *Service) calendarLine(ctx context.Context) string {
	if s.cal == nil {
		return ""
	}
	n, err := s.cal.TodayCount(ctx)
	if err != nil {
		return ""
	}
	if n == 0 {
		return "Takvim boş."
	}
	return fmt.Sprintf("Takvimde %d etkinlik var.", n)
}

// newsLine phrases the unread count. It is dropped when no RSS is configured,
// the fetch fails, or there is nothing unread (no news is not worth saying).
func (s *Service) newsLine(ctx context.Context) string {
	if s.news == nil {
		return ""
	}
	n, err := s.news.UnreadCount(ctx)
	if err != nil || n <= 0 {
		return ""
	}
	return fmt.Sprintf("%d okunmamış haber var.", n)
}

// clock returns the current time from the injectable clock.
func (s *Service) clock() time.Time {
	if s.now != nil {
		return s.now()
	}
	return time.Now()
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
