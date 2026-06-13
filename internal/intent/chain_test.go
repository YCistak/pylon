package intent

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
)

// fakeParser is a scripted Parser for chain tests.
type fakeParser struct {
	name string
	cmd  Command
	err  error
	hits *int // incremented on each Parse call
}

func (f *fakeParser) Name() string { return f.name }
func (f *fakeParser) Parse(context.Context, string, string) (Command, error) {
	if f.hits != nil {
		*f.hits++
	}
	return f.cmd, f.err
}

func quietChain(parsers ...Parser) *Chain {
	return NewChain(parsers, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func TestChainFallsThroughOnQuota(t *testing.T) {
	var firstHits, secondHits int
	first := &fakeParser{name: "p1", err: &apiError{Provider: "p1", Status: 429, Msg: "quota"}, hits: &firstHits}
	second := &fakeParser{name: "p2", cmd: Command{Action: ActionChat, Args: map[string]string{"reply": "hi"}}, hits: &secondHits}

	cmd, src, err := quietChain(first, second).Parse(context.Background(), "selam", "")
	if err != nil {
		t.Fatalf("expected fallthrough success, got %v", err)
	}
	if src != "p2" || cmd.Action != ActionChat {
		t.Fatalf("unexpected result src=%q cmd=%+v", src, cmd)
	}
	if firstHits != 1 || secondHits != 1 {
		t.Fatalf("expected both tried once, got %d/%d", firstHits, secondHits)
	}
}

func TestChainStopsOnNonRetryable(t *testing.T) {
	var secondHits int
	first := &fakeParser{name: "p1", err: &apiError{Provider: "p1", Status: 400, Msg: "bad"}}
	second := &fakeParser{name: "p2", cmd: Command{Action: ActionChat}, hits: &secondHits}

	_, _, err := quietChain(first, second).Parse(context.Background(), "x", "")
	if err == nil {
		t.Fatal("expected non-retryable error to surface")
	}
	if secondHits != 0 {
		t.Fatalf("second parser should not be tried, hits=%d", secondHits)
	}
}

func TestChainReturnsFirstSuccess(t *testing.T) {
	var secondHits int
	first := &fakeParser{name: "p1", cmd: Command{Action: ActionMute}}
	second := &fakeParser{name: "p2", hits: &secondHits}

	_, src, err := quietChain(first, second).Parse(context.Background(), "sustur", "")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if src != "p1" || secondHits != 0 {
		t.Fatalf("expected first to win, src=%q secondHits=%d", src, secondHits)
	}
}

func TestChainLastRetryableSurfaces(t *testing.T) {
	// Quota on the only/last parser has nowhere to fall through to.
	first := &fakeParser{name: "p1", err: &apiError{Provider: "p1", Status: 503, Msg: "down"}}
	_, _, err := quietChain(first).Parse(context.Background(), "x", "")
	var ae *apiError
	if !errors.As(err, &ae) || ae.Status != 503 {
		t.Fatalf("expected 503 apiError surfaced, got %v", err)
	}
}

func TestUnconfiguredChainErrors(t *testing.T) {
	c := quietChain()
	if c.Configured() {
		t.Fatal("empty chain should not be configured")
	}
	if _, _, err := c.Parse(context.Background(), "x", ""); err == nil {
		t.Fatal("empty chain Parse should error")
	}
}
