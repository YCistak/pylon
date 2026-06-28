package scheduler

import (
	"context"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"
)

// clock is a controllable time source for deterministic firing tests.
type clock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *clock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *clock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

func quiet() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

// base is an arbitrary fixed local time: Wed 2026-06-17 10:00.
var base = time.Date(2026, 6, 17, 10, 0, 0, 0, time.Local)

func TestEveryFiresAfterOneInterval(t *testing.T) {
	clk := &clock{t: base}
	fired := make(chan struct{}, 8)
	s := New(Options{Now: clk.now, Logger: quiet()})
	s.Every("poll", time.Hour, func(context.Context) { fired <- struct{}{} })
	s.seed()

	// Before the interval elapses, nothing fires.
	s.fireDue(context.Background())
	select {
	case <-fired:
		t.Fatal("fired before interval elapsed")
	case <-time.After(50 * time.Millisecond):
	}

	// After one interval, it fires once.
	clk.advance(time.Hour)
	s.fireDue(context.Background())
	waitFire(t, fired)

	// And again after the next interval — not before.
	s.fireDue(context.Background())
	select {
	case <-fired:
		t.Fatal("fired twice within one interval")
	case <-time.After(50 * time.Millisecond):
	}
	clk.advance(time.Hour)
	s.fireDue(context.Background())
	waitFire(t, fired)
}

func TestDailyAtFires(t *testing.T) {
	// base is 10:00; a 09:00 job should next fire tomorrow 09:00.
	clk := &clock{t: base}
	fired := make(chan struct{}, 4)
	s := New(Options{Now: clk.now, Logger: quiet()})
	s.DailyAt("morning", 9, 0, func(context.Context) { fired <- struct{}{} })
	s.seed()

	clk.advance(22 * time.Hour) // → next day 08:00, still before 09:00
	s.fireDue(context.Background())
	select {
	case <-fired:
		t.Fatal("fired before 09:00")
	case <-time.After(50 * time.Millisecond):
	}

	clk.advance(time.Hour) // → next day 09:00
	s.fireDue(context.Background())
	waitFire(t, fired)
}

func TestRunIdleNoJobs(t *testing.T) {
	s := New(Options{Logger: quiet()})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- s.Run(ctx) }()
	cancel()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("Run should return ctx error")
		}
	case <-time.After(time.Second):
		t.Fatal("Run did not stop on cancel")
	}
}

func TestNextDaily(t *testing.T) {
	at := func(h, m int) time.Time { return time.Date(2026, 6, 17, h, m, 0, 0, time.Local) }
	// 10:00 now → 22:00 today is later same day.
	if got := nextDaily(at(10, 0), 22, 0); !got.Equal(at(22, 0)) {
		t.Fatalf("nextDaily 22:00 = %v", got)
	}
	// 23:00 now → 22:00 already passed, roll to tomorrow.
	want := at(22, 0).AddDate(0, 0, 1)
	if got := nextDaily(at(23, 0), 22, 0); !got.Equal(want) {
		t.Fatalf("nextDaily rollover = %v want %v", got, want)
	}
	// Exactly at the target time → strictly after, so next day.
	if got := nextDaily(at(22, 0), 22, 0); !got.Equal(want) {
		t.Fatalf("nextDaily at-target should roll: %v", got)
	}
}

func TestNextWeekly(t *testing.T) {
	// base is Wednesday 2026-06-17 10:00.
	// Next Sunday 21:00 is 2026-06-21.
	want := time.Date(2026, 6, 21, 21, 0, 0, 0, time.Local)
	if got := nextWeekly(base, time.Sunday, 21, 0); !got.Equal(want) {
		t.Fatalf("nextWeekly Sunday = %v want %v", got, want)
	}
	// Same weekday but the time already passed today → next week.
	wed := time.Date(2026, 6, 17, 12, 0, 0, 0, time.Local)
	wantNext := time.Date(2026, 6, 24, 9, 0, 0, 0, time.Local)
	if got := nextWeekly(wed, time.Wednesday, 9, 0); !got.Equal(wantNext) {
		t.Fatalf("nextWeekly same-day-passed = %v want %v", got, wantNext)
	}
	// Same weekday, time still ahead today → today.
	wantToday := time.Date(2026, 6, 17, 18, 0, 0, 0, time.Local)
	if got := nextWeekly(wed, time.Wednesday, 18, 0); !got.Equal(wantToday) {
		t.Fatalf("nextWeekly same-day-ahead = %v want %v", got, wantToday)
	}
}

func waitFire(t *testing.T, fired <-chan struct{}) {
	t.Helper()
	select {
	case <-fired:
	case <-time.After(time.Second):
		t.Fatal("job did not fire")
	}
}
