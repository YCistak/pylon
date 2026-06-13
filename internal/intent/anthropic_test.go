package intent

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestAnthropicParsesToolUse(t *testing.T) {
	var gotKey, gotVer, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotKey = r.Header.Get("x-api-key")
		gotVer = r.Header.Get("anthropic-version")
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		// Messages envelope: a tool_use block carries the structured input.
		io.WriteString(w, `{"content":[{"type":"tool_use","name":"emit_command","input":{"action":"media.mute","process":"","content":"","reply":""}}]}`)
	}))
	defer srv.Close()

	p := newAnthropic(ProviderSpec{APIKey: "ak-test", Model: "claude-haiku-4-5", BaseURL: srv.URL}, time.Second, srv.Client())
	cmd, err := p.Parse(context.Background(), "sustur", "")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if cmd.Action != ActionMute {
		t.Fatalf("action = %q", cmd.Action)
	}
	if gotKey != "ak-test" || gotVer != anthropicVersion {
		t.Fatalf("headers key=%q ver=%q", gotKey, gotVer)
	}
	if !strings.Contains(gotBody, `"tool_choice"`) || !strings.Contains(gotBody, commandToolName) {
		t.Fatalf("request missing forced tool call: %s", gotBody)
	}
}

func TestAnthropicSurfacesServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		io.WriteString(w, `{"error":{"message":"overloaded"}}`)
	}))
	defer srv.Close()

	p := newAnthropic(ProviderSpec{APIKey: "k", Model: "m", BaseURL: srv.URL}, time.Second, srv.Client())
	_, err := p.Parse(context.Background(), "x", "")
	if !retryable(err) {
		t.Fatalf("500 should be retryable, got %v", err)
	}
}

func TestNewParserDispatch(t *testing.T) {
	cases := map[string]string{"gemini": "gemini:m", "openai": "openai:m", "anthropic": "anthropic:m"}
	for provider, want := range cases {
		p, err := NewParser(ProviderSpec{Provider: provider, Model: "m", APIKey: "k"}, time.Second)
		if err != nil {
			t.Fatalf("%s: %v", provider, err)
		}
		if p.Name() != want {
			t.Fatalf("%s name = %q, want %q", provider, p.Name(), want)
		}
	}
	if _, err := NewParser(ProviderSpec{Provider: "bogus", Model: "m", APIKey: "k"}, time.Second); err == nil {
		t.Fatal("unknown provider should error")
	}
	if _, err := NewParser(ProviderSpec{Provider: "gemini", Model: "m"}, time.Second); err == nil {
		t.Fatal("missing API key should error")
	}
}
