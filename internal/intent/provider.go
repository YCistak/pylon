package intent

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"
)

// Parser turns a transcript into a structured Command using a remote LLM. Each
// provider (Gemini, OpenAI, Anthropic) implements it; a Chain tries them in
// order, falling through on quota/rate-limit errors. styleCard is an optional
// persona hint injected into the system prompt so chat replies mirror the user.
type Parser interface {
	Parse(ctx context.Context, transcript, styleCard string) (Command, error)
	Name() string
}

// ProviderSpec is the resolved configuration for one Parser: the API key is
// already looked up from the environment by the caller, keeping this package
// independent of the config package.
type ProviderSpec struct {
	Provider string // "gemini" | "openai" | "anthropic"
	Model    string
	APIKey   string
	BaseURL  string // optional endpoint override (tests/proxies)
}

// NewParser builds the Parser for a single chain entry.
func NewParser(s ProviderSpec, timeout time.Duration) (Parser, error) {
	if s.APIKey == "" {
		return nil, fmt.Errorf("intent: %s/%s has no API key", s.Provider, s.Model)
	}
	switch s.Provider {
	case "gemini":
		return NewEngine(EngineOptions{APIKey: s.APIKey, Model: s.Model, BaseURL: s.BaseURL, Timeout: timeout}), nil
	case "openai":
		return newOpenAI(s, timeout, nil), nil
	case "anthropic":
		return newAnthropic(s, timeout, nil), nil
	default:
		return nil, fmt.Errorf("intent: unknown provider %q", s.Provider)
	}
}

// apiError carries the HTTP status from a provider call so the chain can tell a
// transient/quota failure (retry on the next model) from a fatal one.
type apiError struct {
	Provider string
	Status   int
	Msg      string
}

func (e *apiError) Error() string {
	return fmt.Sprintf("%s API %d: %s", e.Provider, e.Status, e.Msg)
}

// retryable reports whether err should trigger fallthrough to the next model:
// rate-limit/quota (429), server errors (5xx), and network timeouts.
func retryable(err error) bool {
	var ae *apiError
	if errors.As(err, &ae) {
		return ae.Status == http.StatusTooManyRequests || ae.Status >= 500
	}
	var ne net.Error
	if errors.As(err, &ne) {
		return ne.Timeout()
	}
	return false
}

// decodeCommandFields builds a Command from the four raw JSON fields every
// provider returns, applying the same normalization: process is reduced to its
// canonical executable token, content has trailing speech markers stripped.
func decodeCommandFields(action, process, content, reply string) Command {
	cmd := Command{Action: Action(action), Confidence: 1, Args: map[string]string{}}
	if process != "" {
		cmd.Args["process"] = canonicalProcess(firstToken(normalize(process)))
	}
	if c := trimSpeechTail(content); c != "" {
		cmd.Args["content"] = c
	}
	if reply != "" {
		cmd.Args["reply"] = reply
	}
	if len(cmd.Args) == 0 {
		cmd.Args = nil
	}
	return cmd
}

// jsonSchemaCommand is the command schema in standard JSON Schema (lowercase
// types, additionalProperties:false, all keys required) for providers that
// speak OpenAPI/JSON-Schema directly (OpenAI response_format, Anthropic tools).
// Gemini uses its own commandSchema() with upper-case OpenAPI types.
func jsonSchemaCommand() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"action", "process", "content", "reply"},
		"properties": map[string]any{
			"action":  map[string]any{"type": "string", "enum": allActions()},
			"process": map[string]any{"type": "string"},
			"content": map[string]any{"type": "string"},
			"reply":   map[string]any{"type": "string"},
		},
	}
}

// httpClientOr returns hc if set, otherwise a client with the given timeout
// (defaulting to 15s). Shared by the HTTP-based providers.
func httpClientOr(hc *http.Client, timeout time.Duration) *http.Client {
	if hc != nil {
		return hc
	}
	if timeout == 0 {
		timeout = 15 * time.Second
	}
	return &http.Client{Timeout: timeout}
}
