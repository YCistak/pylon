package voice

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
	"time"
)

// recordGrace is how long past the capture window a self-terminating recorder is
// allowed before it's force-stopped.
const recordGrace = 3 * time.Second

// defaultSilenceSeconds is how much quiet ends a capture when silence-stop is
// on but no value was configured.
const defaultSilenceSeconds = 1.0

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
	silence float64
}

// NewRecorder builds a Recorder from a command template and a capture ceiling in
// seconds. An empty cmd picks a per-OS default: with silenceStop the capture
// ends as soon as the speaker goes quiet for silenceSecs, which is what makes a
// push-to-talk turn as short as the sentence rather than always `seconds` long.
// A configured cmd is always honoured as-is.
func NewRecorder(cmd []string, seconds int, silenceStop bool, silenceSecs float64) Recorder {
	if silenceSecs <= 0 {
		silenceSecs = defaultSilenceSeconds
	}
	if len(cmd) == 0 {
		cmd = pickRecordCmd(silenceStop)
	}
	if seconds <= 0 {
		seconds = 5
	}
	return &cmdRecorder{cmd: cmd, seconds: seconds, silence: silenceSecs}
}

// pickRecordCmd prefers the silence-terminated capture, falling back to the
// plain per-OS default when its binary is not installed — a missing sox should
// slow voice down, not break it.
func pickRecordCmd(silenceStop bool) []string {
	if !silenceStop {
		return defaultRecordCmd()
	}
	cmd := silenceRecordCmd()
	if len(cmd) == 0 {
		return defaultRecordCmd()
	}
	if _, err := exec.LookPath(cmd[0]); err != nil {
		log.Printf("voice: %s bulunamadı, sabit süreli kayda düşüldü (susunca-dur kapalı)", cmd[0])
		return defaultRecordCmd()
	}
	return cmd
}

// soxSilenceRecordCmd builds a sox capture that starts on speech and stops after
// {silence} seconds of quiet, with `trim 0 {seconds}` as the ceiling. The
// per-OS files supply the binary and any device flags.
func soxSilenceRecordCmd(bin string, pre ...string) []string {
	cmd := append([]string{bin}, pre...)
	return append(cmd,
		"-q", "-r", "16000", "-c", "1", "-b", "16", "{file}",
		// "1 0.1 1%": begin once audio exceeds 1% for 0.1s — swallows the click
		// of the hotkey. "1 {silence} 2%": stop after {silence} below 2%.
		"silence", "1", "0.1", "1%", "1", "{silence}", "2%",
		"trim", "0", "{seconds}",
	)
}

func (r *cmdRecorder) Record(ctx context.Context, wavPath string) error {
	args := recordArgs(r.cmd, wavPath, r.seconds, r.silence)
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
func recordArgs(cmd []string, wav string, seconds int, silence float64) []string {
	return substituteArgs(cmd[1:], wav, seconds, silence)
}

func commandUsesSeconds(cmd []string) bool {
	for _, a := range cmd {
		if strings.Contains(a, "{seconds}") {
			return true
		}
	}
	return false
}
