// Package watcher observes a configured set of processes and emits Started /
// Exited events as they appear and disappear. It polls rather than hooking the
// kernel, which keeps it portable across platforms (the per-OS part is just
// "list running process names").
package watcher

import (
	"context"
	"log/slog"
	"time"
)

// Kind distinguishes process lifecycle transitions.
type Kind int

const (
	Started Kind = iota // a watched process appeared
	Exited              // a watched process went away
)

func (k Kind) String() string {
	if k == Started {
		return "started"
	}
	return "exited"
}

// Event is a single watched-process transition.
type Event struct {
	Name string
	Kind Kind
	At   time.Time
}

// Lister returns the set of currently-running process names. Only names matter
// (not PIDs): a watched app is "running" while at least one matching process
// exists. Implementations are platform-specific (see proc_*.go).
type Lister func() (map[string]struct{}, error)

// Watcher polls a Lister and reports transitions for the names it watches.
type Watcher struct {
	names      map[string]struct{}
	interval   time.Duration
	list       Lister
	onEvent    func(Event)
	onBaseline func([]string)
	log        *slog.Logger

	running map[string]struct{} // watched names seen on the last poll
}

// Options configures a Watcher.
type Options struct {
	Names    []string      // process names to watch
	Interval time.Duration // poll interval (default 2s)
	List     Lister        // process lister (default: platform lister)
	OnEvent  func(Event)   // called for every transition (must not block long)

	// OnBaseline reports the watched processes already running when Run starts.
	// Those produce no Started event on purpose — they are not transitions — but
	// a consumer that tracks *state* rather than changes would otherwise never
	// learn about the editor you left open across a restart, so it is handed the
	// starting set once, up front.
	OnBaseline func(running []string)

	Logger *slog.Logger
}

// New constructs a Watcher. Run must be called to begin polling.
func New(opts Options) *Watcher {
	if opts.Interval <= 0 {
		opts.Interval = 2 * time.Second
	}
	if opts.List == nil {
		opts.List = ListProcesses
	}
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}
	if opts.OnEvent == nil {
		opts.OnEvent = func(Event) {}
	}
	if opts.OnBaseline == nil {
		opts.OnBaseline = func([]string) {}
	}
	names := make(map[string]struct{}, len(opts.Names))
	for _, n := range opts.Names {
		if n != "" {
			names[n] = struct{}{}
		}
	}
	return &Watcher{
		names:      names,
		interval:   opts.Interval,
		list:       opts.List,
		onEvent:    opts.OnEvent,
		onBaseline: opts.OnBaseline,
		log:        opts.Logger,
		running:    make(map[string]struct{}),
	}
}

// Run polls until ctx is cancelled. It blocks, so callers typically launch it in
// a goroutine. The first poll seeds the baseline without emitting Started events
// for already-running processes (we only care about transitions from now on).
func (w *Watcher) Run(ctx context.Context) error {
	if len(w.names) == 0 {
		w.log.Info("watcher: no processes configured, idle")
		<-ctx.Done()
		return ctx.Err()
	}

	// Seed baseline: whatever is already running is the starting state.
	if cur, err := w.snapshot(); err == nil {
		w.running = cur
	} else {
		w.log.Warn("watcher: initial poll failed", "err", err)
	}
	// Reported even when that poll failed (an empty set), so a consumer waiting
	// on the baseline is never left hanging on a transient /proc read error.
	w.onBaseline(keys(w.running))

	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
	w.log.Info("watcher: started", "watching", keys(w.names), "interval", w.interval)

	for {
		select {
		case <-ctx.Done():
			w.log.Info("watcher: stopped")
			return ctx.Err()
		case <-ticker.C:
			w.poll()
		}
	}
}

// poll compares the current running set against the previous one and emits
// events for any transitions.
func (w *Watcher) poll() {
	cur, err := w.snapshot()
	if err != nil {
		w.log.Warn("watcher: poll failed", "err", err)
		return
	}
	now := time.Now()

	for name := range cur {
		if _, was := w.running[name]; !was {
			w.emit(Event{Name: name, Kind: Started, At: now})
		}
	}
	for name := range w.running {
		if _, still := cur[name]; !still {
			w.emit(Event{Name: name, Kind: Exited, At: now})
		}
	}
	w.running = cur
}

// snapshot returns the subset of running processes that we watch.
func (w *Watcher) snapshot() (map[string]struct{}, error) {
	all, err := w.list()
	if err != nil {
		return nil, err
	}
	out := make(map[string]struct{})
	for name := range w.names {
		if _, ok := all[name]; ok {
			out[name] = struct{}{}
		}
	}
	return out, nil
}

func (w *Watcher) emit(e Event) {
	w.log.Info("watcher: event", "process", e.Name, "kind", e.Kind.String())
	w.onEvent(e)
}

func keys(m map[string]struct{}) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
