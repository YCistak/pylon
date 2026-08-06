package db

import (
	"database/sql"
	"sort"
	"time"
)

// Session is one continuous run of a tracked app. End is zero while the app is
// still running; Seen is the last moment the daemon observed it running, and
// exists so a crash or a power cut can be recovered from (see CloseOpenSessions).
type Session struct {
	ID       int64
	App      string
	Start    time.Time
	End      time.Time
	Seen     time.Time
	Duration time.Duration
}

// Open reports whether the session is still running.
func (s Session) Open() bool { return s.End.IsZero() }

// AppTotal is the time spent in one app over a window.
type AppTotal struct {
	App   string
	Total time.Duration
}

// StartSession opens a session for app. Any session left open for the same app
// is closed at `at` first: two open rows for one app would double-count every
// query afterwards, and the only way to get there is a bug or a missed exit.
func (d *DB) StartSession(app string, at time.Time) (int64, error) {
	if err := d.EndSession(app, at); err != nil {
		return 0, err
	}
	res, err := d.sql.Exec(
		`INSERT INTO sessions(app, start, seen) VALUES (?, ?, ?)`,
		app, at.UTC(), at.UTC(),
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// EndSession closes the open session for app at `at`. Closing when nothing is
// open is not an error — the caller wants the app recorded as stopped, and it
// already is.
func (d *DB) EndSession(app string, at time.Time) error {
	rows, err := d.sql.Query(
		`SELECT id, start FROM sessions WHERE app = ? AND end IS NULL`, app)
	if err != nil {
		return err
	}
	type open struct {
		id    int64
		start time.Time
	}
	var opens []open
	for rows.Next() {
		var o open
		if err := rows.Scan(&o.id, &o.start); err != nil {
			rows.Close()
			return err
		}
		opens = append(opens, o)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	// The arithmetic is done here rather than in SQL because SQLite's date
	// functions depend on the exact text format the driver wrote, and getting
	// that subtly wrong would silently corrupt every duration.
	for _, o := range opens {
		if err := d.closeSession(o.id, o.start, at); err != nil {
			return err
		}
	}
	return nil
}

// closeSession writes the end timestamp and the derived duration. A clock that
// jumped backwards (NTP correction, suspend) must not produce negative time.
func (d *DB) closeSession(id int64, start, end time.Time) error {
	secs := int64(end.Sub(start).Seconds())
	if secs < 0 {
		secs = 0
	}
	_, err := d.sql.Exec(
		`UPDATE sessions SET end = ?, seen = ?, duration = ? WHERE id = ?`,
		end.UTC(), end.UTC(), secs, id,
	)
	return err
}

// TouchSessions refreshes the heartbeat for the named apps' open sessions. It is
// what makes an unclean shutdown recoverable: the loss is bounded by how often
// the caller calls this, not by how long the machine stayed off.
func (d *DB) TouchSessions(apps []string, at time.Time) error {
	for _, app := range apps {
		if _, err := d.sql.Exec(
			`UPDATE sessions SET seen = ? WHERE app = ? AND end IS NULL`,
			at.UTC(), app,
		); err != nil {
			return err
		}
	}
	return nil
}

// CloseOpenSessions closes every session still marked open, crediting each only
// up to its last heartbeat. Called at daemon start to clean up after a crash.
// Returns how many were closed.
//
// The current time is deliberately not used. A row is open here because the
// daemon died without seeing the app exit, and nothing that happened after the
// last heartbeat was observed — the machine may have been off for a day. A
// clean shutdown never reaches this path: the tracker closes its sessions at
// the real moment before the daemon exits.
func (d *DB) CloseOpenSessions() (int, error) {
	sessions, err := d.OpenSessions()
	if err != nil {
		return 0, err
	}
	for _, s := range sessions {
		// A session with no heartbeat yet (opened seconds before the crash) has
		// nothing to credit, so it collapses to zero rather than inventing time.
		end := s.Seen
		if end.IsZero() || end.Before(s.Start) {
			end = s.Start
		}
		if err := d.closeSession(s.ID, s.Start, end); err != nil {
			return 0, err
		}
	}
	return len(sessions), nil
}

// OpenSessions returns the sessions currently marked as running.
func (d *DB) OpenSessions() ([]Session, error) {
	rows, err := d.sql.Query(
		`SELECT id, app, start, seen FROM sessions WHERE end IS NULL ORDER BY start`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Session
	for rows.Next() {
		var s Session
		var seen sql.NullTime
		if err := rows.Scan(&s.ID, &s.App, &s.Start, &seen); err != nil {
			return nil, err
		}
		if seen.Valid {
			s.Seen = seen.Time
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// SessionTotals sums time per app inside [from, to), busiest app first.
//
// Sessions are clipped to the window rather than counted whole, so an evening
// that runs past midnight lands on both days in the right proportions. An open
// session counts up to `now`: the app you are in right now is the one you are
// most likely asking about, and leaving it out would report zero for a day of
// work that simply has not ended yet.
func (d *DB) SessionTotals(from, to, now time.Time) ([]AppTotal, error) {
	rows, err := d.sql.Query(
		`SELECT app, start, end FROM sessions
		  WHERE start < ? AND (end IS NULL OR end > ?)`,
		to.UTC(), from.UTC(),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	byApp := map[string]time.Duration{}
	for rows.Next() {
		var app string
		var start time.Time
		var end sql.NullTime
		if err := rows.Scan(&app, &start, &end); err != nil {
			return nil, err
		}
		finish := now
		if end.Valid {
			finish = end.Time
		}
		byApp[app] += overlap(start, finish, from, to)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	out := make([]AppTotal, 0, len(byApp))
	for app, total := range byApp {
		if total > 0 {
			out = append(out, AppTotal{App: app, Total: total})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Total != out[j].Total {
			return out[i].Total > out[j].Total
		}
		return out[i].App < out[j].App // stable output for equal totals
	})
	return out, nil
}

// overlap returns how much of [aStart, aEnd) falls inside [bStart, bEnd).
func overlap(aStart, aEnd, bStart, bEnd time.Time) time.Duration {
	if aStart.Before(bStart) {
		aStart = bStart
	}
	if aEnd.After(bEnd) {
		aEnd = bEnd
	}
	if !aEnd.After(aStart) {
		return 0
	}
	return aEnd.Sub(aStart)
}
