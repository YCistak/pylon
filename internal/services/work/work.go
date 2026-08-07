// Package work turns process lifecycle into a record of where your time went.
//
// It holds two halves of the same feature. Tracker listens to a process watcher
// and writes session rows as tracked apps come and go; Service reads those rows
// back and answers "bugün ne kadar çalıştım" as a spoken line. They live
// together because the honesty of the answer depends on how the rows were
// written — the heartbeat, the clipping, the recovery after a crash are one
// design decision, not two.
package work

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/YCistak/pylon/internal/db"
	"github.com/YCistak/pylon/internal/intent"
)

// Actions this service answers.
const (
	ActionToday intent.Action = "work.today"
	ActionWeek  intent.Action = "work.week"
)

// heartbeat is how often an open session's "last seen" mark is refreshed. It
// bounds what an unclean shutdown can lose: a killed daemon costs at most one
// interval of the session it was in, instead of the whole session.
const heartbeat = time.Minute

// Store is the slice of persistence this package needs. *db.DB satisfies it.
type Store interface {
	StartSession(app string, at time.Time) (int64, error)
	EndSession(app string, at time.Time) error
	TouchSessions(apps []string, at time.Time) error
	CloseOpenSessions() (int, error)
	SessionTotals(from, to, now time.Time) ([]db.AppTotal, error)
}

// ---------------------------------------------------------------- tracker ---

// Tracker converts "app started" / "app stopped" observations into session
// rows. It is deliberately ignorant of how those observations are made, so the
// process watcher stays the only thing that has to know about /proc.
type Tracker struct {
	store Store
	apps  map[string]struct{}
	log   *slog.Logger
	now   func() time.Time
	beat  time.Duration

	mu      sync.Mutex
	running map[string]struct{}
}

// TrackerOptions configures a Tracker.
type TrackerOptions struct {
	Store  Store
	Apps   []string         // process names to record sessions for
	Logger *slog.Logger     //
	Now    func() time.Time // injectable clock (tests)
	Beat   time.Duration    // heartbeat interval; zero means the default
}

// NewTracker builds a Tracker. It returns nil when no apps are tracked, so the
// caller can skip registering anything at all.
func NewTracker(o TrackerOptions) *Tracker {
	apps := map[string]struct{}{}
	for _, a := range o.Apps {
		if a = strings.TrimSpace(a); a != "" {
			apps[a] = struct{}{}
		}
	}
	if len(apps) == 0 || o.Store == nil {
		return nil
	}
	if o.Logger == nil {
		o.Logger = slog.Default()
	}
	if o.Now == nil {
		o.Now = time.Now
	}
	if o.Beat <= 0 {
		o.Beat = heartbeat
	}
	return &Tracker{
		store:   o.Store,
		apps:    apps,
		log:     o.Logger,
		now:     o.Now,
		beat:    o.Beat,
		running: map[string]struct{}{},
	}
}

// Names returns the tracked app names, so the caller can fold them into the set
// a shared process watcher polls for.
func (t *Tracker) Names() []string {
	out := make([]string, 0, len(t.apps))
	for a := range t.apps {
		out = append(out, a)
	}
	return out
}

// Tracks reports whether this app is one we record.
func (t *Tracker) Tracks(app string) bool {
	_, ok := t.apps[app]
	return ok
}

// Seed adopts the apps that were already running when the daemon started. A
// process watcher reports transitions, not state, so without this an editor
// left open across a daemon restart would be credited nothing for the rest of
// the day.
func (t *Tracker) Seed(running []string) {
	at := t.now()
	for _, app := range running {
		t.Observe(app, true, at)
	}
}

// Observe records a transition. Repeated observations of the same state are
// ignored, so a watcher that re-reports is harmless.
func (t *Tracker) Observe(app string, isRunning bool, at time.Time) {
	if !t.Tracks(app) {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()

	_, was := t.running[app]
	switch {
	case isRunning && !was:
		if _, err := t.store.StartSession(app, at); err != nil {
			t.log.Warn("sessions: start failed", "app", app, "err", err)
			return
		}
		t.running[app] = struct{}{}
		t.log.Info("sessions: started", "app", app)
	case !isRunning && was:
		if err := t.store.EndSession(app, at); err != nil {
			t.log.Warn("sessions: end failed", "app", app, "err", err)
			return
		}
		delete(t.running, app)
		t.log.Info("sessions: ended", "app", app)
	}
}

// Run keeps open sessions marked alive and closes them on shutdown. It blocks
// until ctx is cancelled, matching the daemon's background-service contract.
func (t *Tracker) Run(ctx context.Context) error {
	ticker := time.NewTicker(t.beat)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			// A clean exit closes what is open, so the next start has nothing to
			// recover and the time is credited to the moment we actually stopped.
			t.closeAll()
			return ctx.Err()
		case <-ticker.C:
			t.touch()
		}
	}
}

func (t *Tracker) touch() {
	t.mu.Lock()
	apps := make([]string, 0, len(t.running))
	for a := range t.running {
		apps = append(apps, a)
	}
	t.mu.Unlock()

	if len(apps) == 0 {
		return
	}
	if err := t.store.TouchSessions(apps, t.now()); err != nil {
		t.log.Warn("sessions: heartbeat failed", "err", err)
	}
}

func (t *Tracker) closeAll() {
	at := t.now()
	t.mu.Lock()
	apps := make([]string, 0, len(t.running))
	for a := range t.running {
		apps = append(apps, a)
	}
	t.running = map[string]struct{}{}
	t.mu.Unlock()

	for _, app := range apps {
		if err := t.store.EndSession(app, at); err != nil {
			t.log.Warn("sessions: close on shutdown failed", "app", app, "err", err)
		}
	}
}

// ---------------------------------------------------------------- service ---

// Service answers questions about recorded sessions.
type Service struct {
	store Store
	goal  time.Duration // daily goal; zero disables the progress sentence
	loc   *time.Location
	now   func() time.Time
}

// ServiceOptions configures a Service.
type ServiceOptions struct {
	Store     Store
	GoalHours float64
	Timezone  string // IANA name; a day starts at midnight here, not in UTC
	Now       func() time.Time
}

// NewService builds the read side. It returns nil when there is no store, so
// the caller can leave the service unregistered rather than register one that
// answers every question with an error.
func NewService(o ServiceOptions) *Service {
	if o.Store == nil {
		return nil
	}
	loc := time.Local
	if o.Timezone != "" {
		if l, err := time.LoadLocation(o.Timezone); err == nil {
			loc = l
		}
	}
	if o.Now == nil {
		o.Now = time.Now
	}
	goal := time.Duration(0)
	if o.GoalHours > 0 {
		goal = time.Duration(o.GoalHours * float64(time.Hour))
	}
	return &Service{store: o.Store, goal: goal, loc: loc, now: o.Now}
}

func (s *Service) Name() string { return "work" }

func (s *Service) Actions() []intent.ActionSpec {
	return []intent.ActionSpec{
		{
			Name: ActionToday,
			Desc: `"work.today": report how long the tracked apps have been used today, per app, against the daily goal. No args. Use for "bugün ne kadar çalıştım", "bugün kaç saat", "hedefimi tutturdum mu", "bugün ne kadar oyun oynadım".`,
		},
		{
			Name: ActionWeek,
			Desc: `"work.week": report the last seven days of tracked app use, per app, with the daily average. No args. Use for "bu hafta ne kadar çalıştım", "haftalık özet", "son bir haftada kaç saat".`,
		},
	}
}

func (s *Service) Execute(_ context.Context, action intent.Action, _ map[string]string) (string, error) {
	now := s.now().In(s.loc)
	switch action {
	case ActionToday:
		start := midnight(now)
		return s.summary(start, start.AddDate(0, 0, 1), now, "Bugün", 1)
	case ActionWeek:
		end := midnight(now).AddDate(0, 0, 1)
		return s.summary(end.AddDate(0, 0, -7), end, now, "Son 7 günde", 7)
	default:
		return "", fmt.Errorf("work: bilinmeyen aksiyon %q", action)
	}
}

// summary renders one window. days scales the goal so a weekly answer is judged
// against a week's worth of it.
func (s *Service) summary(from, to, now time.Time, lead string, days int) (string, error) {
	totals, err := s.store.SessionTotals(from, to, now)
	if err != nil {
		return "", err
	}
	if len(totals) == 0 {
		return lead + " takip edilen bir uygulamada zaman geçirmemişsin.", nil
	}

	var sum time.Duration
	parts := make([]string, 0, len(totals))
	for _, t := range totals {
		sum += t.Total
		parts = append(parts, fmt.Sprintf("%s %s", t.App, humanDuration(t.Total)))
	}

	// With a single app the breakdown *is* the total, and saying both ("Bugün 1
	// saat: code 1 saat") sounds broken read aloud.
	line := fmt.Sprintf("%s %s ile %s.", lead, totals[0].App, humanDuration(sum))
	if len(totals) > 1 {
		line = fmt.Sprintf("%s toplam %s: %s.", lead, humanDuration(sum), strings.Join(parts, ", "))
	}
	if s.goal > 0 {
		line += " " + s.goalSentence(sum, days)
	}
	return line, nil
}

// goalSentence phrases progress without number suffixes: Turkish agreement on
// "%80'i" changes with the digit, and getting it wrong is exactly the kind of
// thing a spoken reply makes obvious.
func (s *Service) goalSentence(sum time.Duration, days int) string {
	target := s.goal * time.Duration(days)
	if sum >= target {
		if days > 1 {
			return fmt.Sprintf("Haftalık hedefi (%s) geçtin.", humanDuration(target))
		}
		return fmt.Sprintf("Günlük hedefi (%s) geçtin.", humanDuration(target))
	}
	pct := int(float64(sum) / float64(target) * 100)
	label := "Günlük hedef"
	if days > 1 {
		label = "Haftalık hedef"
	}
	return fmt.Sprintf("%s %s, %%%d tamamlandı — %s kaldı.",
		label, humanDuration(target), pct, humanDuration(target-sum))
}

// midnight returns the start of the day t falls in, in t's own location.
func midnight(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, t.Location())
}

// humanDuration renders a duration the way it would be said out loud. Rounding
// to the minute is deliberate: seconds are noise in an answer about a day.
func humanDuration(d time.Duration) string {
	// Checked before rounding: half a minute rounds up to "1 dakika", which
	// overstates a session that barely happened.
	if d < time.Minute {
		return "bir dakikadan az"
	}
	d = d.Round(time.Minute)
	h := int(d / time.Hour)
	m := int((d % time.Hour) / time.Minute)
	switch {
	case h > 0 && m > 0:
		return fmt.Sprintf("%d saat %d dakika", h, m)
	case h > 0:
		return fmt.Sprintf("%d saat", h)
	case m > 0:
		return fmt.Sprintf("%d dakika", m)
	default:
		return "bir dakikadan az"
	}
}
