package db

import (
	"testing"
	"time"
)

// Timestamps survive the round-trip through SQLite as the same instant. This is
// the assumption every duration here rests on, so it is checked directly rather
// than inferred from a total coming out right.
func TestSessionTimestampRoundTrip(t *testing.T) {
	d := openTestDB(t)
	start := time.Date(2026, 8, 7, 9, 30, 15, 0, time.FixedZone("+03", 3*60*60))

	if _, err := d.StartSession("code", start); err != nil {
		t.Fatalf("start: %v", err)
	}
	open, err := d.OpenSessions()
	if err != nil {
		t.Fatalf("open sessions: %v", err)
	}
	if len(open) != 1 {
		t.Fatalf("expected 1 open session, got %d", len(open))
	}
	if !open[0].Start.Equal(start) {
		t.Fatalf("start round-tripped to %v, want %v", open[0].Start, start)
	}
}

func TestSessionStartAndEnd(t *testing.T) {
	d := openTestDB(t)
	start := time.Date(2026, 8, 7, 10, 0, 0, 0, time.UTC)

	if _, err := d.StartSession("code", start); err != nil {
		t.Fatalf("start: %v", err)
	}
	if err := d.EndSession("code", start.Add(90*time.Minute)); err != nil {
		t.Fatalf("end: %v", err)
	}

	open, err := d.OpenSessions()
	if err != nil {
		t.Fatalf("open sessions: %v", err)
	}
	if len(open) != 0 {
		t.Fatalf("session stayed open: %+v", open)
	}

	totals, err := d.SessionTotals(start.Add(-time.Hour), start.Add(4*time.Hour), start.Add(4*time.Hour))
	if err != nil {
		t.Fatalf("totals: %v", err)
	}
	if len(totals) != 1 || totals[0].App != "code" || totals[0].Total != 90*time.Minute {
		t.Fatalf("totals = %+v", totals)
	}
}

// Ending a session that was never opened is the shape of a missed Started event
// or a double Exited. It must not error and must not invent a row.
func TestEndSessionWithNothingOpenIsNoOp(t *testing.T) {
	d := openTestDB(t)
	if err := d.EndSession("cs2", time.Now()); err != nil {
		t.Fatalf("end with nothing open: %v", err)
	}
	totals, err := d.SessionTotals(time.Now().Add(-time.Hour), time.Now(), time.Now())
	if err != nil {
		t.Fatalf("totals: %v", err)
	}
	if len(totals) != 0 {
		t.Fatalf("expected no totals, got %+v", totals)
	}
}

// Two open rows for one app would double-count every query, so a second start
// closes the first.
func TestStartSessionClosesAStaleOpenOne(t *testing.T) {
	d := openTestDB(t)
	t0 := time.Date(2026, 8, 7, 10, 0, 0, 0, time.UTC)

	if _, err := d.StartSession("code", t0); err != nil {
		t.Fatalf("start: %v", err)
	}
	if _, err := d.StartSession("code", t0.Add(time.Hour)); err != nil {
		t.Fatalf("restart: %v", err)
	}

	open, err := d.OpenSessions()
	if err != nil {
		t.Fatalf("open sessions: %v", err)
	}
	if len(open) != 1 {
		t.Fatalf("expected exactly one open session, got %d", len(open))
	}

	// The first hour is still credited; only the overlap is gone.
	totals, err := d.SessionTotals(t0, t0.Add(2*time.Hour), t0.Add(2*time.Hour))
	if err != nil {
		t.Fatalf("totals: %v", err)
	}
	if len(totals) != 1 || totals[0].Total != 2*time.Hour {
		t.Fatalf("totals = %+v, want a single 2h entry", totals)
	}
}

// An open session counts up to now — the app you are in right now is usually
// the one being asked about.
func TestSessionTotalsCountsOpenSessionUpToNow(t *testing.T) {
	d := openTestDB(t)
	t0 := time.Date(2026, 8, 7, 10, 0, 0, 0, time.UTC)
	now := t0.Add(45 * time.Minute)

	if _, err := d.StartSession("code", t0); err != nil {
		t.Fatalf("start: %v", err)
	}

	totals, err := d.SessionTotals(t0.Add(-time.Hour), now, now)
	if err != nil {
		t.Fatalf("totals: %v", err)
	}
	if len(totals) != 1 || totals[0].Total != 45*time.Minute {
		t.Fatalf("totals = %+v", totals)
	}
}

// A session that runs past midnight belongs to both days, split at the boundary
// — not counted whole on either.
func TestSessionTotalsClipsToWindow(t *testing.T) {
	d := openTestDB(t)
	start := time.Date(2026, 8, 6, 23, 0, 0, 0, time.UTC)
	end := time.Date(2026, 8, 7, 2, 0, 0, 0, time.UTC)
	midnight := time.Date(2026, 8, 7, 0, 0, 0, 0, time.UTC)

	if _, err := d.StartSession("cs2", start); err != nil {
		t.Fatalf("start: %v", err)
	}
	if err := d.EndSession("cs2", end); err != nil {
		t.Fatalf("end: %v", err)
	}

	yesterday, err := d.SessionTotals(midnight.Add(-24*time.Hour), midnight, end)
	if err != nil {
		t.Fatalf("totals: %v", err)
	}
	if len(yesterday) != 1 || yesterday[0].Total != time.Hour {
		t.Fatalf("yesterday = %+v, want 1h", yesterday)
	}

	today, err := d.SessionTotals(midnight, midnight.Add(24*time.Hour), end)
	if err != nil {
		t.Fatalf("totals: %v", err)
	}
	if len(today) != 1 || today[0].Total != 2*time.Hour {
		t.Fatalf("today = %+v, want 2h", today)
	}
}

func TestSessionTotalsSortsBusiestFirst(t *testing.T) {
	d := openTestDB(t)
	t0 := time.Date(2026, 8, 7, 8, 0, 0, 0, time.UTC)

	for _, s := range []struct {
		app string
		dur time.Duration
	}{{"steam", 20 * time.Minute}, {"code", 3 * time.Hour}, {"cs2", time.Hour}} {
		if _, err := d.StartSession(s.app, t0); err != nil {
			t.Fatalf("start %s: %v", s.app, err)
		}
		if err := d.EndSession(s.app, t0.Add(s.dur)); err != nil {
			t.Fatalf("end %s: %v", s.app, err)
		}
	}

	totals, err := d.SessionTotals(t0, t0.Add(6*time.Hour), t0.Add(6*time.Hour))
	if err != nil {
		t.Fatalf("totals: %v", err)
	}
	got := []string{}
	for _, tot := range totals {
		got = append(got, tot.App)
	}
	if len(got) != 3 || got[0] != "code" || got[1] != "cs2" || got[2] != "steam" {
		t.Fatalf("order = %v", got)
	}
}

// The crash case: the daemon dies without seeing the app exit. The next start
// must credit the session up to its last heartbeat — not zero, and not the
// hours the machine spent powered off.
func TestCloseOpenSessionsCreditsUpToLastHeartbeat(t *testing.T) {
	d := openTestDB(t)
	t0 := time.Date(2026, 8, 7, 9, 0, 0, 0, time.UTC)

	if _, err := d.StartSession("code", t0); err != nil {
		t.Fatalf("start: %v", err)
	}
	if err := d.TouchSessions([]string{"code"}, t0.Add(2*time.Hour)); err != nil {
		t.Fatalf("touch: %v", err)
	}

	// Restart happens a day later; the heartbeat is what bounds the credit.
	n, err := d.CloseOpenSessions()
	if err != nil {
		t.Fatalf("close: %v", err)
	}
	if n != 1 {
		t.Fatalf("closed %d sessions, want 1", n)
	}

	totals, err := d.SessionTotals(t0, t0.Add(48*time.Hour), t0.Add(48*time.Hour))
	if err != nil {
		t.Fatalf("totals: %v", err)
	}
	if len(totals) != 1 || totals[0].Total != 2*time.Hour {
		t.Fatalf("totals = %+v, want 2h (up to the last heartbeat)", totals)
	}
}

// A session that never got a heartbeat has nothing to credit; it must collapse
// to zero rather than invent the time since it started.
func TestCloseOpenSessionsWithoutHeartbeatCreditsNothing(t *testing.T) {
	d := openTestDB(t)
	t0 := time.Date(2026, 8, 7, 9, 0, 0, 0, time.UTC)

	if _, err := d.sql.Exec(`INSERT INTO sessions(app, start) VALUES (?, ?)`, "cs2", t0.UTC()); err != nil {
		t.Fatalf("insert: %v", err)
	}
	if _, err := d.CloseOpenSessions(); err != nil {
		t.Fatalf("close: %v", err)
	}

	totals, err := d.SessionTotals(t0, t0.Add(24*time.Hour), t0.Add(24*time.Hour))
	if err != nil {
		t.Fatalf("totals: %v", err)
	}
	if len(totals) != 0 {
		t.Fatalf("totals = %+v, want nothing credited", totals)
	}
}

// A clock jumping backwards (NTP, resume from suspend) must not produce a
// negative duration that then cancels out real work.
func TestCloseSessionClampsBackwardsClock(t *testing.T) {
	d := openTestDB(t)
	t0 := time.Date(2026, 8, 7, 9, 0, 0, 0, time.UTC)

	if _, err := d.StartSession("code", t0); err != nil {
		t.Fatalf("start: %v", err)
	}
	if err := d.EndSession("code", t0.Add(-time.Hour)); err != nil {
		t.Fatalf("end: %v", err)
	}

	var secs int64
	if err := d.sql.QueryRow(`SELECT duration FROM sessions WHERE app = 'code'`).Scan(&secs); err != nil {
		t.Fatalf("read duration: %v", err)
	}
	if secs != 0 {
		t.Fatalf("duration = %d, want 0", secs)
	}
}
