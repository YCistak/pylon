package github

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestPollAnnouncesOnlyNew(t *testing.T) {
	f := &fakeGH{byQuery: map[string]struct {
		total int
		items []Item
	}{
		"review-requested": res(1, Item{Title: "Fix auth", Repo: "me/app", Number: 12}),
	}}
	p := testGitHub(f).NewPoller()

	// First poll: the outstanding PR is new → announced.
	msg, ok, err := p.Poll(context.Background())
	if err != nil {
		t.Fatalf("poll: %v", err)
	}
	if !ok || !contains(msg, "1 yeni PR") || !contains(msg, "me/app#12 Fix auth") {
		t.Fatalf("first poll = %q ok=%v", msg, ok)
	}

	// Second poll, same result: nothing new → silent.
	if _, ok, _ := p.Poll(context.Background()); ok {
		t.Fatal("second poll should be silent")
	}

	// A new PR appears → only that one is announced.
	f.byQuery["review-requested"] = res(2,
		Item{Title: "Fix auth", Repo: "me/app", Number: 12},
		Item{Title: "Add cache", Repo: "me/app", Number: 15},
	)
	msg, ok, _ = p.Poll(context.Background())
	if !ok || !contains(msg, "1 yeni PR") || !contains(msg, "me/app#15 Add cache") || contains(msg, "#12") {
		t.Fatalf("third poll = %q ok=%v", msg, ok)
	}
}

func TestPollEmptySilent(t *testing.T) {
	if _, ok, _ := testGitHub(&fakeGH{}).NewPoller().Poll(context.Background()); ok {
		t.Fatal("empty poll should be silent")
	}
}

func TestCommitReminder(t *testing.T) {
	today := time.Date(2026, 6, 17, 22, 0, 0, 0, time.Local)
	commits := map[string]time.Time{
		"/home/me/pylon": time.Date(2026, 6, 17, 9, 0, 0, 0, time.Local),  // committed today
		"/home/me/flint": time.Date(2026, 6, 16, 18, 0, 0, 0, time.Local), // yesterday → missing
	}
	cr := &CommitReminder{
		repos: []string{"/home/me/pylon", "/home/me/flint"},
		now:   func() time.Time { return today },
		last:  func(_ context.Context, repo string) (time.Time, error) { return commits[repo], nil },
	}
	msg, ok := cr.Check(context.Background())
	if !ok || !contains(msg, "flint") || contains(msg, "pylon") {
		t.Fatalf("check = %q ok=%v", msg, ok)
	}
}

func TestCommitReminderAllCommitted(t *testing.T) {
	today := time.Date(2026, 6, 17, 22, 0, 0, 0, time.Local)
	cr := &CommitReminder{
		repos: []string{"/home/me/pylon"},
		now:   func() time.Time { return today },
		last:  func(context.Context, string) (time.Time, error) { return today, nil },
	}
	if _, ok := cr.Check(context.Background()); ok {
		t.Fatal("all-committed should be silent")
	}
}

func TestCommitReminderSkipsUnreadable(t *testing.T) {
	today := time.Date(2026, 6, 17, 22, 0, 0, 0, time.Local)
	cr := &CommitReminder{
		repos: []string{"/not/a/repo"},
		now:   func() time.Time { return today },
		last:  func(context.Context, string) (time.Time, error) { return time.Time{}, context.Canceled },
	}
	if _, ok := cr.Check(context.Background()); ok {
		t.Fatal("unreadable repo should be skipped, not reported")
	}
}

func contains(s, sub string) bool { return strings.Contains(s, sub) }
