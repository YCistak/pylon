package github

import (
	"context"
	"fmt"
	"sync"
)

// Poller backs the 15-minute background poll (Phase 2.2). It remembers which
// review-requested PRs it has already announced so each one is spoken about
// only once — the first poll reports the current outstanding requests, then
// later polls report only newly-appeared ones.
type Poller struct {
	gh   *GitHub
	mu   sync.Mutex
	seen map[string]struct{} // "owner/repo#num"
}

// NewPoller returns a Poller bound to this service's auth/client.
func (g *GitHub) NewPoller() *Poller {
	return &Poller{gh: g, seen: map[string]struct{}{}}
}

// Poll fetches the PRs awaiting the user's review and returns a spoken message
// about the ones not seen before. ok is false when there is nothing new to say.
func (p *Poller) Poll(ctx context.Context) (msg string, ok bool, err error) {
	api, err := p.gh.client()
	if err != nil {
		return "", false, err
	}
	_, items, err := api.search(ctx, "is:open is:pr review-requested:@me archived:false")
	if err != nil {
		return "", false, err
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	var fresh []Item
	for _, it := range items {
		key := fmt.Sprintf("%s#%d", it.Repo, it.Number)
		if _, seen := p.seen[key]; seen {
			continue
		}
		p.seen[key] = struct{}{}
		fresh = append(fresh, it)
	}
	if len(fresh) == 0 {
		return "", false, nil
	}
	return fmt.Sprintf("GitHub: incelemen istenen %d yeni PR: %s.", len(fresh), summarize(fresh)), true, nil
}
