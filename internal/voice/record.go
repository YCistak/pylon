package voice

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// recordGrace is how long past the capture window a self-terminating recorder is
// allowed before it's force-stopped.
const recordGrace = 3 * time.Second

// Recorder captures microphone audio into a WAV file.
type Recorder interface {
	Record(ctx context.Context, wavPath string) error
}

// cmdRecorder runs a configurable capture command. Recorders that accept a
// duration use the "{seconds}" placeholder and self-terminate; those that don't
// (e.g. pw-record) are stopped with SIGINT after the window so the WAV header is
// finalized rather than truncated by a SIGKILL.
type cmdRecorder struct {
	cmd     []string
	seconds int
}

// NewRecorder builds a Recorder from a command template (per-OS default when
// empty) and capture window in seconds.
func NewRecorder(cmd []string, seconds int) Recorder {
	if len(cmd) == 0 {
		cmd = defaultRecordCmd()
	}
	if seconds <= 0 {
		seconds = 5
	}
	return &cmdRecorder{cmd: cmd, seconds: seconds}
}

func (r *cmdRecorder) Record(ctx context.Context, wavPath string) error {
	args := recordArgs(r.cmd, wavPath, r.seconds)
	selfStops := commandUsesSeconds(r.cmd)

	window := time.Duration(r.seconds) * time.Second
	timeout := window
	if selfStops {
		timeout += recordGrace // let the recorder exit on its own first
	}
	rctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(rctx, r.cmd[0], args...)
	// On timeout, interrupt (not kill) so the recorder flushes the WAV header.
	cmd.Cancel = func() error { return cmd.Process.Signal(os.Interrupt) }
	cmd.WaitDelay = 2 * time.Second
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	err := cmd.Run()
	// A signal-stopped recorder reports an error even though the capture is
	// valid; treat hitting the window as success.
	if !selfStops && rctx.Err() == context.DeadlineExceeded {
		return nil
	}
	if err != nil {
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return fmt.Errorf("%s: %w: %s", r.cmd[0], err, msg)
		}
		return fmt.Errorf("%s: %w", r.cmd[0], err)
	}
	return nil
}

// recordArgs substitutes placeholders into the recorder's argument list.
func recordArgs(cmd []string, wav string, seconds int) []string {
	return substituteArgs(cmd[1:], wav, seconds)
}

func commandUsesSeconds(cmd []string) bool {
	for _, a := range cmd {
		if strings.Contains(a, "{seconds}") {
			return true
		}
	}
	return false
}
