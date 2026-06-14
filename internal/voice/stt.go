package voice

import (
	"context"
	"fmt"
	"strings"
)

// Transcriber turns a recorded WAV file into text.
type Transcriber interface {
	Transcribe(ctx context.Context, wavPath string) (string, error)
}

// whisperTranscriber shells out to the whisper.cpp CLI. Language "auto" lets
// whisper detect the spoken language, so input in any language works.
type whisperTranscriber struct {
	bin   string
	model string
	lang  string
	run   runFunc
}

// NewTranscriber builds a whisper.cpp-backed Transcriber. lang may be "auto" or
// an ISO code (tr, en, ...).
func NewTranscriber(bin, model, lang string) Transcriber {
	if lang == "" {
		lang = "auto"
	}
	return &whisperTranscriber{bin: bin, model: model, lang: lang, run: execRun}
}

func (w *whisperTranscriber) Transcribe(ctx context.Context, wavPath string) (string, error) {
	if w.model == "" {
		return "", fmt.Errorf("voice: stt_model not configured")
	}
	out, err := w.run(ctx, nil, w.bin, whisperArgs(w.model, w.lang, wavPath))
	if err != nil {
		return "", err
	}
	return parseWhisperOutput(out), nil
}

// whisperArgs builds the CLI arguments: no timestamps (-nt) and no progress
// prints (-np) so stdout carries only the transcript.
func whisperArgs(model, lang, wav string) []string {
	return []string{"-m", model, "-l", lang, "-nt", "-np", "-f", wav}
}

// parseWhisperOutput collapses whisper's per-segment stdout lines into one
// trimmed transcript.
func parseWhisperOutput(out []byte) string {
	var parts []string
	for _, line := range strings.Split(string(out), "\n") {
		if s := strings.TrimSpace(line); s != "" {
			parts = append(parts, s)
		}
	}
	return strings.Join(parts, " ")
}
