package intent

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// mockGemini returns an httptest server that replies with the given command JSON
// wrapped in the Gemini envelope, and captures the last request body.
func mockGemini(t *testing.T, status int, commandJSON string, captured *geminiRequest) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if captured != nil {
			body, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(body, captured)
		}
		if r.Header.Get("x-goog-api-key") == "" {
			t.Errorf("missing api key header")
		}
		w.WriteHeader(status)
		if status != http.StatusOK {
			io.WriteString(w, `{"error":{"message":"boom","status":"INVALID_ARGUMENT"}}`)
			return
		}
		// Envelope: candidates[0].content.parts[0].text = the model's JSON.
		resp := map[string]any{
			"candidates": []map[string]any{
				{"content": map[string]any{"parts": []map[string]any{{"text": commandJSON}}}},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
}

func newTestEngine(t *testing.T, srv *httptest.Server) *Engine {
	t.Helper()
	return NewEngine(EngineOptions{
		APIKey:  "test-key",
		Model:   "gemini-flash-lite",
		BaseURL: srv.URL,
		HTTP:    srv.Client(),
	})
}

func TestEngineParsesRemindCommand(t *testing.T) {
	var captured geminiRequest
	srv := mockGemini(t, 200,
		`{"action":"task.remind_on_exit","process":"vscode","content":"hocaya yaz"}`, &captured)
	defer srv.Close()

	cmd, err := newTestEngine(t, srv).Parse(context.Background(), "vscode kapanınca hocaya yaz", "")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if cmd.Action != ActionRemindOnExit {
		t.Fatalf("action = %q", cmd.Action)
	}
	if cmd.arg("process") != "code" { // alias normalization applied
		t.Fatalf("process = %q, want code", cmd.arg("process"))
	}
	if cmd.arg("content") != "hocaya yaz" {
		t.Fatalf("content = %q", cmd.arg("content"))
	}
	// User transcript must be sent as content, not merged into instructions.
	if len(captured.Contents) == 0 || captured.Contents[0].Parts[0].Text != "vscode kapanınca hocaya yaz" {
		t.Fatalf("transcript not sent as user content: %+v", captured.Contents)
	}
}

func TestEngineStripsSpeechTail(t *testing.T) {
	srv := mockGemini(t, 200,
		`{"action":"task.remind_on_exit","process":"discord","content":"anneni ara de","reply":""}`, nil)
	defer srv.Close()

	cmd, err := newTestEngine(t, srv).Parse(context.Background(), "discord kapanınca anneni ara de", "")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if cmd.arg("content") != "anneni ara" {
		t.Fatalf("content = %q, want %q", cmd.arg("content"), "anneni ara")
	}
}

func TestEngineChatReply(t *testing.T) {
	srv := mockGemini(t, 200, `{"action":"chat","reply":"iyiyim kanka, sen?"}`, nil)
	defer srv.Close()

	cmd, err := newTestEngine(t, srv).Parse(context.Background(), "naber", "User says 'kanka'; casual.")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if cmd.Action != ActionChat || cmd.arg("reply") != "iyiyim kanka, sen?" {
		t.Fatalf("unexpected chat command: %+v", cmd)
	}
}

func TestEngineStyleCardInjected(t *testing.T) {
	var captured geminiRequest
	srv := mockGemini(t, 200, `{"action":"chat","reply":"selam"}`, &captured)
	defer srv.Close()

	_, err := newTestEngine(t, srv).Parse(context.Background(), "selam", "Addresses you as 'kanka'.")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	sys := captured.SystemInstruction.Parts[0].Text
	if !strings.Contains(sys, "kanka") {
		t.Fatalf("style card not in system prompt: %q", sys)
	}
	if !strings.Contains(sys, "SECURITY") {
		t.Fatalf("injection guard missing from system prompt")
	}
}

func TestEngineAPIError(t *testing.T) {
	srv := mockGemini(t, 400, "", nil)
	defer srv.Close()
	_, err := newTestEngine(t, srv).Parse(context.Background(), "x", "")
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("expected API error surfaced, got %v", err)
	}
}

func TestEngineNotConfigured(t *testing.T) {
	e := NewEngine(EngineOptions{}) // no key
	if e.Configured() {
		t.Fatal("engine without key should not be configured")
	}
	if _, err := e.Parse(context.Background(), "x", ""); err == nil {
		t.Fatal("expected error when not configured")
	}
}
