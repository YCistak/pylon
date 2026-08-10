package freshrss

import (
	"context"
	"strings"
	"testing"

	"github.com/YCistak/pylon/internal/intent"
)

type fakeFever struct {
	count int
	err   error
}

func (f *fakeFever) unreadCount(context.Context) (int, error) { return f.count, f.err }

func testFreshRSS(api feverAPI) *FreshRSS {
	return &FreshRSS{cfg: Config{URL: "https://rss.test", APIKey: "k"}, api: api}
}

func TestUnreadCountReply(t *testing.T) {
	got, err := testFreshRSS(&fakeFever{count: 5}).Execute(context.Background(), ActionUnreadCount, nil)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(got, "5 unread articles") {
		t.Fatalf("reply = %q", got)
	}
}

func TestUnreadCountZero(t *testing.T) {
	got, _ := testFreshRSS(&fakeFever{count: 0}).Execute(context.Background(), ActionUnreadCount, nil)
	if !strings.Contains(got, "no unread articles") {
		t.Fatalf("zero reply = %q", got)
	}
}

func TestExecuteErrorPropagates(t *testing.T) {
	if _, err := testFreshRSS(&fakeFever{err: context.DeadlineExceeded}).Execute(context.Background(), ActionUnreadCount, nil); err == nil {
		t.Fatal("api error should propagate")
	}
}

func TestUnknownAction(t *testing.T) {
	if _, err := testFreshRSS(&fakeFever{}).Execute(context.Background(), intent.Action("freshrss.nope"), nil); err == nil {
		t.Fatal("unknown action should error")
	}
}

func TestApiKey(t *testing.T) {
	// Explicit key wins.
	if got := apiKey(Config{APIKey: "abc", Username: "u", APIPassword: "p"}); got != "abc" {
		t.Fatalf("explicit key = %q", got)
	}
	// Derived key is a 32-char md5 hex and deterministic.
	k1 := apiKey(Config{Username: "user", APIPassword: "pass"})
	k2 := apiKey(Config{Username: "user", APIPassword: "pass"})
	if len(k1) != 32 || k1 != k2 {
		t.Fatalf("derived key not a stable md5 hex: %q", k1)
	}
	// Different credentials → different key.
	if apiKey(Config{Username: "user", APIPassword: "other"}) == k1 {
		t.Fatal("different password should change key")
	}
	// Missing pieces → empty.
	if apiKey(Config{Username: "user"}) != "" || apiKey(Config{}) != "" {
		t.Fatal("incomplete creds should yield empty key")
	}
}

func TestConfigured(t *testing.T) {
	if !Configured(Config{URL: "https://x", APIKey: "k"}) {
		t.Fatal("url+key should be configured")
	}
	if !Configured(Config{URL: "https://x", Username: "u", APIPassword: "p"}) {
		t.Fatal("url+user+pass should be configured")
	}
	if Configured(Config{APIKey: "k"}) {
		t.Fatal("no url should not be configured")
	}
	if Configured(Config{URL: "https://x"}) {
		t.Fatal("no key should not be configured")
	}
}

func TestCountIDs(t *testing.T) {
	cases := map[string]int{"": 0, "  ": 0, "1": 1, "1,2,3": 3, "10,20,30,40": 4}
	for in, want := range cases {
		if got := countIDs(in); got != want {
			t.Fatalf("countIDs(%q) = %d want %d", in, got, want)
		}
	}
}

func TestActionsDeclared(t *testing.T) {
	names := map[intent.Action]bool{}
	for _, s := range New(Config{}).Actions() {
		names[s.Name] = true
	}
	if !names[ActionUnreadCount] {
		t.Fatalf("missing freshrss action: %v", names)
	}
}

func TestNewTrimsURL(t *testing.T) {
	if f := New(Config{URL: "https://rss.test/"}); f.cfg.URL != "https://rss.test" {
		t.Fatalf("trailing slash not trimmed: %q", f.cfg.URL)
	}
}
