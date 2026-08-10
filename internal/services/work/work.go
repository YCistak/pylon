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
	"github.com/YCistak/pylon/internal/i18n"
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

// breakRepeat is how long the tracker stays quiet after nudging you about a
// break before nudging again. Ignoring the first nudge usually means "not now",
// not "never" — but repeating it every heartbeat would get the banner muted for
// good.
const breakRepeat = 30 * time.Minute

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

	breakAfter time.Duration     // unbroken stretch that earns a nudge; 0 disables
	nudge      func(text string) // how the nudge reaches you; nil disables

	mu      sync.Mutex
	running map[string]struct{}
	since   time.Time // start of the current stretch; zero when nothing is open
	nudged  time.Time // last nudge in this stretch; zero when not yet nudged
}

// TrackerOptions configures a Tracker.
type TrackerOptions struct {
	Store  Store
	Apps   []string         // process names to record sessions for
	Logger *slog.Logger     //
	Now    func() time.Time // injectable clock (tests)
	Beat   time.Duration    // heartbeat interval; zero means the default

	// BreakAfter nudges you to stand up once a tracked app has been open this
	// long without a gap; zero (the default) never nudges. Nudge receives the
	// finished sentence — the tracker words it, the caller decides whether that
	// is a banner, speech or a log line.
	BreakAfter time.Duration
	Nudge      func(text string)
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
		store:      o.Store,
		apps:       apps,
		log:        o.Logger,
		now:        o.Now,
		beat:       o.Beat,
		breakAfter: o.BreakAfter,
		nudge:      o.Nudge,
		running:    map[string]struct{}{},
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
		// First tracked app up after a quiet spell: a new unbroken stretch. Apps
		// opening on top of it don't restart the clock — the stretch is about you,
		// not about any one window.
		if len(t.running) == 1 {
			t.since, t.nudged = at, time.Time{}
		}
	case !isRunning && was:
		if err := t.store.EndSession(app, at); err != nil {
			t.log.Warn("sessions: end failed", "app", app, "err", err)
			return
		}
		delete(t.running, app)
		t.log.Info("sessions: ended", "app", app)
		// Everything closed — whatever you are doing now, it isn't this. The next
		// stretch starts from scratch.
		if len(t.running) == 0 {
			t.since, t.nudged = time.Time{}, time.Time{}
		}
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
			t.checkBreak()
		}
	}
}

// checkBreak nudges you to stand up when the current stretch has run past
// BreakAfter, then goes quiet for breakRepeat. It rides the heartbeat rather
// than its own timer so there is one clock deciding what "still open" means.
//
// The honest limitation: this measures how long a tracked app has been *open*,
// not how long you have been at the keyboard. An editor left open over lunch
// keeps the stretch running. That errs toward reminding you too often, which is
// the harmless direction for a banner that disappears by itself.
func (t *Tracker) checkBreak() {
	if t.breakAfter <= 0 || t.nudge == nil {
		return
	}
	now := t.now()

	t.mu.Lock()
	stretch := time.Duration(0)
	due := false
	if !t.since.IsZero() {
		stretch = now.Sub(t.since)
		switch {
		case stretch < t.breakAfter:
			// Not long enough yet.
		case t.nudged.IsZero():
			due = true // first nudge of this stretch
		case now.Sub(t.nudged) >= breakRepeat:
			due = true // you kept going; remind again
		}
	}
	if due {
		t.nudged = now
	}
	t.mu.Unlock()

	if !due {
		return
	}
	// Phrased without a suffix on the duration: "2 saat" and "45 dakika" would
	// need different Turkish endings, and a separate sentence sidesteps it.
	text := i18n.T("work.break_nudge", i18n.Duration(stretch))
	t.log.Info("sessions: break nudge", "stretch", stretch.Round(time.Minute))
	t.nudge(text)
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
	t.since, t.nudged = time.Time{}, time.Time{}
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
		return s.summary(start, start.AddDate(0, 0, 1), now, "today", 1)
	case ActionWeek:
		end := midnight(now).AddDate(0, 0, 1)
		return s.summary(end.AddDate(0, 0, -7), end, now, "week", 7)
	default:
		return "", fmt.Errorf("work: unknown action %q", action)
	}
}

// summary renders one window. window is "today" or "week" — a key suffix, not
// a phrase, because the period is part of the sentence in every language and
// cannot be prefixed onto it. days scales the goal so a weekly answer is judged
// against a week's worth of it.
func (s *Service) summary(from, to, now time.Time, window string, days int) (string, error) {
	totals, err := s.store.SessionTotals(from, to, now)
	if err != nil {
		return "", err
	}
	if len(totals) == 0 {
		return i18n.T("work." + window + ".nothing"), nil
	}

	var sum time.Duration
	parts := make([]string, 0, len(totals))
	for _, t := range totals {
		sum += t.Total
		parts = append(parts, fmt.Sprintf("%s %s", t.App, i18n.Duration(t.Total)))
	}

	// With a single app the breakdown *is* the total, and saying both ("Today 1
	// hour: code 1 hour") sounds broken read aloud.
	line := i18n.T("work."+window+".single", totals[0].App, i18n.Duration(sum))
	if len(totals) > 1 {
		line = i18n.T("work."+window+".total", i18n.Duration(sum), strings.Join(parts, ", "))
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
	period := "daily"
	if days > 1 {
		period = "weekly"
	}
	if sum >= target {
		return i18n.T("work.goal."+period+".met", i18n.Duration(target))
	}
	pct := int(float64(sum) / float64(target) * 100)
	return i18n.T("work.goal."+period+".progress",
		i18n.Duration(target), pct, i18n.Duration(target-sum))
}

// midnight returns the start of the day t falls in, in t's own location.
func midnight(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, t.Location())
}
