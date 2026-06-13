package db

import (
	"database/sql"
	"time"
)

// ContextEntry is one key/value memory fact (Phase 1.8). Keys are unique;
// writing an existing key updates its value and timestamp.
type ContextEntry struct {
	Key       string
	Value     string
	UpdatedAt time.Time
}

// SetContext upserts a context fact by key.
func (d *DB) SetContext(key, value string) error {
	_, err := d.sql.Exec(
		`INSERT INTO context(key, value, updated_at)
		      VALUES (?, ?, CURRENT_TIMESTAMP)
		 ON CONFLICT(key) DO UPDATE SET
		      value      = excluded.value,
		      updated_at = excluded.updated_at`,
		key, value,
	)
	return err
}

// GetContext returns the value for key and whether it was found.
func (d *DB) GetContext(key string) (string, bool, error) {
	var value string
	err := d.sql.QueryRow(`SELECT value FROM context WHERE key = ?`, key).Scan(&value)
	if err == sql.ErrNoRows {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return value, true, nil
}

// RecentContext returns the most recently updated entries, newest first.
func (d *DB) RecentContext(limit int) ([]ContextEntry, error) {
	if limit <= 0 {
		limit = 10
	}
	rows, err := d.sql.Query(
		`SELECT key, value, updated_at FROM context
		  ORDER BY updated_at DESC, id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []ContextEntry
	for rows.Next() {
		var e ContextEntry
		if err := rows.Scan(&e.Key, &e.Value, &e.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
