package intent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// defaultGeminiBaseURL is the Google Generative Language REST endpoint root.
const defaultGeminiBaseURL = "https://generativelanguage.googleapis.com/v1beta"

// Engine is the Gemini-backed fallback used when the local Router cannot resolve
// a transcript. It returns the same Command vocabulary the Router uses, plus an
// ActionChat for conversational replies.
type Engine struct {
	apiKey  string
	model   string
	baseURL string
	http    *http.Client
}

// EngineOptions configures an Engine.
type EngineOptions struct {
	APIKey  string
	Model   string        // e.g. gemini-flash-lite
	BaseURL string        // override for tests; defaults to the public endpoint
	Timeout time.Duration // per-request timeout (default 15s)
	HTTP    *http.Client  // injected client for tests
}

// ActionChat marks a conversational (non-command) turn; the reply text is in
// Args["reply"], styled to mirror the user.
const ActionChat Action = "chat"

// NewEngine builds an Engine. A Parse call fails fast if APIKey is empty.
func NewEngine(opts EngineOptions) *Engine {
	if opts.BaseURL == "" {
		opts.BaseURL = defaultGeminiBaseURL
	}
	if opts.Model == "" {
		opts.Model = "gemini-flash-lite"
	}
	return &Engine{
		apiKey:  opts.APIKey,
		model:   opts.Model,
		baseURL: strings.TrimRight(opts.BaseURL, "/"),
		http:    httpClientOr(opts.HTTP, opts.Timeout),
	}
}

// Configured reports whether the engine has an API key and can be called.
func (e *Engine) Configured() bool { return e != nil && e.apiKey != "" }

// Name identifies this parser in logs, e.g. "gemini:gemini-flash-lite-latest".
func (e *Engine) Name() string { return "gemini:" + e.model }

// Parse sends transcript to Gemini and returns a structured Command. styleCard
// is the persona style hint (may be empty) injected into the system prompt so
// conversational replies mirror the user.
func (e *Engine) Parse(ctx context.Context, transcript, styleCard string) (Command, error) {
	if !e.Configured() {
		return Command{}, fmt.Errorf("gemini engine not configured (missing API key)")
	}

	reqBody := geminiRequest{
		SystemInstruction: &geminiContent{Parts: []geminiPart{{Text: systemPrompt(styleCard)}}},
		Contents:          []geminiContent{{Role: "user", Parts: []geminiPart{{Text: transcript}}}},
		GenerationConfig: geminiGenConfig{
			ResponseMIMEType: "application/json",
			ResponseSchema:   commandSchema(),
			Temperature:      ptr(0.2),
		},
	}
	payload, err := json.Marshal(reqBody)
	if err != nil {
		return Command{}, err
	}

	url := fmt.Sprintf("%s/models/%s:generateContent", e.baseURL, e.model)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return Command{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-goog-api-key", e.apiKey)

	resp, err := e.http.Do(httpReq)
	if err != nil {
		return Command{}, fmt.Errorf("gemini request: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))

	if resp.StatusCode != http.StatusOK {
		return Command{}, &apiError{Provider: "gemini", Status: resp.StatusCode, Msg: geminiErrorMessage(body)}
	}
	return parseGeminiResponse(body)
}

// parseGeminiResponse extracts the model's JSON command from the API envelope.
func parseGeminiResponse(body []byte) (Command, error) {
	var gr geminiResponse
	if err := json.Unmarshal(body, &gr); err != nil {
		return Command{}, fmt.Errorf("decode gemini envelope: %w", err)
	}
	if len(gr.Candidates) == 0 || len(gr.Candidates[0].Content.Parts) == 0 {
		return Command{}, fmt.Errorf("gemini returned no candidates")
	}

	cmd, err := decodeCommandJSON([]byte(gr.Candidates[0].Content.Parts[0].Text))
	if err != nil {
		return Command{}, fmt.Errorf("gemini: %w", err)
	}
	return cmd, nil
}

func ptr[T any](v T) *T { return &v }
