package db

import (
	"path/filepath"
	"testing"
	"time"
)

func openTestDB(t *testing.T) *DB {
	t.Helper()
	d, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { d.Close() })
	return d
}

func TestMigrateSetsVersionAndIsIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")

	d, err := Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	v, err := d.SchemaVersion()
	if err != nil {
		t.Fatalf("version: %v", err)
	}
	if v != len(migrations) {
		t.Fatalf("schema version = %d, want %d", v, len(migrations))
	}
	d.Close()

	// Reopening must not re-run or fail on existing tables.
	d2, err := Open(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer d2.Close()
	v2, _ := d2.SchemaVersion()
	if v2 != len(migrations) {
		t.Fatalf("version after reopen = %d, want %d", v2, len(migrations))
	}
}

func TestTaskLifecycle(t *testing.T) {
	d := openTestDB(t)

	id, err := d.AddTask(Task{Content: "message teacher", TriggerProcess: "code"})
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	if id == 0 {
		t.Fatal("expected non-zero id")
	}

	// Unrelated process yields nothing.
	if got, _ := d.PendingForProcess("cs2"); len(got) != 0 {
		t.Fatalf("expected 0 tasks for cs2, got %d", len(got))
	}

	pending, err := d.PendingForProcess("code")
	if err != nil {
		t.Fatalf("pending: %v", err)
	}
	if len(pending) != 1 || pending[0].Content != "message teacher" {
		t.Fatalf("unexpected pending: %+v", pending)
	}

	if err := d.CompleteTask(id); err != nil {
		t.Fatalf("complete: %v", err)
	}
	if got, _ := d.PendingForProcess("code"); len(got) != 0 {
		t.Fatalf("task should be done, still pending: %d", len(got))
	}
}

func TestTaskWithTriggerTime(t *testing.T) {
	d := openTestDB(t)
	when := time.Date(2026, 6, 14, 15, 0, 0, 0, time.UTC)
	if _, err := d.AddTask(Task{Content: "standup", TriggerTime: &when}); err != nil {
		t.Fatalf("add: %v", err)
	}
	all, err := d.PendingTasks()
	if err != nil {
		t.Fatalf("pending: %v", err)
	}
	if len(all) != 1 {
		t.Fatalf("want 1 task, got %d", len(all))
	}
	if all[0].TriggerTime == nil || !all[0].TriggerTime.Equal(when) {
		t.Fatalf("trigger time round-trip failed: %+v", all[0].TriggerTime)
	}
}
