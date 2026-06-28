// Package scheduler runs named jobs on a clock: at a fixed interval, daily at a
// wall-clock time, or weekly on a given weekday. It drives Pylon's background
// reminders — the GitHub PR poll and commit reminder (Phase 2.2), and later the
// morning briefing and weekly report (Phase 3).
//
// A single Run loop ticks and fires every job whose time has come, so there is
// one goroutine and one injectable clock — the firing logic is deterministic and
// testable without sleeping. Each job's work runs in its own goroutine when due,
// so a slow job never delays the loop or the other jobs.
package scheduler

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

// job is a scheduled unit of work. nextAt computes the next fire time strictly
// after the time it is given; fireAt is when this job next runs.
type job struct {
	name   string
	fn     func(context.Context)
	nextAt func(after time.Time) time.Time
	fireAt time.Time
}

// Scheduler holds the registered jobs and loop configuration.
type Scheduler struct {
	tick time.Duration
	now  func() time.Time
	log  *slog.Logger

	mu   sync.Mutex
	jobs []*job
}

// Options configures a Scheduler.
type Options struct {
	Tick   time.Duration    // how often the loop checks for due jobs (default 30s)
	Now    func() time.Time // injectable clock (default time.Now)
	Logger *slog.Logger
}

// New constructs a Scheduler. Register jobs with Every/DailyAt/WeeklyAt, then
// call Run.
func New(opts Options) *Scheduler {
	if opts.Tick <= 0 {
		opts.Tick = 30 * time.Second
	}
	if opts.Now == nil {
		opts.Now = time.Now
	}
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}
	return &Scheduler{tick: opts.Tick, now: opts.Now, log: opts.Logger}
}

// Every registers a job that fires once per interval, the first time one
// interval after Run starts. A non-positive interval or nil fn is ignored.
func (s *Scheduler) Every(name string, every time.Duration, fn func(context.Context)) {
	if every <= 0 || fn == nil {
		return
	}
	s.add(name, fn, func(after time.Time) time.Time { return after.Add(every) })
}

// DailyAt registers a job that fires every day at hour:min (local time).
func (s *Scheduler) DailyAt(name string, hour, min int, fn func(context.Context)) {
	if fn == nil {
		return
	}
	s.add(name, fn, func(after time.Time) time.Time { return nextDaily(after, hour, min) })
}

// WeeklyAt registers a job that fires once a week on wd at hour:min (local time).
func (s *Scheduler) WeeklyAt(name string, wd time.Weekday, hour, min int, fn func(context.Context)) {
	if fn == nil {
		return
	}
	s.add(name, fn, func(after time.Time) time.Time { return nextWeekly(after, wd, hour, min) })
}

// Jobs reports how many jobs are registered.
func (s *Scheduler) Jobs() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.jobs)
}

func (s *Scheduler) add(name string, fn func(context.Context), nextAt func(time.Time) time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.jobs = append(s.jobs, &job{name: name, fn: fn, nextAt: nextAt})
}

// Run seeds each job's next fire time and then ticks until ctx is cancelled,
// firing jobs as they come due. It blocks, so the daemon registers it as a
// background service (run in a goroutine).
func (s *Scheduler) Run(ctx context.Context) error {
	s.seed()

	if s.Jobs() == 0 {
		s.log.Info("scheduler: no jobs, idle")
		<-ctx.Done()
		return ctx.Err()
	}

	ticker := time.NewTicker(s.tick)
	defer ticker.Stop()
	s.log.Info("scheduler: started", "jobs", s.Jobs(), "tick", s.tick)

	for {
		select {
		case <-ctx.Done():
			s.log.Info("scheduler: stopped")
			return ctx.Err()
		case <-ticker.C:
			s.fireDue(ctx)
		}
	}
}

// seed sets each job's first fire time relative to now.
func (s *Scheduler) seed() {
	now := s.now()
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, j := range s.jobs {
		j.fireAt = j.nextAt(now)
	}
}

// fireDue runs every job whose fire time has arrived and reschedules it.
func (s *Scheduler) fireDue(ctx context.Context) {
	now := s.now()
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, j := range s.jobs {
		if j.fireAt.After(now) {
			continue
		}
		s.log.Info("scheduler: fire", "job", j.name)
		go j.fn(ctx)
		j.fireAt = j.nextAt(now)
	}
}

// nextDaily returns the next hour:min strictly after `after`.
func nextDaily(after time.Time, hour, min int) time.Time {
	t := time.Date(after.Year(), after.Month(), after.Day(), hour, min, 0, 0, after.Location())
	if !t.After(after) {
		t = t.AddDate(0, 0, 1)
	}
	return t
}

// nextWeekly returns the next wd at hour:min strictly after `after`.
func nextWeekly(after time.Time, wd time.Weekday, hour, min int) time.Time {
	t := time.Date(after.Year(), after.Month(), after.Day(), hour, min, 0, 0, after.Location())
	if days := (int(wd) - int(after.Weekday()) + 7) % 7; days > 0 {
		t = t.AddDate(0, 0, days)
	}
	if !t.After(after) {
		t = t.AddDate(0, 0, 7)
	}
	return t
}
