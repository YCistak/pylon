package watcher

import (
	"context"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"
)

// fakeLister returns process sets from a scripted sequence, advancing one step
// per call and holding on the last entry.
type fakeLister struct {
	mu    sync.Mutex
	steps []map[string]struct{}
	i     int
}

func set(names ...string) map[string]struct{} {
	m := make(map[string]struct{}, len(names))
	for _, n := range names {
		m[n] = struct{}{}
	}
	return m
}

func (f *fakeLister) list() (map[string]struct{}, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	s := f.steps[f.i]
	if f.i < len(f.steps)-1 {
		f.i++
	}
	return s, nil
}

// collectEvents runs a watcher until it has gathered want events or times out.
func collectEvents(t *testing.T, opts Options, want int) []Event {
	t.Helper()
	var (
		mu   sync.Mutex
		got  []Event
		done = make(chan struct{})
	)
	userOnEvent := opts.OnEvent
	opts.OnEvent = func(e Event) {
		if userOnEvent != nil {
			userOnEvent(e)
		}
		mu.Lock()
		got = append(got, e)
		if len(got) == want {
			select {
			case <-done:
			default:
				close(done)
			}
		}
		mu.Unlock()
	}
	if opts.Logger == nil {
		opts.Logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}

	w := New(opts)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	go w.Run(ctx)

	select {
	case <-done:
	case <-ctx.Done():
		t.Fatalf("timed out: got %d/%d events: %+v", len(got), want, got)
	}
	mu.Lock()
	defer mu.Unlock()
	return append([]Event(nil), got...)
}

func TestExitEventFires(t *testing.T) {
	// Baseline: code running. Then it disappears.
	f := &fakeLister{steps: []map[string]struct{}{
		set("code", "bash"), // baseline (seeded, no event)
		set("bash"),         // code exited
	}}
	events := collectEvents(t, Options{
		Names:    []string{"code"},
		Interval: 10 * time.Millisecond,
		List:     f.list,
	}, 1)

	if len(events) != 1 || events[0].Name != "code" || events[0].Kind != Exited {
		t.Fatalf("expected one Exited(code), got %+v", events)
	}
}

func TestStartEventFires(t *testing.T) {
	f := &fakeLister{steps: []map[string]struct{}{
		set("bash"),         // baseline: code not running
		set("bash", "code"), // code appeared
	}}
	events := collectEvents(t, Options{
		Names:    []string{"code"},
		Interval: 10 * time.Millisecond,
		List:     f.list,
	}, 1)

	if len(events) != 1 || events[0].Name != "code" || events[0].Kind != Started {
		t.Fatalf("expected one Started(code), got %+v", events)
	}
}

func TestUnwatchedProcessesIgnored(t *testing.T) {
	// firefox churns but we only watch code; only code's exit should surface.
	f := &fakeLister{steps: []map[string]struct{}{
		set("code", "firefox"),
		set("firefox"),         // code exited (watched)
		set(),                  // firefox exited (ignored)
		set("firefox", "code"), // both back; only code is a Started we watch
	}}
	events := collectEvents(t, Options{
		Names:    []string{"code"},
		Interval: 10 * time.Millisecond,
		List:     f.list,
	}, 2)

	for _, e := range events {
		if e.Name != "code" {
			t.Fatalf("unwatched process leaked into events: %+v", e)
		}
	}
	if events[0].Kind != Exited || events[1].Kind != Started {
		t.Fatalf("expected Exited then Started for code, got %+v", events)
	}
}

func TestNoProcessesConfiguredIdlesUntilCancel(t *testing.T) {
	w := New(Options{Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- w.Run(ctx) }()
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("idle watcher did not return after cancel")
	}
}
