package work

import (
	"context"
	"io"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/YCistak/pylon/internal/db"
)

func testStore(t *testing.T) *db.DB {
	t.Helper()
	d, err := db.Open(filepath.Join(t.TempDir(), "work.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { d.Close() })
	return d
}

func quietLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// clock is a hand-advanced time source, so a test can cover a working day
// without waiting for one.
type clock struct{ t time.Time }

func (c *clock) now() time.Time          { return c.t }
func (c *clock) advance(d time.Duration) { c.t = c.t.Add(d) }

func newTracker(t *testing.T, store Store, c *clock, apps ...string) *Tracker {
	t.Helper()
	tr := NewTracker(TrackerOptions{Store: store, Apps: apps, Logger: quietLogger(), Now: c.now})
	if tr == nil {
		t.Fatal("NewTracker returned nil for a non-empty app list")
	}
	return tr
}

// With no tracked apps there is nothing to register, and the caller relies on
// nil to skip the whole feature rather than run a tracker that does nothing.
func TestNewTrackerNilWhenNothingTracked(t *testing.T) {
	if tr := NewTracker(TrackerOptions{Store: testStore(t)}); tr != nil {
		t.Fatal("expected nil for an empty app list")
	}
	if tr := NewTracker(TrackerOptions{Store: testStore(t), Apps: []string{"", "  "}}); tr != nil {
		t.Fatal("expected nil for blank app names")
	}
}

func TestTrackerRecordsASession(t *testing.T) {
	store := testStore(t)
	c := &clock{t: time.Date(2026, 8, 7, 9, 0, 0, 0, time.UTC)}
	tr := newTracker(t, store, c, "code")

	tr.Observe("code", true, c.now())
	c.advance(90 * time.Minute)
	tr.Observe("code", false, c.now())

	totals, err := store.SessionTotals(
		time.Date(2026, 8, 7, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 8, 8, 0, 0, 0, 0, time.UTC), c.now())
	if err != nil {
		t.Fatalf("totals: %v", err)
	}
	if len(totals) != 1 || totals[0].App != "code" || totals[0].Total != 90*time.Minute {
		t.Fatalf("totals = %+v", totals)
	}
}

// A process watcher that re-reports the same state must not open a second
// session or close one that is still running.
func TestTrackerIgnoresRepeatedObservations(t *testing.T) {
	store := testStore(t)
	c := &clock{t: time.Date(2026, 8, 7, 9, 0, 0, 0, time.UTC)}
	tr := newTracker(t, store, c, "code")

	tr.Observe("code", true, c.now())
	c.advance(time.Hour)
	tr.Observe("code", true, c.now()) // duplicate start
	tr.Observe("cs2", false, c.now()) // untracked, and never started

	open, err := store.OpenSessions()
	if err != nil {
		t.Fatalf("open sessions: %v", err)
	}
	if len(open) != 1 {
		t.Fatalf("expected one open session, got %+v", open)
	}
	if !open[0].Start.Equal(time.Date(2026, 8, 7, 9, 0, 0, 0, time.UTC)) {
		t.Fatalf("the duplicate start moved the session to %v", open[0].Start)
	}
}

// Apps outside tracked_apps are watched for other reasons (task reminders) and
// must not leak into the time record.
func TestTrackerIgnoresUntrackedApps(t *testing.T) {
	store := testStore(t)
	c := &clock{t: time.Date(2026, 8, 7, 9, 0, 0, 0, time.UTC)}
	tr := newTracker(t, store, c, "code")

	tr.Observe("firefox", true, c.now())

	open, err := store.OpenSessions()
	if err != nil {
		t.Fatalf("open sessions: %v", err)
	}
	if len(open) != 0 {
		t.Fatalf("untracked app was recorded: %+v", open)
	}
}

// Seed exists because a watcher reports transitions, not state: an editor left
// open across a daemon restart would otherwise be credited nothing.
func TestTrackerSeedAdoptsRunningApps(t *testing.T) {
	store := testStore(t)
	c := &clock{t: time.Date(2026, 8, 7, 9, 0, 0, 0, time.UTC)}
	tr := newTracker(t, store, c, "code", "cs2")

	tr.Seed([]string{"code", "firefox"})

	open, err := store.OpenSessions()
	if err != nil {
		t.Fatalf("open sessions: %v", err)
	}
	if len(open) != 1 || open[0].App != "code" {
		t.Fatalf("seeded %+v, want only code", open)
	}
}

// Shutting the daemon down closes what is open, so the next start has nothing
// to recover and the time lands at the moment we actually stopped.
func TestTrackerClosesSessionsOnShutdown(t *testing.T) {
	store := testStore(t)
	c := &clock{t: time.Date(2026, 8, 7, 9, 0, 0, 0, time.UTC)}
	tr := NewTracker(TrackerOptions{
		Store: store, Apps: []string{"code"}, Logger: quietLogger(),
		Now: c.now, Beat: time.Millisecond,
	})

	tr.Observe("code", true, c.now())
	c.advance(2 * time.Hour)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- tr.Run(ctx) }()
	cancel()
	<-done

	open, err := store.OpenSessions()
	if err != nil {
		t.Fatalf("open sessions: %v", err)
	}
	if len(open) != 0 {
		t.Fatalf("session left open after shutdown: %+v", open)
	}

	totals, err := store.SessionTotals(
		time.Date(2026, 8, 7, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 8, 8, 0, 0, 0, 0, time.UTC), c.now())
	if err != nil {
		t.Fatalf("totals: %v", err)
	}
	if len(totals) != 1 || totals[0].Total != 2*time.Hour {
		t.Fatalf("totals = %+v, want 2h", totals)
	}
}

// The heartbeat is what makes a crash survivable — it must actually reach the
// store while a session is open.
func TestTrackerHeartbeatMarksOpenSessions(t *testing.T) {
	store := testStore(t)
	c := &clock{t: time.Date(2026, 8, 7, 9, 0, 0, 0, time.UTC)}
	tr := newTracker(t, store, c, "code")

	tr.Observe("code", true, c.now())
	c.advance(30 * time.Minute)
	tr.touch()

	open, err := store.OpenSessions()
	if err != nil {
		t.Fatalf("open sessions: %v", err)
	}
	if len(open) != 1 {
		t.Fatalf("expected one open session, got %d", len(open))
	}
	if !open[0].Seen.Equal(c.now()) {
		t.Fatalf("heartbeat = %v, want %v", open[0].Seen, c.now())
	}
}

// --- service ---

func newService(t *testing.T, store Store, at time.Time, goalHours float64) *Service {
	t.Helper()
	s := NewService(ServiceOptions{
		Store: store, GoalHours: goalHours, Timezone: "Europe/Istanbul",
		Now: func() time.Time { return at },
	})
	if s == nil {
		t.Fatal("NewService returned nil with a store")
	}
	return s
}

func TestServiceTodayReportsPerAppAndGoal(t *testing.T) {
	store := testStore(t)
	ist, err := time.LoadLocation("Europe/Istanbul")
	if err != nil {
		t.Skipf("tz database unavailable: %v", err)
	}
	start := time.Date(2026, 8, 7, 9, 0, 0, 0, ist)
	now := start.Add(4 * time.Hour)

	if _, err := store.StartSession("code", start); err != nil {
		t.Fatal(err)
	}
	if err := store.EndSession("code", start.Add(3*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if _, err := store.StartSession("cs2", start.Add(3*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := store.EndSession("cs2", start.Add(3*time.Hour+30*time.Minute)); err != nil {
		t.Fatal(err)
	}

	got, err := newService(t, store, now, 4).Execute(context.Background(), ActionToday, nil)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	for _, want := range []string{"Bugün toplam 3 saat 30 dakika", "code 3 saat", "cs2 30 dakika", "Günlük hedef 4 saat", "%87", "30 dakika kaldı"} {
		if !strings.Contains(got, want) {
			t.Fatalf("summary %q missing %q", got, want)
		}
	}
}

func TestServiceTodayWhenGoalIsMet(t *testing.T) {
	store := testStore(t)
	ist, _ := time.LoadLocation("Europe/Istanbul")
	start := time.Date(2026, 8, 7, 9, 0, 0, 0, ist)

	if _, err := store.StartSession("code", start); err != nil {
		t.Fatal(err)
	}
	if err := store.EndSession("code", start.Add(5*time.Hour)); err != nil {
		t.Fatal(err)
	}

	got, err := newService(t, store, start.Add(6*time.Hour), 4).Execute(context.Background(), ActionToday, nil)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(got, "geçtin") {
		t.Fatalf("summary = %q, expected it to say the goal was passed", got)
	}
}

// A day with nothing recorded must say so plainly rather than report zero
// against a goal, which reads like a failure report every morning.
func TestServiceTodayWithNothingRecorded(t *testing.T) {
	store := testStore(t)
	got, err := newService(t, store, time.Now(), 4).Execute(context.Background(), ActionToday, nil)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(got, "zaman geçirmemişsin") {
		t.Fatalf("summary = %q", got)
	}
}

// The goal sentence is optional: a user who set no goal should get the numbers
// without being measured against zero.
func TestServiceOmitsGoalWhenUnset(t *testing.T) {
	store := testStore(t)
	ist, _ := time.LoadLocation("Europe/Istanbul")
	start := time.Date(2026, 8, 7, 9, 0, 0, 0, ist)

	if _, err := store.StartSession("code", start); err != nil {
		t.Fatal(err)
	}
	if err := store.EndSession("code", start.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}

	got, err := newService(t, store, start.Add(2*time.Hour), 0).Execute(context.Background(), ActionToday, nil)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if strings.Contains(got, "hedef") {
		t.Fatalf("summary mentions a goal that was never set: %q", got)
	}
}

// A day starts at midnight where the user lives, not in UTC. With Istanbul at
// +03, work done at 01:00 local belongs to today even though it is still
// yesterday in UTC.
func TestServiceDayBoundaryUsesConfiguredTimezone(t *testing.T) {
	store := testStore(t)
	ist, err := time.LoadLocation("Europe/Istanbul")
	if err != nil {
		t.Skipf("tz database unavailable: %v", err)
	}
	// 01:00 Istanbul on the 7th is 22:00 UTC on the 6th.
	start := time.Date(2026, 8, 7, 1, 0, 0, 0, ist)

	if _, err := store.StartSession("code", start); err != nil {
		t.Fatal(err)
	}
	if err := store.EndSession("code", start.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}

	got, err := newService(t, store, start.Add(2*time.Hour), 0).Execute(context.Background(), ActionToday, nil)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(got, "1 saat") {
		t.Fatalf("work at 01:00 local was not counted as today: %q", got)
	}
}

func TestServiceWeekJudgesAgainstAWeeksGoal(t *testing.T) {
	store := testStore(t)
	ist, _ := time.LoadLocation("Europe/Istanbul")
	now := time.Date(2026, 8, 7, 18, 0, 0, 0, ist)

	// Two hours a day for the last three days: 6h against a 28h weekly goal.
	for i := 0; i < 3; i++ {
		day := time.Date(2026, 8, 7-i, 10, 0, 0, 0, ist)
		if _, err := store.StartSession("code", day); err != nil {
			t.Fatal(err)
		}
		if err := store.EndSession("code", day.Add(2*time.Hour)); err != nil {
			t.Fatal(err)
		}
	}

	got, err := newService(t, store, now, 4).Execute(context.Background(), ActionWeek, nil)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	for _, want := range []string{"Son 7 günde code ile 6 saat", "Haftalık hedef 28 saat"} {
		if !strings.Contains(got, want) {
			t.Fatalf("summary %q missing %q", got, want)
		}
	}
}

func TestServiceRejectsUnknownAction(t *testing.T) {
	if _, err := newService(t, testStore(t), time.Now(), 4).Execute(context.Background(), "work.nope", nil); err == nil {
		t.Fatal("expected an error for an unknown action")
	}
}

func TestHumanDuration(t *testing.T) {
	cases := map[time.Duration]string{
		0:                            "bir dakikadan az",
		30 * time.Second:             "bir dakikadan az",
		45 * time.Minute:             "45 dakika",
		time.Hour:                    "1 saat",
		2*time.Hour + time.Minute:    "2 saat 1 dakika",
		3*time.Hour + 29*time.Second: "3 saat",
	}
	for d, want := range cases {
		if got := humanDuration(d); got != want {
			t.Errorf("humanDuration(%v) = %q, want %q", d, got, want)
		}
	}
}

// With one app the breakdown is the total, and saying both ("Bugün 1 saat:
// code 1 saat") sounds broken read aloud.
func TestServiceSingleAppDoesNotRepeatTheTotal(t *testing.T) {
	store := testStore(t)
	ist, _ := time.LoadLocation("Europe/Istanbul")
	start := time.Date(2026, 8, 7, 9, 0, 0, 0, ist)

	if _, err := store.StartSession("code", start); err != nil {
		t.Fatal(err)
	}
	if err := store.EndSession("code", start.Add(90*time.Minute)); err != nil {
		t.Fatal(err)
	}

	got, err := newService(t, store, start.Add(2*time.Hour), 0).Execute(context.Background(), ActionToday, nil)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if got != "Bugün code ile 1 saat 30 dakika." {
		t.Fatalf("summary = %q", got)
	}
}

// ------------------------------------------------------------ break nudge ---

// nudgeTracker builds a tracker that records its nudges instead of showing them.
func nudgeTracker(t *testing.T, store Store, c *clock, after time.Duration, got *[]string) *Tracker {
	t.Helper()
	tr := NewTracker(TrackerOptions{
		Store: store, Apps: []string{"code"}, Logger: quietLogger(), Now: c.now,
		BreakAfter: after,
		Nudge:      func(text string) { *got = append(*got, text) },
	})
	if tr == nil {
		t.Fatal("NewTracker returned nil")
	}
	return tr
}

func TestBreakNudgeFiresOnceThenRepeats(t *testing.T) {
	store := testStore(t)
	c := &clock{t: time.Date(2026, 8, 10, 9, 0, 0, 0, time.UTC)}
	var got []string
	tr := nudgeTracker(t, store, c, 2*time.Hour, &got)

	tr.Observe("code", true, c.now())

	// Short of the threshold: nothing.
	c.advance(119 * time.Minute)
	tr.checkBreak()
	if len(got) != 0 {
		t.Fatalf("nudged too early: %v", got)
	}

	c.advance(time.Minute) // exactly 2h
	tr.checkBreak()
	if len(got) != 1 || !strings.Contains(got[0], "Aralıksız 2 saat oldu") {
		t.Fatalf("first nudge = %v", got)
	}

	// Still working, but inside the quiet window.
	c.advance(29 * time.Minute)
	tr.checkBreak()
	if len(got) != 1 {
		t.Fatalf("nudged again during the quiet window: %v", got)
	}

	c.advance(time.Minute) // 30 minutes since the last nudge
	tr.checkBreak()
	if len(got) != 2 || !strings.Contains(got[1], "2 saat 30 dakika") {
		t.Fatalf("repeat nudge = %v", got)
	}
}

// Closing every tracked app is the break: the clock starts over, so a full
// threshold has to pass again before the next nudge.
func TestBreakNudgeResetsWhenEverythingCloses(t *testing.T) {
	store := testStore(t)
	c := &clock{t: time.Date(2026, 8, 10, 9, 0, 0, 0, time.UTC)}
	var got []string
	tr := nudgeTracker(t, store, c, 2*time.Hour, &got)

	tr.Observe("code", true, c.now())
	c.advance(110 * time.Minute)
	tr.Observe("code", false, c.now()) // break taken
	c.advance(20 * time.Minute)
	tr.Observe("code", true, c.now()) // back at it

	c.advance(30 * time.Minute) // 30 minutes into the new stretch
	tr.checkBreak()
	if len(got) != 0 {
		t.Fatalf("stretch should have restarted: %v", got)
	}

	c.advance(90 * time.Minute) // 2h into the new stretch
	tr.checkBreak()
	if len(got) != 1 || !strings.Contains(got[0], "2 saat") {
		t.Fatalf("nudge after the new stretch = %v", got)
	}
}

// A second app opening mid-stretch is not a fresh start — the stretch is about
// the person, not the window.
func TestBreakNudgeIgnoresAppsOpeningMidStretch(t *testing.T) {
	store := testStore(t)
	c := &clock{t: time.Date(2026, 8, 10, 9, 0, 0, 0, time.UTC)}
	var got []string
	tr := NewTracker(TrackerOptions{
		Store: store, Apps: []string{"code", "steam"}, Logger: quietLogger(), Now: c.now,
		BreakAfter: 2 * time.Hour,
		Nudge:      func(text string) { got = append(got, text) },
	})

	tr.Observe("code", true, c.now())
	c.advance(90 * time.Minute)
	tr.Observe("steam", true, c.now()) // opened later
	c.advance(30 * time.Minute)        // 2h since code came up

	tr.checkBreak()
	if len(got) != 1 {
		t.Fatalf("second app restarted the clock: %v", got)
	}

	// One app closing while another stays open is not a break either.
	tr.Observe("code", false, c.now())
	c.advance(31 * time.Minute)
	tr.checkBreak()
	if len(got) != 2 {
		t.Fatalf("stretch should continue while steam is open: %v", got)
	}
}

// Zero disables the feature, and a tracker with no Nudge never calls one.
func TestBreakNudgeOffByDefault(t *testing.T) {
	store := testStore(t)
	c := &clock{t: time.Date(2026, 8, 10, 9, 0, 0, 0, time.UTC)}
	var got []string
	tr := nudgeTracker(t, store, c, 0, &got)

	tr.Observe("code", true, c.now())
	c.advance(9 * time.Hour)
	tr.checkBreak()
	if len(got) != 0 {
		t.Fatalf("disabled tracker nudged: %v", got)
	}
}
