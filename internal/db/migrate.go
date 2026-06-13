package db

import "fmt"

// migrations are applied in order. Each entry is run exactly once; the index+1
// is recorded as the schema version. Never edit or reorder an applied
// migration — append a new one instead.
var migrations = []string{
	// 1: tasks queue (Phase 1.7)
	`CREATE TABLE tasks (
		id              INTEGER PRIMARY KEY AUTOINCREMENT,
		content         TEXT    NOT NULL,
		trigger_process TEXT,
		trigger_time    DATETIME,
		done            INTEGER NOT NULL DEFAULT 0,
		created_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	);
	CREATE INDEX idx_tasks_trigger_process ON tasks(trigger_process) WHERE done = 0;`,

	// 2: context memory (Phase 1.8)
	`CREATE TABLE context (
		id         INTEGER PRIMARY KEY AUTOINCREMENT,
		key        TEXT NOT NULL UNIQUE,
		value      TEXT NOT NULL,
		updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	);`,

	// 3: persona style signals (Phase 1.9). weight carries exponentially-decayed
	// frequency; updated_at anchors the decay computation.
	`CREATE TABLE persona (
		id         INTEGER PRIMARY KEY AUTOINCREMENT,
		signal     TEXT NOT NULL UNIQUE,
		value      TEXT,
		weight     REAL NOT NULL DEFAULT 0,
		updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	);`,

	// 4: work sessions (Phase 3.2). end is NULL while a session is open.
	`CREATE TABLE sessions (
		id       INTEGER PRIMARY KEY AUTOINCREMENT,
		app      TEXT NOT NULL,
		start    DATETIME NOT NULL,
		end      DATETIME,
		duration INTEGER
	);
	CREATE INDEX idx_sessions_app_start ON sessions(app, start);`,
}

// migrate applies any migrations not yet recorded in schema_version.
func (d *DB) migrate() error {
	if _, err := d.sql.Exec(`CREATE TABLE IF NOT EXISTS schema_version (version INTEGER NOT NULL)`); err != nil {
		return fmt.Errorf("create schema_version: %w", err)
	}

	var current int
	if err := d.sql.QueryRow(`SELECT COALESCE(MAX(version), 0) FROM schema_version`).Scan(&current); err != nil {
		return fmt.Errorf("read schema version: %w", err)
	}

	for i := current; i < len(migrations); i++ {
		version := i + 1
		tx, err := d.sql.Begin()
		if err != nil {
			return fmt.Errorf("begin migration %d: %w", version, err)
		}
		if _, err := tx.Exec(migrations[i]); err != nil {
			tx.Rollback()
			return fmt.Errorf("apply migration %d: %w", version, err)
		}
		if _, err := tx.Exec(`INSERT INTO schema_version(version) VALUES (?)`, version); err != nil {
			tx.Rollback()
			return fmt.Errorf("record migration %d: %w", version, err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit migration %d: %w", version, err)
		}
	}
	return nil
}

// SchemaVersion returns the highest applied migration version.
func (d *DB) SchemaVersion() (int, error) {
	var v int
	err := d.sql.QueryRow(`SELECT COALESCE(MAX(version), 0) FROM schema_version`).Scan(&v)
	return v, err
}
