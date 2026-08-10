package github

import (
	"context"
	"strings"
	"testing"

	"github.com/YCistak/pylon/internal/intent"
)

// fakeGH returns canned results keyed by a substring of the search query, so a
// single fake can answer the review-requested, authored, and assignee searches.
type fakeGH struct {
	byQuery map[string]struct {
		total int
		items []Item
	}
	queries []string
	err     error
}

func (f *fakeGH) search(_ context.Context, query string) (int, []Item, error) {
	f.queries = append(f.queries, query)
	if f.err != nil {
		return 0, nil, f.err
	}
	for key, res := range f.byQuery {
		if strings.Contains(query, key) {
			return res.total, res.items, nil
		}
	}
	return 0, nil, nil
}

func testGitHub(api ghAPI) *GitHub {
	return &GitHub{cfg: Config{Token: "x", BaseURL: defaultBaseURL}, api: api}
}

func res(total int, items ...Item) struct {
	total int
	items []Item
} {
	return struct {
		total int
		items []Item
	}{total, items}
}

func TestListPRsBoth(t *testing.T) {
	f := &fakeGH{byQuery: map[string]struct {
		total int
		items []Item
	}{
		"review-requested": res(2, Item{Title: "Fix auth", Repo: "me/app", Number: 12}),
		"author:@me":       res(1, Item{Title: "Add cache", Repo: "me/app", Number: 9}),
	}}
	got, err := testGitHub(f).Execute(context.Background(), ActionListPRs, nil)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	for _, want := range []string{"incelemen istenen 2 PR", "me/app#12 Fix auth", "açık 1 PR'ın", "me/app#9 Add cache"} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in %q", want, got)
		}
	}
}

func TestListPRsEmpty(t *testing.T) {
	got, _ := testGitHub(&fakeGH{}).Execute(context.Background(), ActionListPRs, nil)
	if !strings.Contains(got, "Bekleyen PR yok") {
		t.Fatalf("empty = %q", got)
	}
}

func TestListPRsCaps(t *testing.T) {
	f := &fakeGH{byQuery: map[string]struct {
		total int
		items []Item
	}{
		"review-requested": res(5,
			Item{Title: "a", Repo: "r", Number: 1},
			Item{Title: "b", Repo: "r", Number: 2},
			Item{Title: "c", Repo: "r", Number: 3},
			Item{Title: "d", Repo: "r", Number: 4},
		),
	}}
	got, _ := testGitHub(f).Execute(context.Background(), ActionListPRs, nil)
	// Total count is reported even though only 3 titles are listed (+ ellipsis).
	if !strings.Contains(got, "istenen 5 PR") || !strings.Contains(got, "…") {
		t.Fatalf("cap/total = %q", got)
	}
	if strings.Contains(got, "r#4") {
		t.Fatalf("should not list 4th item: %q", got)
	}
}

func TestListIssues(t *testing.T) {
	f := &fakeGH{byQuery: map[string]struct {
		total int
		items []Item
	}{
		"assignee:@me": res(1, Item{Title: "Crash on start", Repo: "me/app", Number: 7}),
	}}
	got, _ := testGitHub(f).Execute(context.Background(), ActionListIssues, nil)
	for _, want := range []string{"1 issue assigned to you", "me/app#7 Crash on start"} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in %q", want, got)
		}
	}
}

func TestListIssuesEmpty(t *testing.T) {
	got, _ := testGitHub(&fakeGH{}).Execute(context.Background(), ActionListIssues, nil)
	if !strings.Contains(got, "no open issues assigned") {
		t.Fatalf("empty = %q", got)
	}
}

func TestSearchErrorPropagates(t *testing.T) {
	if _, err := testGitHub(&fakeGH{err: context.DeadlineExceeded}).Execute(context.Background(), ActionListPRs, nil); err == nil {
		t.Fatal("search error should propagate")
	}
}

func TestNoTokenErrors(t *testing.T) {
	// No injected api and no token → client() should refuse.
	if _, err := New(Config{}).Execute(context.Background(), ActionListPRs, nil); err == nil {
		t.Fatal("missing token should error")
	}
}

func TestRepoFromURL(t *testing.T) {
	if got := repoFromURL("https://api.github.com/repos/me/app"); got != "me/app" {
		t.Fatalf("repoFromURL = %q", got)
	}
	if got := repoFromURL("garbage"); got != "" {
		t.Fatalf("repoFromURL(garbage) = %q", got)
	}
}

func TestActionsDeclared(t *testing.T) {
	names := map[intent.Action]bool{}
	for _, s := range New(Config{}).Actions() {
		names[s.Name] = true
	}
	if !names[ActionListPRs] || !names[ActionListIssues] {
		t.Fatalf("missing github actions: %v", names)
	}
}
