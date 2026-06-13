package db

import (
	"database/sql"
	"time"
)

// Task is a single queued reminder. A task may be triggered by a process
// exiting (TriggerProcess) and/or at a wall-clock time (TriggerTime).
type Task struct {
	ID             int64
	Content        string
	TriggerProcess string // "" if not process-triggered
	TriggerTime    *time.Time
	Done           bool
	CreatedAt      time.Time
}

// AddTask inserts a new task and returns its id.
func (d *DB) AddTask(t Task) (int64, error) {
	res, err := d.sql.Exec(
		`INSERT INTO tasks(content, trigger_process, trigger_time) VALUES (?, ?, ?)`,
		t.Content, nullStr(t.TriggerProcess), t.TriggerTime,
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// PendingForProcess returns undone tasks waiting on the given process name.
func (d *DB) PendingForProcess(process string) ([]Task, error) {
	rows, err := d.sql.Query(
		`SELECT id, content, trigger_process, trigger_time, done, created_at
		   FROM tasks
		  WHERE done = 0 AND trigger_process = ?
		  ORDER BY created_at`, process,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanTasks(rows)
}

// CompleteTask marks a task done.
func (d *DB) CompleteTask(id int64) error {
	_, err := d.sql.Exec(`UPDATE tasks SET done = 1 WHERE id = ?`, id)
	return err
}

// PendingTasks returns all undone tasks, newest last.
func (d *DB) PendingTasks() ([]Task, error) {
	rows, err := d.sql.Query(
		`SELECT id, content, trigger_process, trigger_time, done, created_at
		   FROM tasks WHERE done = 0 ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanTasks(rows)
}

func scanTasks(rows *sql.Rows) ([]Task, error) {
	var out []Task
	for rows.Next() {
		var (
			t       Task
			proc    sql.NullString
			trigger sql.NullTime
			done    int
		)
		if err := rows.Scan(&t.ID, &t.Content, &proc, &trigger, &done, &t.CreatedAt); err != nil {
			return nil, err
		}
		t.TriggerProcess = proc.String
		if trigger.Valid {
			tt := trigger.Time
			t.TriggerTime = &tt
		}
		t.Done = done != 0
		out = append(out, t)
	}
	return out, rows.Err()
}

func nullStr(s string) any {
	if s == "" {
		return nil
	}
	return s
}
