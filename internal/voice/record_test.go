package voice

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The silence-stop capture must carry both placeholders: {silence} ends the
// turn when the speaker goes quiet, {seconds} caps a runaway recording (and is
// what marks the command self-terminating, so Record returns early).
func TestSoxSilenceRecordCmd(t *testing.T) {
	cmd := soxSilenceRecordCmd("rec")
	if cmd[0] != "rec" {
		t.Fatalf("binary = %q", cmd[0])
	}
	js := strings.Join(cmd, " ")
	for _, want := range []string{"silence", "{silence}", "{threshold}", "trim 0 {seconds}", "{file}", "-r 16000", "-c 1"} {
		if !strings.Contains(js, want) {
			t.Fatalf("cmd missing %q: %v", want, cmd)
		}
	}
	if !commandUsesSeconds(cmd) {
		t.Fatal("silence capture must look self-terminating, else Record waits out the window")
	}
}

// Device flags from the per-OS file must land before sox's own options.
func TestSoxSilenceRecordCmdWithDeviceFlag(t *testing.T) {
	cmd := soxSilenceRecordCmd("sox", "-d")
	if cmd[0] != "sox" || cmd[1] != "-d" {
		t.Fatalf("prefix = %v", cmd[:2])
	}
}

func TestRecordArgsSubstitutes(t *testing.T) {
	cmd := soxSilenceRecordCmd("rec")
	args := recordArgs(cmd, "/tmp/r.wav", 15, 1.0, 3.0)
	js := strings.Join(args, " ")
	if strings.Contains(js, "{") {
		t.Fatalf("unsubstituted placeholder: %v", args)
	}
	for _, want := range []string{"/tmp/r.wav", "trim 0 15", "silence 1 0.1 3.00% 1 1.00 3.00%"} {
		if !strings.Contains(js, want) {
			t.Fatalf("args missing %q: %v", want, args)
		}
	}
}

// A configured record_cmd always wins: silence-stop must not silently replace
// what the user asked for.
func TestNewRecorderKeepsConfiguredCmd(t *testing.T) {
	want := []string{"arecord", "-d", "{seconds}", "{file}"}
	r := NewRecorder(RecorderOptions{Cmd: want, Seconds: 10, SilenceStop: true, SilenceSeconds: 1.0}).(*cmdRecorder)
	if strings.Join(r.cmd, " ") != strings.Join(want, " ") {
		t.Fatalf("configured cmd replaced: %v", r.cmd)
	}
}

func TestNewRecorderSilenceStopOffUsesDefault(t *testing.T) {
	r := NewRecorder(RecorderOptions{Seconds: 10, SilenceStop: false, SilenceSeconds: 1.0}).(*cmdRecorder)
	if strings.Join(r.cmd, " ") != strings.Join(defaultRecordCmd(), " ") {
		t.Fatalf("expected the plain default, got %v", r.cmd)
	}
}

func TestNewRecorderDefaultsSilenceSeconds(t *testing.T) {
	r := NewRecorder(RecorderOptions{Seconds: 10}).(*cmdRecorder)
	if r.silence != defaultSilenceSeconds {
		t.Fatalf("silence = %v", r.silence)
	}
}

// pickRecordCmd falls back to the plain recorder when the silence-capable
// binary is not installed, so a missing sox slows voice down instead of
// breaking it.
func TestPickRecordCmdFallsBackWhenBinaryMissing(t *testing.T) {
	t.Setenv("PATH", t.TempDir()) // nothing resolvable
	got := pickRecordCmd(true)
	if strings.Join(got, " ") != strings.Join(defaultRecordCmd(), " ") {
		t.Fatalf("expected fallback to the default recorder, got %v", got)
	}
}

// Only a silence-triggered capture may wait for speech to begin; a fixed-window
// recorder must keep the old, tighter deadline.
func TestTimeoutAddsSpeechStartGraceOnlyForSilence(t *testing.T) {
	r := &cmdRecorder{seconds: 5}
	if got := r.timeout(true, true); got != 5*time.Second+speechStartGrace+recordGrace {
		t.Fatalf("silence capture timeout = %v", got)
	}
	if got := r.timeout(false, false); got != 5*time.Second {
		t.Fatalf("fixed-window timeout = %v", got)
	}
}

// The Pipeline hands Record an empty temp file. Anything at or below a bare WAV
// header means the recorder captured nothing.
func TestHasAudio(t *testing.T) {
	dir := t.TempDir()
	empty := filepath.Join(dir, "empty.wav")
	if err := os.WriteFile(empty, make([]byte, wavHeaderBytes), 0o600); err != nil {
		t.Fatal(err)
	}
	if hasAudio(empty) {
		t.Fatal("a header-only file holds no samples")
	}

	withSamples := filepath.Join(dir, "full.wav")
	if err := os.WriteFile(withSamples, make([]byte, wavHeaderBytes+2), 0o600); err != nil {
		t.Fatal(err)
	}
	if !hasAudio(withSamples) {
		t.Fatal("samples past the header should count as audio")
	}
	if hasAudio(filepath.Join(dir, "yok.wav")) {
		t.Fatal("a missing file is not audio")
	}
}

// Nothing said is a normal outcome, so callers can tell it apart from a broken
// recorder and say "ses algılanamadı" instead of surfacing a subprocess error.
func TestNoSpeechIsRecognisable(t *testing.T) {
	if !IsNoSpeech(fmt.Errorf("record: %w", errNoSpeech)) {
		t.Fatal("wrapped errNoSpeech should still be recognisable")
	}
	if IsNoSpeech(errors.New("rec: device busy")) {
		t.Fatal("an ordinary failure must not look like silence")
	}
}
