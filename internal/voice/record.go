package voice

import (
	"bytes"
	"context"
	"errors"
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

// defaultSilenceThreshold is the share of full scale below which audio counts as
// quiet. Measured room noise on a desk mic sat around 1% peak, so 3% clears the
// noise floor while ordinary speech runs far above it. Quiet rooms or hot mics
// can tune this with voice.silence_threshold.
const defaultSilenceThreshold = 3.0

// speechStartGrace is how long a silence-triggered capture waits for the
// speaker to start. record_seconds caps how long they may *talk*; without this
// grace, pausing to collect your thoughts would abort the turn.
const speechStartGrace = 15 * time.Second

// wavHeaderBytes is the canonical PCM header size; a file no larger than this
// holds no samples.
const wavHeaderBytes = 44

// errNoSpeech reports a capture that timed out before the speaker said
// anything — a normal outcome worth a plain message, not a subprocess error.
var errNoSpeech = errors.New("konuşma algılanmadı")

// Recorder captures microphone audio into a WAV file.
type Recorder interface {
	Record(ctx context.Context, wavPath string) error
}

// cmdRecorder runs a configurable capture command. Recorders that accept a
// duration use the "{seconds}" placeholder and self-terminate; those that don't
// (e.g. pw-record) are stopped with SIGINT after the window so the WAV header is
// finalized rather than truncated by a SIGKILL.
type cmdRecorder struct {
	cmd       []string
	seconds   int
	silence   float64
	threshold float64
}

// RecorderOptions configures a Recorder.
type RecorderOptions struct {
	Cmd     []string // empty → per-OS default
	Seconds int      // capture ceiling
	// SilenceStop ends the capture once the speaker goes quiet, which is what
	// makes a turn as short as the sentence rather than always Seconds long.
	SilenceStop      bool
	SilenceSeconds   float64 // quiet time that ends a capture
	SilenceThreshold float64 // percent of full scale counted as quiet
}

// NewRecorder builds a Recorder. A configured Cmd is always honoured as-is; only
// an empty one picks a silence-aware per-OS default.
func NewRecorder(o RecorderOptions) Recorder {
	if o.SilenceSeconds <= 0 {
		o.SilenceSeconds = defaultSilenceSeconds
	}
	if o.SilenceThreshold <= 0 {
		o.SilenceThreshold = defaultSilenceThreshold
	}
	if len(o.Cmd) == 0 {
		o.Cmd = pickRecordCmd(o.SilenceStop)
	}
	if o.Seconds <= 0 {
		o.Seconds = 5
	}
	return &cmdRecorder{
		cmd:       o.Cmd,
		seconds:   o.Seconds,
		silence:   o.SilenceSeconds,
		threshold: o.SilenceThreshold,
	}
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
		// Begin once audio stays above {threshold} for 0.1s — that ignores the
		// click of the hotkey and the room's noise floor — then stop after
		// {silence} back below it.
		"silence", "1", "0.1", "{threshold}", "1", "{silence}", "{threshold}",
		"trim", "0", "{seconds}",
	)
}

func (r *cmdRecorder) Record(ctx context.Context, wavPath string) error {
	args := recordArgs(r.cmd, wavPath, r.seconds, r.silence, r.threshold)
	selfStops := commandUsesSeconds(r.cmd)
	waitsForSpeech := commandUsesSilence(r.cmd)

	rctx, cancel := context.WithTimeout(ctx, r.timeout(selfStops, waitsForSpeech))
	defer cancel()

	cmd := exec.CommandContext(rctx, r.cmd[0], args...)
	// On timeout, interrupt (not kill) so the recorder flushes the WAV header.
	cmd.Cancel = func() error { return cmd.Process.Signal(os.Interrupt) }
	cmd.WaitDelay = 2 * time.Second
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	err := cmd.Run()
	// A recorder stopped by our signal reports an error even though the capture
	// is valid, so hitting the deadline is only a failure when nothing landed in
	// the file — which for a silence-triggered capture means the speaker never
	// started talking.
	if rctx.Err() == context.DeadlineExceeded {
		if hasAudio(wavPath) {
			return nil
		}
		if waitsForSpeech {
			return errNoSpeech
		}
		return fmt.Errorf("%s: kayıt boş kaldı", r.cmd[0])
	}
	if err != nil {
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return fmt.Errorf("%s: %w: %s", r.cmd[0], err, msg)
		}
		return fmt.Errorf("%s: %w", r.cmd[0], err)
	}
	return nil
}

// timeout is how long the recorder is allowed to run. A silence-triggered
// capture also has to wait for the speaker to *begin*, which is unbounded from
// the recorder's point of view, so it gets speechStartGrace on top of the
// ceiling — otherwise a moment's hesitation aborts the turn.
func (r *cmdRecorder) timeout(selfStops, waitsForSpeech bool) time.Duration {
	d := time.Duration(r.seconds) * time.Second
	if waitsForSpeech {
		d += speechStartGrace
	}
	if selfStops {
		d += recordGrace // let the recorder exit on its own first
	}
	return d
}

// hasAudio reports whether the recorder actually captured samples, rather than
// leaving the empty temp file the Pipeline created.
func hasAudio(wavPath string) bool {
	fi, err := os.Stat(wavPath)
	return err == nil && fi.Size() > wavHeaderBytes
}

// recordArgs substitutes placeholders into the recorder's argument list.
func recordArgs(cmd []string, wav string, seconds int, silence, threshold float64) []string {
	return substituteArgs(cmd[1:], wav, seconds, silence, threshold)
}

func commandUsesSeconds(cmd []string) bool {
	return commandUsesPlaceholder(cmd, "{seconds}")
}

func commandUsesSilence(cmd []string) bool {
	return commandUsesPlaceholder(cmd, "{silence}")
}

func commandUsesPlaceholder(cmd []string, placeholder string) bool {
	for _, a := range cmd {
		if strings.Contains(a, placeholder) {
			return true
		}
	}
	return false
}
