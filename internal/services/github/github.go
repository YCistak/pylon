// Package github integrates GitHub into Pylon: on-demand pull-request and issue
// queries over the REST search API, authenticated with a Personal Access Token.
// Like the other services it speaks through a small interface so it can be
// faked in tests, and contributes its actions to the intent vocabulary.
package github

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/YCistak/pylon/internal/i18n"
	"github.com/YCistak/pylon/internal/intent"
)

// GitHub actions.
const (
	ActionListPRs    intent.Action = "github.list_prs"
	ActionListIssues intent.Action = "github.list_issues"
)

const defaultBaseURL = "https://api.github.com"

// Config holds GitHub settings. Auth is a Personal Access Token (classic with
// `repo`+`read:org`, or fine-grained with PR/issue read). The config loader
// expands ${ENV} so the token can live in the environment, not the file.
type Config struct {
	Token   string // Personal Access Token
	BaseURL string // override for tests / GitHub Enterprise; defaults to api.github.com
}

// Item is the minimal shape Pylon needs from a PR or issue.
type Item struct {
	Title  string
	Repo   string // owner/repo
	Number int
}

// ghAPI is the slice of the GitHub API the service uses; a fake implements it in
// tests. search runs a GitHub issue/PR search and returns the total match count
// (which can exceed len(items) when results are paginated) plus the first page.
type ghAPI interface {
	search(ctx context.Context, query string) (total int, items []Item, err error)
}

// GitHub is the GitHub Service.
type GitHub struct {
	cfg Config
	api ghAPI // injected in tests; otherwise built lazily from the token
}

// New builds the service from config. It does not touch the network until first
// use, so it can be registered as soon as a token is configured.
func New(cfg Config) *GitHub {
	if cfg.BaseURL == "" {
		cfg.BaseURL = defaultBaseURL
	}
	return &GitHub{cfg: cfg}
}

// Configured reports whether GitHub is ready to serve commands (a token is set).
// main.go registers the service only when true.
func Configured(cfg Config) bool { return strings.TrimSpace(cfg.Token) != "" }

func (g *GitHub) Name() string { return "github" }

func (g *GitHub) Actions() []intent.ActionSpec {
	return []intent.ActionSpec{
		{
			Name: ActionListPRs,
			Desc: `"github.list_prs": list open GitHub pull requests waiting on the user — ones where their review is requested, plus their own open PRs. No args. Use for "GitHub'da bekleyen PR var mı", "açık PR'larım var mı".`,
		},
		{
			Name: ActionListIssues,
			Desc: `"github.list_issues": list open GitHub issues assigned to the user. No args. Use for "bana atanmış issue var mı".`,
		},
	}
}

func (g *GitHub) Execute(ctx context.Context, action intent.Action, _ map[string]string) (string, error) {
	api, err := g.client()
	if err != nil {
		return "", err
	}
	switch action {
	case ActionListPRs:
		return g.listPRs(ctx, api)
	case ActionListIssues:
		return g.listIssues(ctx, api)
	default:
		return "", fmt.Errorf("github: unknown action %q", action)
	}
}

func (g *GitHub) listPRs(ctx context.Context, api ghAPI) (string, error) {
	reviewN, review, err := api.search(ctx, "is:open is:pr review-requested:@me archived:false")
	if err != nil {
		return "", err
	}
	mineN, mine, err := api.search(ctx, "is:open is:pr author:@me archived:false")
	if err != nil {
		return "", err
	}
	if reviewN == 0 && mineN == 0 {
		return "Bekleyen PR yok.", nil
	}
	var parts []string
	if reviewN > 0 {
		parts = append(parts, fmt.Sprintf("incelemen istenen %d PR (%s)", reviewN, summarize(review)))
	}
	if mineN > 0 {
		parts = append(parts, fmt.Sprintf("açık %d PR'ın (%s)", mineN, summarize(mine)))
	}
	return "GitHub: " + strings.Join(parts, "; ") + ".", nil
}

func (g *GitHub) listIssues(ctx context.Context, api ghAPI) (string, error) {
	n, items, err := api.search(ctx, "is:open is:issue assignee:@me archived:false")
	if err != nil {
		return "", err
	}
	if n == 0 {
		return i18n.T("github.no_issues"), nil
	}
	return i18n.N("github.issues", n, summarize(items)), nil
}

// summarize renders the first few items as "owner/repo#123 başlık", capping the
// list so the spoken reply stays short.
func summarize(items []Item) string {
	const max = 3
	var names []string
	for i, it := range items {
		if i >= max {
			names = append(names, "…")
			break
		}
		names = append(names, fmt.Sprintf("%s#%d %s", it.Repo, it.Number, it.Title))
	}
	return strings.Join(names, ", ")
}

// client lazily builds the real GitHub API client, unless one was injected (tests).
func (g *GitHub) client() (ghAPI, error) {
	if g.api != nil {
		return g.api, nil
	}
	if !Configured(g.cfg) {
		return nil, errors.New("github: no token configured (services.github.token)")
	}
	return &realGH{token: g.cfg.Token, base: g.cfg.BaseURL, hc: &http.Client{Timeout: 10 * time.Second}}, nil
}

// realGH calls the GitHub REST search API over plain HTTP.
type realGH struct {
	token string
	base  string
	hc    *http.Client
}

func (r *realGH) search(ctx context.Context, query string) (int, []Item, error) {
	u := r.base + "/search/issues?per_page=10&q=" + url.QueryEscape(query)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Authorization", "Bearer "+r.token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	resp, err := r.hc.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return 0, nil, fmt.Errorf("github API %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var out struct {
		TotalCount int `json:"total_count"`
		Items      []struct {
			Title         string `json:"title"`
			Number        int    `json:"number"`
			RepositoryURL string `json:"repository_url"`
		} `json:"items"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return 0, nil, err
	}
	items := make([]Item, 0, len(out.Items))
	for _, it := range out.Items {
		items = append(items, Item{Title: it.Title, Number: it.Number, Repo: repoFromURL(it.RepositoryURL)})
	}
	return out.TotalCount, items, nil
}

// repoFromURL extracts "owner/repo" from a repository API URL
// (https://api.github.com/repos/owner/repo).
func repoFromURL(u string) string {
	const marker = "/repos/"
	if i := strings.Index(u, marker); i >= 0 {
		return u[i+len(marker):]
	}
	return ""
}
