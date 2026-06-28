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

const defaultOpenAIBaseURL = "https://api.openai.com/v1"

// openaiProvider parses transcripts via the OpenAI Chat Completions API using
// structured outputs (response_format json_schema).
type openaiProvider struct {
	apiKey  string
	model   string
	baseURL string
	http    *http.Client
}

func newOpenAI(s ProviderSpec, timeout time.Duration, hc *http.Client) *openaiProvider {
	base := s.BaseURL
	if base == "" {
		base = defaultOpenAIBaseURL
	}
	return &openaiProvider{
		apiKey:  s.APIKey,
		model:   s.Model,
		baseURL: strings.TrimRight(base, "/"),
		http:    httpClientOr(hc, timeout),
	}
}

func (o *openaiProvider) Name() string { return "openai:" + o.model }

func (o *openaiProvider) Parse(ctx context.Context, transcript, styleCard string) (Command, error) {
	reqBody := map[string]any{
		"model":       o.model,
		"temperature": 0.2,
		"messages": []map[string]string{
			{"role": "system", "content": systemPrompt(styleCard)},
			{"role": "user", "content": transcript},
		},
		"response_format": map[string]any{
			"type": "json_schema",
			"json_schema": map[string]any{
				"name":   "pylon_command",
				"strict": true,
				"schema": jsonSchemaCommand(),
			},
		},
	}
	payload, err := json.Marshal(reqBody)
	if err != nil {
		return Command{}, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, o.baseURL+"/chat/completions", bytes.NewReader(payload))
	if err != nil {
		return Command{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+o.apiKey)

	resp, err := o.http.Do(req)
	if err != nil {
		return Command{}, fmt.Errorf("openai request: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))

	if resp.StatusCode != http.StatusOK {
		return Command{}, &apiError{Provider: "openai", Status: resp.StatusCode, Msg: openaiErrorMessage(body)}
	}

	var env struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		return Command{}, fmt.Errorf("decode openai envelope: %w", err)
	}
	if len(env.Choices) == 0 {
		return Command{}, fmt.Errorf("openai returned no choices")
	}
	return decodeCommandJSON([]byte(env.Choices[0].Message.Content))
}

// decodeCommandJSON parses a {action,process,content,reply} JSON object (as
// emitted by OpenAI/Anthropic structured output) into a Command.
func decodeCommandJSON(raw []byte) (Command, error) {
	var p struct {
		Action   string `json:"action"`
		Process  string `json:"process"`
		Content  string `json:"content"`
		Reply    string `json:"reply"`
		Datetime string `json:"datetime"`
	}
	if err := json.Unmarshal(raw, &p); err != nil {
		return Command{}, fmt.Errorf("decode command %q: %w", string(raw), err)
	}
	return decodeCommandFields(p.Action, p.Process, p.Content, p.Reply, p.Datetime), nil
}

func openaiErrorMessage(body []byte) string {
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
