package intent

import (
	"context"
	"fmt"
	"log/slog"
)

// Chain is an ordered list of Parsers tried in priority order. When a parser
// fails with a retryable error (quota/rate-limit 429, 5xx, timeout) the next is
// tried, so independent per-model quota buckets are used in turn. A non-retryable
// error (bad request, decode failure) stops the chain and is returned.
type Chain struct {
	parsers []Parser
	log     *slog.Logger
}

// NewChain builds a Chain from parsers already constructed in priority order.
func NewChain(parsers []Parser, log *slog.Logger) *Chain {
	if log == nil {
		log = slog.Default()
	}
	return &Chain{parsers: parsers, log: log}
}

// Configured reports whether the chain has at least one usable parser.
func (c *Chain) Configured() bool { return c != nil && len(c.parsers) > 0 }

// Parse tries each parser in order. The model that produced the command is
// returned so the caller can attribute the source.
func (c *Chain) Parse(ctx context.Context, transcript, styleCard string) (Command, string, error) {
	if !c.Configured() {
		return Command{}, "", fmt.Errorf("intent: no models configured")
	}
	var lastErr error
	for i, p := range c.parsers {
		cmd, err := p.Parse(ctx, transcript, styleCard)
		if err == nil {
			return cmd, p.Name(), nil
		}
		lastErr = err
		if retryable(err) && i < len(c.parsers)-1 {
			c.log.Warn("intent: model fell through", "model", p.Name(), "err", err)
			continue
		}
		return Command{}, p.Name(), err
	}
	return Command{}, "", lastErr
}
