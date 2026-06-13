package db

import (
	"database/sql"
	"time"
)

// Signal is one learned style trait. Weight is the recency-weighted frequency
// as of UpdatedAt; callers apply exponential decay relative to "now" before
// using or updating it (the decay math lives in the profile package, so the DB
// stays a dumb store).
type Signal struct {
	Signal    string // category, e.g. "address", "formality"
	Value     string // observed value, e.g. "kanka", "informal"
	Weight    float64
	UpdatedAt time.Time
}

// PersonaSignals returns all stored signals.
func (d *DB) PersonaSignals() ([]Signal, error) {
	rows, err := d.sql.Query(
		`SELECT signal, value, weight, updated_at FROM persona ORDER BY signal`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Signal
	for rows.Next() {
		var (
			s   Signal
			val sql.NullString
		)
		if err := rows.Scan(&s.Signal, &val, &s.Weight, &s.UpdatedAt); err != nil {
			return nil, err
		}
		s.Value = val.String
		out = append(out, s)
	}
	return out, rows.Err()
}

// UpsertPersonaSignal stores weight and stamps updated_at = now for the given
// signal key. The persona table's UNIQUE key is `signal`; to track competing
// values within a category the profile package uses composite keys of the form
// "category:value" (e.g. "address:kanka"), storing the bare value in `value`.
func (d *DB) UpsertPersonaSignal(signal, value string, weight float64) error {
	_, err := d.sql.Exec(
		`INSERT INTO persona(signal, value, weight, updated_at)
		      VALUES (?, ?, ?, CURRENT_TIMESTAMP)
		 ON CONFLICT(signal) DO UPDATE SET
		      value      = excluded.value,
		      weight     = excluded.weight,
		      updated_at = excluded.updated_at`,
		signal, nullStr(value), weight,
	)
	return err
}
