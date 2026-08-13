// Package feedback turns something a user typed into a filed GitHub issue.
//
// It exists because the alternative shipped for a while: nothing. Feedback
// arrived only from people who already knew the project had a tracker, found
// it, and had an account — which selects for contributors and against exactly
// the users worth hearing from.
//
// Two things it deliberately does not do. It sends nothing the user has not
// been shown: the diagnostics are a fixed, short line the form displays under
// the box, not a log, a config dump, or anything read off disk. And it invents
// no identity — the issue is filed with the token already in the vault, under
// the account that owns it, or not at all.
package feedback

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// repo is where feedback lands. Hard-coded for the same reason the update
// endpoint is: a configurable destination for a "send feedback" button is a
// way to get a user's words delivered somewhere they did not intend.
const repo = "YCistak/pylon"

// Categories are the kinds of feedback the form offers, in the order it offers
// them. They are ids, not labels — the interface translates them, and they end
// up as GitHub labels, which are not translated.
var Categories = []string{"bug", "idea", "question", "other"}

// Valid reports whether c is one of Categories. The GUI sends the id straight
// through, and an unknown one would become a label nobody is watching.
func Valid(c string) bool {
	for _, known := range Categories {
		if c == known {
			return true
		}
	}
	return false
}

// Env is the handful of facts attached to a report, and the whole of it. Enough
// to tell a Wayland bug from an X11 one without asking; not enough to identify
// anybody.
type Env struct {
	Version  string // the daemon's
	OS       string // runtime.GOOS
	Desktop  string // hyprland, gnome, windows…
	Language string // what Pylon is speaking
}

// Line renders Env as the single line the form shows the user before they send.
// The form displays exactly this, so what is on screen is what is submitted.
func (e Env) Line() string {
	parts := []string{}
	for _, v := range []string{e.Version, e.OS, e.Desktop, e.Language} {
		if v = strings.TrimSpace(v); v != "" {
			parts = append(parts, v)
		}
	}
	return strings.Join(parts, " · ")
}

// Report is one piece of feedback.
type Report struct {
	Category string
	Body     string
	Env      Env
}

// maxTitle keeps the issue list readable. GitHub allows far more, but a title
// that is a whole paragraph is a title nobody can scan.
const maxTitle = 72

// Title is the issue title: the report's first line, trimmed, tagged with the
// category. A body with no usable first line still gets a title, because an
// issue called "" is worse than one called "bug".
func (r Report) Title() string {
	first := strings.TrimSpace(strings.SplitN(strings.TrimSpace(r.Body), "\n", 2)[0])
	if first == "" {
		return "[" + r.Category + "]"
	}
	if len([]rune(first)) > maxTitle {
		first = strings.TrimSpace(string([]rune(first)[:maxTitle])) + "…"
	}
	return "[" + r.Category + "] " + first
}

// Issue is the body as filed: what the user wrote, then the diagnostics, with a
// rule between so the two are never mistaken for each other.
func (r Report) Issue() string {
	body := strings.TrimSpace(r.Body)
	line := r.Env.Line()
	if line == "" {
		return body
	}
	return body + "\n\n---\n" + line
}

// BrowserURL is where to send someone whose vault holds no GitHub token: the
// new-issue form, prefilled. It is the fallback rather than the default because
// it drops the user into a login flow to say one sentence — but a button that
// dead-ends for everyone without a token would be worse than either.
func (r Report) BrowserURL() string {
	q := url.Values{}
	q.Set("title", r.Title())
	q.Set("body", r.Issue())
	q.Set("labels", r.Category)
	return "https://github.com/" + repo + "/issues/new?" + q.Encode()
}

// Submit files the report and returns the URL of the issue it created.
//
// The token is the user's own, the one already in the vault for the GitHub
// widget, so the issue is filed under their account and is visible to them
// afterwards — which is why the URL comes back rather than a bare "sent".
func Submit(ctx context.Context, token string, r Report) (string, error) {
	if strings.TrimSpace(token) == "" {
		return "", fmt.Errorf("feedback: no github token")
	}
	if strings.TrimSpace(r.Body) == "" {
		return "", fmt.Errorf("feedback: nothing written")
	}

	payload, err := json.Marshal(map[string]any{
		"title":  r.Title(),
		"body":   r.Issue(),
		"labels": []string{r.Category},
	})
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://api.github.com/repos/"+repo+"/issues", bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("Content-Type", "application/json")

	resp, err := (&http.Client{Timeout: 20 * time.Second}).Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		// The message matters here more than most: a token without issue
		// permission and a repository with issues turned off both fail, and
		// they need different things done about them.
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return "", fmt.Errorf("github API %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var out struct {
		HTMLURL string `json:"html_url"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&out); err != nil {
		return "", err
	}
	return out.HTMLURL, nil
}
