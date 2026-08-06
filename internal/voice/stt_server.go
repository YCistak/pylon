package voice

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// serverRequestTimeout bounds a single transcription. The model is already
// resident, so a turn that takes longer than this means the server is wedged
// and we are better off falling back to the CLI.
const serverRequestTimeout = 30 * time.Second

// serverTranscriber posts the recording to a warm whisper.cpp server, which
// keeps the model in memory — that removes the per-turn model load (measured
// ~610 ms with large-v3-turbo). fallback keeps voice working when the server is
// down: a dead server should cost latency, not the feature.
type serverTranscriber struct {
	addr     string
	lang     string
	fallback Transcriber
	client   *http.Client

	warnOnce sync.Once
}

// NewServerTranscriber builds a Transcriber that talks to a whisper.cpp server
// at addr ("host:port"), falling back to fallback when it cannot be reached.
func NewServerTranscriber(addr, lang string, fallback Transcriber) Transcriber {
	if lang == "" {
		lang = "auto"
	}
	return &serverTranscriber{
		addr:     addr,
		lang:     lang,
		fallback: fallback,
		client:   &http.Client{Timeout: serverRequestTimeout},
	}
}

func (s *serverTranscriber) Transcribe(ctx context.Context, wavPath string) (string, error) {
	text, err := s.post(ctx, wavPath)
	if err == nil {
		return text, nil
	}
	if s.fallback == nil {
		return "", err
	}
	// Log the reason once: a server that dies mid-session would otherwise
	// produce one line per utterance.
	s.warnOnce.Do(func() {
		log.Printf("voice: STT sunucusuna ulaşılamadı (%v), whisper-cli'ye düşülüyor", err)
	})
	return s.fallback.Transcribe(ctx, wavPath)
}

// post uploads the WAV to /inference as multipart form data and returns the
// transcript. whisper.cpp's server accepts the same knobs as the CLI.
func (s *serverTranscriber) post(ctx context.Context, wavPath string) (string, error) {
	wav, err := os.ReadFile(wavPath)
	if err != nil {
		return "", err
	}

	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	part, err := mw.CreateFormFile("file", filepath.Base(wavPath))
	if err != nil {
		return "", err
	}
	if _, err := part.Write(wav); err != nil {
		return "", err
	}
	for k, v := range map[string]string{
		"language":        s.lang,
		"response_format": "text",
		"temperature":     "0.0",
	} {
		if err := mw.WriteField(k, v); err != nil {
			return "", err
		}
	}
	if err := mw.Close(); err != nil {
		return "", err
	}

	url := "http://" + s.addr + "/inference"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, &body)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())

	resp, err := s.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	out, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("stt server: %s: %s", resp.Status, truncate(string(out), 200))
	}
	return parseWhisperOutput(out), nil
}

// truncate keeps an error message from dumping a whole HTML error page into the
// log.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
