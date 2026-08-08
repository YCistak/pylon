package intent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"
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

// decodeCommandArgs builds a Command from the flat object every provider
// returns: "action" names the action and every other key is one of its
// arguments. Two keys keep the normalization they have always had — process is
// reduced to its canonical executable token, content loses trailing speech
// markers. Everything else, including argument names contributed by services,
// is passed through verbatim.
func decodeCommandArgs(fields map[string]string) Command {
	cmd := Command{Confidence: 1, Args: map[string]string{}}
	for key, v := range fields {
		switch key {
		case "action":
			cmd.Action = Action(v)
		case "process":
			if v != "" {
				cmd.Args["process"] = canonicalProcess(firstToken(normalize(v)))
			}
		case "content":
			if c := trimSpeechTail(v); c != "" {
				cmd.Args["content"] = c
			}
		default:
			if v != "" {
				cmd.Args[key] = v
			}
		}
	}
	if len(cmd.Args) == 0 {
		cmd.Args = nil
	}
	return cmd
}

// decodeCommandJSON parses the flat JSON object a provider's structured output
// produces into a Command.
func decodeCommandJSON(raw []byte) (Command, error) {
	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err != nil {
		return Command{}, fmt.Errorf("decode command %q: %w", string(raw), err)
	}
	return decodeCommandArgs(stringFields(obj)), nil
}

// stringFields flattens a decoded JSON object into string values. The schema
// asks for strings throughout, but a model that answers `"lines": 50` should
// not cost the argument; objects and arrays have no place in a flat arg map and
// are dropped, as is an explicit null.
func stringFields(obj map[string]any) map[string]string {
	out := make(map[string]string, len(obj))
	for k, v := range obj {
		switch t := v.(type) {
		case string:
			out[k] = t
		case bool:
			out[k] = strconv.FormatBool(t)
		case float64:
			out[k] = strconv.FormatFloat(t, 'f', -1, 64)
		}
	}
	return out
}

// jsonSchemaCommand is the command schema in standard JSON Schema (lowercase
// types, additionalProperties:false, all keys required) for providers that
// speak OpenAPI/JSON-Schema directly (OpenAI response_format, Anthropic tools).
// Gemini uses its own commandSchema() with upper-case OpenAPI types.
//
// The argument fields come from the live catalog, so registering a service also
// gives its arguments somewhere to arrive.
func jsonSchemaCommand() map[string]any {
	fields := argFields()
	props := map[string]any{"action": map[string]any{"type": "string", "enum": allActions()}}
	required := make([]string, 0, len(fields)+1)
	required = append(required, "action")
	for _, f := range fields {
		props[f] = map[string]any{"type": "string"}
		required = append(required, f)
	}
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             required,
		"properties":           props,
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
