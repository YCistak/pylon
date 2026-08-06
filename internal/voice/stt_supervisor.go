package voice

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"
)

const (
	// sttReadyTimeout bounds the wait for the server to accept connections. The
	// model load is fast once warm, but the very first start on a GPU backend
	// also compiles shaders, which can take several seconds.
	sttReadyTimeout = 90 * time.Second
	sttReadyPoll    = 200 * time.Millisecond

	// sttWarmUpTimeout bounds the throwaway first inference (see warmUp).
	sttWarmUpTimeout = 60 * time.Second

	// sttRestartDelay is the backoff before restarting a server that died, and
	// sttMaxRestarts caps the attempts so a permanently broken binary does not
	// spin forever — transcription falls back to the CLI either way.
	sttRestartDelay = 5 * time.Second
	sttMaxRestarts  = 3
)

// ServerOptions describes the warm STT server the daemon supervises.
type ServerOptions struct {
	Bin       string // whisper.cpp server binary
	Addr      string // "host:port"
	Model     string // ggml model path
	Language  string // "auto" or an ISO code
	ExtraArgs []string
}

// RunSTTServer starts the STT server and keeps it running until ctx is done,
// restarting it a few times if it dies. It is meant to be handed to
// daemon.Register, so the server's lifetime is exactly the daemon's.
func RunSTTServer(ctx context.Context, o ServerOptions) error {
	if o.Bin == "" {
		return errors.New("voice: stt_server.bin not configured")
	}
	if o.Model == "" {
		return errors.New("voice: stt_model not configured")
	}

	for attempt := 0; ; attempt++ {
		err := runSTTServerOnce(ctx, o)
		if ctx.Err() != nil {
			return nil // normal shutdown
		}
		if attempt >= sttMaxRestarts {
			return fmt.Errorf("stt server kept dying, giving up: %w", err)
		}
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(sttRestartDelay):
		}
	}
}

// runSTTServerOnce runs the server until it exits or ctx is cancelled. On
// cancellation the process gets SIGINT so it shuts down cleanly.
func runSTTServerOnce(ctx context.Context, o ServerOptions) error {
	args, err := sttServerArgs(o)
	if err != nil {
		return err
	}

	cmd := exec.CommandContext(ctx, o.Bin, args...)
	cmd.Cancel = func() error { return cmd.Process.Signal(os.Interrupt) }
	cmd.WaitDelay = 5 * time.Second
	// Tail stderr so a startup failure (missing model, busy port) is reportable
	// without keeping the whole log in memory.
	var errTail tailBuffer
	cmd.Stderr = &errTail

	killOnParentDeath(cmd)

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("%s: %w", o.Bin, err)
	}

	// whisper.cpp binds the port only after the model is loaded, so an accepted
	// connection means the server is genuinely ready to transcribe.
	if err := waitForListener(ctx, o.Addr, sttReadyTimeout); err != nil {
		_ = cmd.Process.Signal(os.Interrupt)
		_ = cmd.Wait()
		return fmt.Errorf("stt server not ready: %w: %s", err, errTail.String())
	}
	warmUp(ctx, o.Addr)

	if err := cmd.Wait(); err != nil && ctx.Err() == nil {
		return fmt.Errorf("%s: %w: %s", o.Bin, err, errTail.String())
	}
	return nil
}

// warmUp sends one throwaway clip so the first *spoken* turn is fast. Listening
// on the port only means the weights are loaded; the backend still builds its
// compute graph on the first inference, which measured ~5 s on Vulkan. Failures
// are ignored — this is an optimisation, not a requirement.
func warmUp(ctx context.Context, addr string) {
	ctx, cancel := context.WithTimeout(ctx, sttWarmUpTimeout)
	defer cancel()

	f, err := os.CreateTemp("", "pylon-warmup-*.wav")
	if err != nil {
		return
	}
	defer os.Remove(f.Name())
	_, err = f.Write(silentWAV(1))
	f.Close()
	if err != nil {
		return
	}

	st := &serverTranscriber{addr: addr, lang: "auto", client: &http.Client{Timeout: sttWarmUpTimeout}}
	if _, err := st.post(ctx, f.Name()); err != nil {
		log.Printf("voice: STT sunucusu ısıtılamadı: %v", err)
	}
}

// silentWAV builds `seconds` of 16 kHz mono silence as a WAV, so warming up
// needs no sample file on disk.
func silentWAV(seconds int) []byte {
	const (
		sampleRate = 16000
		channels   = 1
		bits       = 16
	)
	dataLen := sampleRate * channels * bits / 8 * seconds

	var b bytes.Buffer
	b.WriteString("RIFF")
	binary.Write(&b, binary.LittleEndian, uint32(36+dataLen))
	b.WriteString("WAVEfmt ")
	binary.Write(&b, binary.LittleEndian, uint32(16))              // PCM header size
	binary.Write(&b, binary.LittleEndian, uint16(1))               // PCM
	binary.Write(&b, binary.LittleEndian, uint16(channels))        //
	binary.Write(&b, binary.LittleEndian, uint32(sampleRate))      //
	binary.Write(&b, binary.LittleEndian, uint32(sampleRate*2))    // byte rate
	binary.Write(&b, binary.LittleEndian, uint16(channels*bits/8)) // block align
	binary.Write(&b, binary.LittleEndian, uint16(bits))            //
	b.WriteString("data")
	binary.Write(&b, binary.LittleEndian, uint32(dataLen))
	b.Write(make([]byte, dataLen))
	return b.Bytes()
}

// sttServerArgs builds the whisper.cpp server command line.
func sttServerArgs(o ServerOptions) ([]string, error) {
	host, port, err := net.SplitHostPort(o.Addr)
	if err != nil {
		return nil, fmt.Errorf("stt server addr %q: %w", o.Addr, err)
	}
	lang := o.Language
	if lang == "" {
		lang = "auto"
	}
	args := []string{
		"-m", o.Model,
		"--host", host,
		"--port", port,
		"-l", lang,
		"-nt", // no timestamps: the transcript is all we consume
	}
	return append(args, o.ExtraArgs...), nil
}

// waitForListener polls until something accepts on addr, ctx is done, or the
// timeout elapses.
func waitForListener(ctx context.Context, addr string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		conn, err := net.DialTimeout("tcp", addr, sttReadyPoll)
		if err == nil {
			conn.Close()
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("no listener on %s after %s", addr, timeout)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(sttReadyPoll):
		}
	}
}

// tailBuffer keeps only the last few KB written to it, so a chatty server does
// not grow the daemon's memory for the whole session.
type tailBuffer struct {
	buf bytes.Buffer
}

const tailBufferMax = 4096

func (t *tailBuffer) Write(p []byte) (int, error) {
	n := len(p)
	t.buf.Write(p)
	if t.buf.Len() > tailBufferMax {
		b := t.buf.Bytes()
		keep := append([]byte(nil), b[t.buf.Len()-tailBufferMax:]...)
		t.buf.Reset()
		t.buf.Write(keep)
	}
	return n, nil
}

func (t *tailBuffer) String() string { return strings.TrimSpace(t.buf.String()) }
