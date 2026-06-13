// Package db is the SQLite persistence layer for Pylon: tasks, context memory,
// persona signals, and work sessions. It uses the pure-Go modernc.org/sqlite
// driver so the daemon cross-compiles without CGo.
package db

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

// DB wraps a SQLite connection with Pylon's schema applied.
type DB struct {
	sql *sql.DB
}

// Open opens (creating if needed) the SQLite database at path, ensures the
// parent directory exists, enables sane pragmas, and runs migrations.
// Use ":memory:" for an in-memory database (tests).
func Open(path string) (*DB, error) {
	if path != ":memory:" {
		if dir := filepath.Dir(path); dir != "" {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return nil, fmt.Errorf("create db dir: %w", err)
			}
		}
	}

	// Foreign keys on; WAL for concurrent reads while the daemon writes.
	dsn := path + "?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)"
	sqlDB, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	// modernc's driver is not safe for unbounded concurrent writers; serialize.
	sqlDB.SetMaxOpenConns(1)

	if err := sqlDB.Ping(); err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("ping sqlite: %w", err)
	}

	d := &DB{sql: sqlDB}
	if err := d.migrate(); err != nil {
		sqlDB.Close()
		return nil, err
	}
	return d, nil
}

// Close closes the underlying connection.
func (d *DB) Close() error { return d.sql.Close() }

// SQL exposes the raw connection for advanced/ad-hoc use (e.g. tests).
func (d *DB) SQL() *sql.DB { return d.sql }
