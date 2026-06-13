package intent

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestOpenAIParsesCommand(t *testing.T) {
	var gotAuth, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		// Chat Completions envelope: choices[0].message.content holds the JSON.
		io.WriteString(w, `{"choices":[{"message":{"content":"{\"action\":\"task.remind_on_exit\",\"process\":\"steam\",\"content\":\"ödevini yap de\",\"reply\":\"\"}"}}]}`)
	}))
	defer srv.Close()

	p := newOpenAI(ProviderSpec{APIKey: "sk-test", Model: "gpt-4o-mini", BaseURL: srv.URL}, time.Second, srv.Client())
	cmd, err := p.Parse(context.Background(), "steam kapanınca ödevini yap", "")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if cmd.Action != ActionRemindOnExit || cmd.arg("process") != "steam" {
		t.Fatalf("unexpected command: %+v", cmd)
	}
	if cmd.arg("content") != "ödevini yap" { // speech tail "de" stripped
		t.Fatalf("content = %q", cmd.arg("content"))
	}
	if gotAuth != "Bearer sk-test" {
		t.Fatalf("auth header = %q", gotAuth)
	}
	if !strings.Contains(gotBody, `"json_schema"`) || !strings.Contains(gotBody, `"strict":true`) {
		t.Fatalf("request missing structured-output config: %s", gotBody)
	}
}

func TestOpenAISurfacesQuota(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		io.WriteString(w, `{"error":{"message":"rate limit"}}`)
	}))
	defer srv.Close()

	p := newOpenAI(ProviderSpec{APIKey: "k", Model: "m", BaseURL: srv.URL}, time.Second, srv.Client())
	_, err := p.Parse(context.Background(), "x", "")
	if !retryable(err) {
		t.Fatalf("429 should be retryable, got %v", err)
	}
}

func TestOpenAIName(t *testing.T) {
	p := newOpenAI(ProviderSpec{APIKey: "k", Model: "gpt-4o-mini"}, time.Second, nil)
	if p.Name() != "openai:gpt-4o-mini" {
		t.Fatalf("name = %q", p.Name())
	}
}

// Guard against the schema accidentally dropping a required key.
func TestJSONSchemaCommandRequiresAllFields(t *testing.T) {
	raw, _ := json.Marshal(jsonSchemaCommand())
	for _, k := range []string{"action", "process", "content", "reply"} {
		if !strings.Contains(string(raw), `"`+k+`"`) {
			t.Fatalf("schema missing %q: %s", k, raw)
		}
	}
}
