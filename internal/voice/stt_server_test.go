package voice

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// tempWavFile writes a throwaway file to upload; the fake server never decodes
// it.
func tempWavFile(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "rec.wav")
	if err := os.WriteFile(path, []byte("RIFFfake"), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// serverAddr strips the scheme from an httptest URL, since NewServerTranscriber
// takes a bare host:port.
func serverAddr(t *testing.T, srv *httptest.Server) string {
	t.Helper()
	return strings.TrimPrefix(srv.URL, "http://")
}

func TestServerTranscribe(t *testing.T) {
	var gotPath, gotLang, gotFormat, gotFilename string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Errorf("parse multipart: %v", err)
		}
		gotLang = r.FormValue("language")
		gotFormat = r.FormValue("response_format")
		if fh := r.MultipartForm.File["file"]; len(fh) == 1 {
			gotFilename = fh[0].Filename
		}
		w.Write([]byte("  hava nasıl  \n"))
	}))
	defer srv.Close()

	st := NewServerTranscriber(serverAddr(t, srv), "tr", nil)
	got, err := st.Transcribe(context.Background(), tempWavFile(t))
	if err != nil {
		t.Fatalf("transcribe: %v", err)
	}
	if got != "hava nasıl" {
		t.Fatalf("text = %q", got)
	}
	if gotPath != "/inference" {
		t.Fatalf("path = %q", gotPath)
	}
	if gotLang != "tr" || gotFormat != "text" {
		t.Fatalf("fields: language=%q response_format=%q", gotLang, gotFormat)
	}
	if gotFilename != "rec.wav" {
		t.Fatalf("filename = %q", gotFilename)
	}
}

func TestServerTranscribeFallsBackOnError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()

	st := NewServerTranscriber(serverAddr(t, srv), "tr", fakeSTT{text: "yedekten geldi"})
	got, err := st.Transcribe(context.Background(), tempWavFile(t))
	if err != nil {
		t.Fatalf("transcribe: %v", err)
	}
	if got != "yedekten geldi" {
		t.Fatalf("expected CLI fallback, got %q", got)
	}
}

// A server that is not listening at all must also fall back, not fail the turn.
func TestServerTranscribeFallsBackWhenDown(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	addr := serverAddr(t, srv)
	srv.Close() // nothing listens on addr any more

	st := NewServerTranscriber(addr, "tr", fakeSTT{text: "yedek"})
	got, err := st.Transcribe(context.Background(), tempWavFile(t))
	if err != nil {
		t.Fatalf("transcribe: %v", err)
	}
	if got != "yedek" {
		t.Fatalf("expected fallback, got %q", got)
	}
}

// Without a fallback the error must surface rather than be swallowed.
func TestServerTranscribeNoFallbackReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()

	st := NewServerTranscriber(serverAddr(t, srv), "tr", nil)
	if _, err := st.Transcribe(context.Background(), tempWavFile(t)); err == nil {
		t.Fatal("expected an error without a fallback")
	}
}

func TestSTTServerArgs(t *testing.T) {
	args, err := sttServerArgs(ServerOptions{
		Addr:      "127.0.0.1:8910",
		Model:     "m.bin",
		Language:  "tr",
		ExtraArgs: []string{"-t", "8"},
	})
	if err != nil {
		t.Fatalf("args: %v", err)
	}
	js := strings.Join(args, " ")
	for _, want := range []string{"-m m.bin", "--host 127.0.0.1", "--port 8910", "-l tr", "-nt", "-t 8"} {
		if !strings.Contains(js, want) {
			t.Fatalf("args missing %q: %v", want, args)
		}
	}
}

func TestSTTServerArgsRejectsBadAddr(t *testing.T) {
	if _, err := sttServerArgs(ServerOptions{Addr: "nonsense", Model: "m.bin"}); err == nil {
		t.Fatal("expected an error for an addr without a port")
	}
}

func TestRunSTTServerRequiresConfig(t *testing.T) {
	if err := RunSTTServer(context.Background(), ServerOptions{Model: "m.bin"}); err == nil {
		t.Fatal("expected an error without a binary")
	}
	if err := RunSTTServer(context.Background(), ServerOptions{Bin: "whisper-server"}); err == nil {
		t.Fatal("expected an error without a model")
	}
}

func TestWaitForListenerTimesOut(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	addr := serverAddr(t, srv)
	srv.Close()

	err := waitForListener(context.Background(), addr, 300*sttReadyPoll/100)
	if err == nil {
		t.Fatal("expected a timeout with nothing listening")
	}
	if errors.Is(err, context.Canceled) {
		t.Fatalf("unexpected cancellation: %v", err)
	}
}

func TestTailBufferKeepsTail(t *testing.T) {
	var tb tailBuffer
	tb.Write([]byte(strings.Repeat("a", tailBufferMax)))
	tb.Write([]byte("SON"))
	got := tb.String()
	if len(got) > tailBufferMax {
		t.Fatalf("tail grew to %d bytes", len(got))
	}
	if !strings.HasSuffix(got, "SON") {
		t.Fatalf("tail lost the newest bytes: %q", got[len(got)-10:])
	}
}

// warmUp must send a decodable WAV — a malformed header would make the server
// reject it and leave the first real turn slow.
func TestSilentWAVHeader(t *testing.T) {
	b := silentWAV(1)
	if string(b[0:4]) != "RIFF" || string(b[8:12]) != "WAVE" {
		t.Fatalf("not a RIFF/WAVE file: %q", b[:12])
	}
	// 44-byte canonical header + 1 s of 16 kHz mono 16-bit PCM.
	if len(b) != 44+16000*2 {
		t.Fatalf("size = %d", len(b))
	}
}

func TestWarmUpPostsToServer(t *testing.T) {
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		if r.URL.Path != "/inference" {
			t.Errorf("path = %q", r.URL.Path)
		}
		w.Write([]byte(""))
	}))
	defer srv.Close()

	warmUp(context.Background(), serverAddr(t, srv))
	if hits != 1 {
		t.Fatalf("warmUp made %d requests, want 1", hits)
	}
}

// A server that refuses the warm-up must not block startup.
func TestWarmUpIgnoresFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "nope", http.StatusInternalServerError)
	}))
	addr := serverAddr(t, srv)
	srv.Close()

	done := make(chan struct{})
	go func() { warmUp(context.Background(), addr); close(done) }()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("warmUp blocked on an unreachable server")
	}
}
