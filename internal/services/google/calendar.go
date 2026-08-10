package google

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	calendar "google.golang.org/api/calendar/v3"
	"google.golang.org/api/option"

	"github.com/YCistak/pylon/internal/i18n"
	"github.com/YCistak/pylon/internal/intent"
)

// Calendar actions.
const (
	ActionListToday  intent.Action = "calendar.list_today"
	ActionCountToday intent.Action = "calendar.count_today"
	ActionAddEvent   intent.Action = "calendar.add_event"
)

// Event is the minimal shape Pylon needs (decoupled from the API for testing).
type Event struct {
	Summary string
	Start   time.Time
	End     time.Time
}

// calAPI is the slice of the Calendar API the service uses; a fake implements it
// in tests.
type calAPI interface {
	list(ctx context.Context, calID string, min, max time.Time) ([]Event, error)
	insert(ctx context.Context, calID string, e Event) error
}

// Calendar is the Google Calendar Service.
type Calendar struct {
	cfg Config
	api calAPI           // injected in tests; otherwise built lazily from the token
	now func() time.Time // injectable clock (tests)
}

// NewCalendar builds the service from config. It does not touch the network or
// token until first use, so it can be registered before `pylon auth google`.
func NewCalendar(cfg Config) *Calendar {
	if cfg.CalendarID == "" {
		cfg.CalendarID = "primary"
	}
	return &Calendar{cfg: cfg, now: time.Now}
}

func (c *Calendar) Name() string { return "calendar" }

func (c *Calendar) Actions() []intent.ActionSpec {
	return []intent.ActionSpec{
		{
			Name: ActionListToday,
			Desc: `"calendar.list_today": list the user's Google Calendar events for today. No args needed.`,
		},
		{
			Name: ActionCountToday,
			Desc: `"calendar.count_today": say only how many events the user has today, not the list. No args. Use for "bugün kaç etkinliğim var", "programım yoğun mu".`,
		},
		{
			Name: ActionAddEvent,
			Args: []string{"content", "datetime"},
			Desc: `"calendar.add_event": add a calendar event. Put the event title in "content" and the start time in "datetime" (absolute ISO-8601). Use for "yarın saat üçte diş hekimi randevusu ekle".`,
		},
	}
}

func (c *Calendar) Execute(ctx context.Context, action intent.Action, args map[string]string) (string, error) {
	api, err := c.client(ctx)
	if err != nil {
		return "", err
	}
	switch action {
	case ActionListToday:
		return c.listToday(ctx, api)
	case ActionCountToday:
		return c.countToday(ctx, api)
	case ActionAddEvent:
		return c.addEvent(ctx, api, args)
	default:
		return "", fmt.Errorf("calendar: bilinmeyen aksiyon %q", action)
	}
}

// todayWindow is the [midnight, midnight+24h) range in the clock's location.
func (c *Calendar) todayWindow() (time.Time, time.Time) {
	now := c.now()
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	return start, start.Add(24 * time.Hour)
}

func (c *Calendar) listToday(ctx context.Context, api calAPI) (string, error) {
	start, end := c.todayWindow()
	events, err := api.list(ctx, c.cfg.CalendarID, start, end)
	if err != nil {
		return "", err
	}
	if len(events) == 0 {
		return i18n.T("calendar.empty"), nil
	}
	var parts []string
	for _, e := range events {
		parts = append(parts, fmt.Sprintf("%s %s", e.Start.Local().Format("15:04"), e.Summary))
	}
	return i18n.N("calendar.events_detail", len(events), strings.Join(parts, "; ")), nil
}

// countToday reports only how many events today has, for "bugün kaç etkinliğim
// var" — the same fetch as listToday, without the list.
func (c *Calendar) countToday(ctx context.Context, api calAPI) (string, error) {
	n, err := c.count(ctx, api)
	if err != nil {
		return "", err
	}
	if n == 0 {
		return i18n.T("calendar.empty"), nil
	}
	return i18n.N("calendar.events", n), nil
}

// count is the raw event count for today.
func (c *Calendar) count(ctx context.Context, api calAPI) (int, error) {
	start, end := c.todayWindow()
	events, err := api.list(ctx, c.cfg.CalendarID, start, end)
	if err != nil {
		return 0, err
	}
	return len(events), nil
}

// TodayCount returns how many events today has, building the API client on
// demand. It is the typed entry the daily briefing reads, so the briefing can
// phrase the count itself rather than reusing the action's sentence.
func (c *Calendar) TodayCount(ctx context.Context) (int, error) {
	api, err := c.client(ctx)
	if err != nil {
		return 0, err
	}
	return c.count(ctx, api)
}

func (c *Calendar) addEvent(ctx context.Context, api calAPI, args map[string]string) (string, error) {
	title := strings.TrimSpace(args["content"])
	if title == "" {
		return "", errors.New("calendar: the event needs a title")
	}
	dtRaw := strings.TrimSpace(args["datetime"])
	if dtRaw == "" {
		return "", errors.New("calendar: the event needs a date and time")
	}
	start, err := parseDateTime(dtRaw)
	if err != nil {
		return "", fmt.Errorf("calendar: could not read the date (%q)", dtRaw)
	}
	end := start.Add(time.Hour)
	if err := api.insert(ctx, c.cfg.CalendarID, Event{Summary: title, Start: start, End: end}); err != nil {
		return "", err
	}
	return fmt.Sprintf("Eklendi: %s, %s.", title, start.Local().Format("2 January 15:04")), nil
}

// parseDateTime accepts the LLM's ISO-8601 output, with a couple of fallbacks.
func parseDateTime(s string) (time.Time, error) {
	for _, layout := range []string{time.RFC3339, "2006-01-02T15:04:05", "2006-01-02T15:04", "2006-01-02 15:04"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("unparseable datetime %q", s)
}

// client lazily builds the real Calendar API from the saved OAuth token, unless
// one was injected (tests).
func (c *Calendar) client(ctx context.Context) (calAPI, error) {
	if c.api != nil {
		return c.api, nil
	}
	hc, err := httpClient(ctx, c.cfg)
	if err != nil {
		return nil, err
	}
	svc, err := calendar.NewService(ctx, option.WithHTTPClient(hc))
	if err != nil {
		return nil, fmt.Errorf("calendar service: %w", err)
	}
	return &realCal{svc: svc}, nil
}

// realCal adapts the google calendar API to calAPI.
type realCal struct{ svc *calendar.Service }

func (r *realCal) list(ctx context.Context, calID string, min, max time.Time) ([]Event, error) {
	res, err := r.svc.Events.List(calID).
		Context(ctx).
		TimeMin(min.Format(time.RFC3339)).
		TimeMax(max.Format(time.RFC3339)).
		SingleEvents(true).
		OrderBy("startTime").
		Do()
	if err != nil {
		return nil, err
	}
	var out []Event
	for _, it := range res.Items {
		e := Event{Summary: it.Summary}
		if it.Start != nil {
			e.Start = eventTime(it.Start)
		}
		if it.End != nil {
			e.End = eventTime(it.End)
		}
		out = append(out, e)
	}
	return out, nil
}

func (r *realCal) insert(ctx context.Context, calID string, e Event) error {
	_, err := r.svc.Events.Insert(calID, &calendar.Event{
		Summary: e.Summary,
		Start:   &calendar.EventDateTime{DateTime: e.Start.Format(time.RFC3339)},
		End:     &calendar.EventDateTime{DateTime: e.End.Format(time.RFC3339)},
	}).Context(ctx).Do()
	return err
}

func eventTime(dt *calendar.EventDateTime) time.Time {
	if dt.DateTime != "" {
		if t, err := time.Parse(time.RFC3339, dt.DateTime); err == nil {
			return t
		}
	}
	if dt.Date != "" { // all-day event
		if t, err := time.Parse("2006-01-02", dt.Date); err == nil {
			return t
		}
	}
	return time.Time{}
}
