//go:build windows

package voice

// Windows defaults use ffmpeg (DirectShow) to capture a fixed-length 16 kHz mono
// WAV and PowerShell's SoundPlayer to play it. ffmpeg's `-t {seconds}` makes it
// self-terminate. Install ffmpeg and, if the default device name differs, set
// record_cmd in config (audio="<your device>").
func defaultRecordCmd() []string {
	return []string{"ffmpeg", "-y", "-f", "dshow", "-i", "audio=default", "-ar", "16000", "-ac", "1", "-t", "{seconds}", "{file}"}
}

// silenceRecordCmd ends the capture once you stop talking. ffmpeg cannot do
// that, so this needs sox on PATH (`winget install sox`); without it the
// Recorder falls back to defaultRecordCmd and the fixed window.
func silenceRecordCmd() []string {
	return soxSilenceRecordCmd("sox", "-d")
}

func defaultPlayCmd() []string {
	return []string{"powershell", "-NoProfile", "-Command", "(New-Object Media.SoundPlayer '{file}').PlaySync()"}
}
