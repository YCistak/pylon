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

const (
	defaultAnthropicBaseURL = "https://api.anthropic.com"
	anthropicVersion        = "2023-06-01"
	anthropicMaxTokens      = 1024
	commandToolName         = "emit_command"
)

// anthropicProvider parses transcripts via the Anthropic Messages API, using a
// forced tool call as the structured-output channel.
type anthropicProvider struct {
	apiKey  string
	model   string
	baseURL string
	http    *http.Client
}

func newAnthropic(s ProviderSpec, timeout time.Duration, hc *http.Client) *anthropicProvider {
	base := s.BaseURL
	if base == "" {
		base = defaultAnthropicBaseURL
	}
	return &anthropicProvider{
		apiKey:  s.APIKey,
		model:   s.Model,
		baseURL: strings.TrimRight(base, "/"),
		http:    httpClientOr(hc, timeout),
	}
}

func (a *anthropicProvider) Name() string { return "anthropic:" + a.model }

func (a *anthropicProvider) Parse(ctx context.Context, transcript, styleCard string) (Command, error) {
	reqBody := map[string]any{
		"model":      a.model,
		"max_tokens": anthropicMaxTokens,
		"system":     systemPrompt(styleCard),
		"messages": []map[string]any{
			{"role": "user", "content": transcript},
		},
		"tools": []map[string]any{{
			"name":         commandToolName,
			"description":  "Emit the interpreted Pylon command.",
			"input_schema": jsonSchemaCommand(),
		}},
		"tool_choice": map[string]string{"type": "tool", "name": commandToolName},
	}
	payload, err := json.Marshal(reqBody)
	if err != nil {
		return Command{}, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.baseURL+"/v1/messages", bytes.NewReader(payload))
	if err != nil {
		return Command{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", a.apiKey)
	req.Header.Set("anthropic-version", anthropicVersion)

	resp, err := a.http.Do(req)
	if err != nil {
		return Command{}, fmt.Errorf("anthropic request: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))

	if resp.StatusCode != http.StatusOK {
		return Command{}, &apiError{Provider: "anthropic", Status: resp.StatusCode, Msg: anthropicErrorMessage(body)}
	}

	var env struct {
		Content []struct {
			Type  string          `json:"type"`
			Name  string          `json:"name"`
			Input json.RawMessage `json:"input"`
		} `json:"content"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		return Command{}, fmt.Errorf("decode anthropic envelope: %w", err)
	}
	for _, block := range env.Content {
		if block.Type == "tool_use" && block.Name == commandToolName {
			return decodeCommandJSON(block.Input)
		}
	}
	return Command{}, fmt.Errorf("anthropic returned no %s tool call", commandToolName)
}

func anthropicErrorMessage(body []byte) string {
	var e struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &e); err == nil && e.Error.Message != "" {
		return e.Error.Message
	}
	return strings.TrimSpace(string(body))
}
