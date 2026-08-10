package github

import (
	"context"
	"fmt"
	"github.com/YCistak/pylon/internal/i18n"
	"os/exec"
	"strings"
	"time"
)

// CommitReminder backs the daily "you haven't committed today" nudge (Phase
// 2.2). It inspects *local* git repos (complementing the remote API above) and
// reports the ones whose latest commit is not from today. The git lookup is
// injectable so the logic is testable without a real repo.
type CommitReminder struct {
	repos []string
	now   func() time.Time
	last  func(ctx context.Context, repo string) (time.Time, error)
}

// NewCommitReminder builds a reminder over the given local repo paths.
func NewCommitReminder(repos []string) *CommitReminder {
	return &CommitReminder{repos: repos, now: time.Now, last: lastCommit}
}

// Check returns a spoken nudge naming the repos with no commit today, and ok is
// false when every repo already has one (nothing to say). Unreadable paths (not
// a repo, or no commits yet) are skipped quietly.
func (c *CommitReminder) Check(ctx context.Context) (msg string, ok bool) {
	today := c.now()
	var missing []string
	for _, repo := range c.repos {
		t, err := c.last(ctx, repo)
		if err != nil {
			continue
		}
		if !sameDay(t.Local(), today) {
			missing = append(missing, repoName(repo))
		}
	}
	switch len(missing) {
	case 0:
		return "", false
	case 1:
		return i18n.T("github.no_commits_one", missing[0]), true
	default:
		return i18n.T("github.no_commits_many", strings.Join(missing, ", ")), true
	}
}

// lastCommit returns the time of the most recent commit on a repo's HEAD. It
// shells out to git, so it needs no CGo and uses whatever git the user has.
func lastCommit(ctx context.Context, repo string) (time.Time, error) {
	out, err := exec.CommandContext(ctx, "git", "-C", repo, "log", "-1", "--format=%cI").Output()
	if err != nil {
		return time.Time{}, fmt.Errorf("git log %s: %w", repo, err)
	}
	return time.Parse(time.RFC3339, strings.TrimSpace(string(out)))
}

func sameDay(a, b time.Time) bool {
	ay, am, ad := a.Date()
	by, bm, bd := b.Date()
	return ay == by && am == bm && ad == bd
}

func repoName(path string) string {
	path = strings.TrimRight(path, "/")
	if i := strings.LastIndexByte(path, '/'); i >= 0 {
		return path[i+1:]
	}
	return path
}
