//go:build windows

package voice

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// scripts/tts.ps1 is Pylon's ready-made Windows TTS, and its whole value is
// that it runs on a stock Windows box with no install. That can only be
// verified on Windows, so this drives it end to end on the CI Windows runner:
// feed text on stdin, get a playable PCM WAV out.
func TestWindowsTTSScriptProducesWav(t *testing.T) {
	_, thisFile, _, _ := runtime.Caller(0)
	script := filepath.Join(filepath.Dir(thisFile), "..", "..", "scripts", "tts.ps1")
	if _, err := os.Stat(script); err != nil {
		t.Fatalf("tts.ps1 not found at %s: %v", script, err)
	}

	out := filepath.Join(t.TempDir(), "out.wav")
	// Windows PowerShell (5.1) ships System.Speech; pwsh 7 may not, so invoke
	// powershell explicitly — the same binary the script's tts_cmd names.
	cmd := exec.Command("powershell", "-NoProfile", "-ExecutionPolicy", "Bypass",
		"-File", script, out)
	cmd.Stdin = strings.NewReader("Pylon test.")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("tts.ps1 failed: %v\n%s", err, stderr.String())
	}

	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("no WAV written: %v", err)
	}
	if len(data) < 44 { // a WAV header alone is 44 bytes
		t.Fatalf("WAV is %d bytes, too small to contain audio", len(data))
	}
	// The RIFF/WAVE magic is what the player will look for; assert the script
	// produced a real WAV container, not just some bytes.
	if string(data[0:4]) != "RIFF" || string(data[8:12]) != "WAVE" {
		t.Fatalf("output is not a RIFF/WAVE file: % x", data[:12])
	}
	t.Logf("tts.ps1 produced a %d-byte WAV", len(data))
}
