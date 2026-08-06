// Package voice is Pylon's speech I/O: speech-to-text (whisper.cpp) and
// text-to-speech (piper), plus microphone capture and playback. Everything runs
// as a subprocess so the daemon stays CGo-free and cross-compiles; the audio
// commands default per-OS and are overridable via config. The package exposes a
// Pipeline (record → transcribe → ... → speak) driven by `pylon listen`.
package voice

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// runFunc runs name+args with optional stdin and returns stdout. It is a field
// on each component so tests can inject a fake without spawning processes.
type runFunc func(ctx context.Context, stdin []byte, name string, args []string) ([]byte, error)

// execRun is the production runFunc: it shells out and captures stdout, folding
// stderr into the error so failures are diagnosable.
func execRun(ctx context.Context, stdin []byte, name string, args []string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	if stdin != nil {
		cmd.Stdin = bytes.NewReader(stdin)
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg != "" {
			return stdout.Bytes(), fmt.Errorf("%s: %w: %s", name, err, msg)
		}
		return stdout.Bytes(), fmt.Errorf("%s: %w", name, err)
	}
	return stdout.Bytes(), nil
}

// substituteArgs replaces the "{file}", "{seconds}", "{silence}" and
// "{threshold}" placeholders in a command template, returning a fresh slice.
func substituteArgs(tmpl []string, file string, seconds int, silence, threshold float64) []string {
	rep := strings.NewReplacer(
		"{file}", file,
		"{seconds}", fmt.Sprint(seconds),
		// Always keep a decimal point: sox reads a bare integer as a *sample
		// count*, so "1" would end the capture after one sample and clip the
		// last word, while "1.00" means one second.
		"{silence}", strconv.FormatFloat(silence, 'f', 2, 64),
		"{threshold}", strconv.FormatFloat(threshold, 'f', 2, 64)+"%",
	)
	out := make([]string, len(tmpl))
	for i, a := range tmpl {
		out[i] = rep.Replace(a)
	}
	return out
}
