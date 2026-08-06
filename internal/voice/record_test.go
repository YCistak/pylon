package voice

import (
	"strings"
	"testing"
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
	for _, want := range []string{"silence", "{silence}", "trim 0 {seconds}", "{file}", "-r 16000", "-c 1"} {
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
	args := recordArgs(cmd, "/tmp/r.wav", 15, 1.0)
	js := strings.Join(args, " ")
	if strings.Contains(js, "{") {
		t.Fatalf("unsubstituted placeholder: %v", args)
	}
	for _, want := range []string{"/tmp/r.wav", "trim 0 15", "silence 1 0.1 1% 1 1.00 2%"} {
		if !strings.Contains(js, want) {
			t.Fatalf("args missing %q: %v", want, args)
		}
	}
}

// A configured record_cmd always wins: silence-stop must not silently replace
// what the user asked for.
func TestNewRecorderKeepsConfiguredCmd(t *testing.T) {
	want := []string{"arecord", "-d", "{seconds}", "{file}"}
	r := NewRecorder(want, 10, true, 1.0).(*cmdRecorder)
	if strings.Join(r.cmd, " ") != strings.Join(want, " ") {
		t.Fatalf("configured cmd replaced: %v", r.cmd)
	}
}

func TestNewRecorderSilenceStopOffUsesDefault(t *testing.T) {
	r := NewRecorder(nil, 10, false, 1.0).(*cmdRecorder)
	if strings.Join(r.cmd, " ") != strings.Join(defaultRecordCmd(), " ") {
		t.Fatalf("expected the plain default, got %v", r.cmd)
	}
}

func TestNewRecorderDefaultsSilenceSeconds(t *testing.T) {
	r := NewRecorder(nil, 10, false, 0).(*cmdRecorder)
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
