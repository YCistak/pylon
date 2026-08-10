package google

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/YCistak/pylon/internal/intent"
)

type fakeCal struct {
	events   []Event
	inserted []Event
	listErr  error
	insErr   error
}

func (f *fakeCal) list(context.Context, string, time.Time, time.Time) ([]Event, error) {
	return f.events, f.listErr
}
func (f *fakeCal) insert(_ context.Context, _ string, e Event) error {
	f.inserted = append(f.inserted, e)
	return f.insErr
}

func testCalendar(api calAPI) *Calendar {
	fixed := time.Date(2026, 6, 14, 9, 0, 0, 0, time.Local)
	return &Calendar{cfg: Config{CalendarID: "primary"}, api: api, now: func() time.Time { return fixed }}
}

func TestListTodayFormats(t *testing.T) {
	at := func(h, m int) time.Time { return time.Date(2026, 6, 14, h, m, 0, 0, time.Local) }
	c := testCalendar(&fakeCal{events: []Event{
		{Summary: "Toplantı", Start: at(15, 0)},
		{Summary: "Spor", Start: at(18, 30)},
	}})
	got, err := c.Execute(context.Background(), ActionListToday, nil)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	for _, want := range []string{"2 events today", "15:00 Toplantı", "18:30 Spor"} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in %q", want, got)
		}
	}
}

func TestListTodayEmpty(t *testing.T) {
	c := testCalendar(&fakeCal{})
	got, _ := c.Execute(context.Background(), ActionListToday, nil)
	if !strings.Contains(got, "is empty today") {
		t.Fatalf("empty calendar = %q", got)
	}
}

func TestAddEvent(t *testing.T) {
	fc := &fakeCal{}
	c := testCalendar(fc)
	args := map[string]string{"content": "Diş hekimi", "datetime": "2026-06-15T15:00:00+03:00"}
	got, err := c.Execute(context.Background(), ActionAddEvent, args)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if len(fc.inserted) != 1 {
		t.Fatalf("expected 1 insert, got %d", len(fc.inserted))
	}
	ev := fc.inserted[0]
	if ev.Summary != "Diş hekimi" {
		t.Fatalf("summary = %q", ev.Summary)
	}
	if !ev.End.Equal(ev.Start.Add(time.Hour)) {
		t.Fatalf("end should be start+1h: %v..%v", ev.Start, ev.End)
	}
	if !strings.Contains(got, "Eklendi") {
		t.Fatalf("reply = %q", got)
	}
}

func TestAddEventValidation(t *testing.T) {
	c := testCalendar(&fakeCal{})
	if _, err := c.Execute(context.Background(), ActionAddEvent, map[string]string{"datetime": "2026-06-15T15:00:00Z"}); err == nil {
		t.Fatal("missing title should error")
	}
	if _, err := c.Execute(context.Background(), ActionAddEvent, map[string]string{"content": "x"}); err == nil {
		t.Fatal("missing datetime should error")
	}
	if _, err := c.Execute(context.Background(), ActionAddEvent, map[string]string{"content": "x", "datetime": "bozuk"}); err == nil {
		t.Fatal("bad datetime should error")
	}
}

func TestActionsDeclared(t *testing.T) {
	c := NewCalendar(Config{})
	names := map[intent.Action]bool{}
	for _, s := range c.Actions() {
		names[s.Name] = true
	}
	if !names[ActionListToday] || !names[ActionAddEvent] {
		t.Fatalf("missing calendar actions: %v", names)
	}
}

func TestCountTodaySpeaksOnlyCount(t *testing.T) {
	at := func(h, m int) time.Time { return time.Date(2026, 6, 14, h, m, 0, 0, time.Local) }
	c := testCalendar(&fakeCal{events: []Event{
		{Summary: "Toplantı", Start: at(15, 0)},
		{Summary: "Spor", Start: at(18, 30)},
	}})
	got, err := c.Execute(context.Background(), ActionCountToday, nil)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(got, "2 events today") {
		t.Errorf("count = %q, want the number", got)
	}
	// The count must NOT leak the event titles — that is list_today's job.
	if strings.Contains(got, "Toplantı") || strings.Contains(got, "Spor") {
		t.Errorf("count_today leaked the event list: %q", got)
	}
}

func TestCountTodayEmpty(t *testing.T) {
	c := testCalendar(&fakeCal{})
	got, _ := c.Execute(context.Background(), ActionCountToday, nil)
	if !strings.Contains(got, "is empty today") {
		t.Errorf("empty day = %q, want it to say the day is empty", got)
	}
}
