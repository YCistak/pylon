//go:build windows

package voice

// Windows defaults use ffmpeg (DirectShow) to capture a fixed-length 16 kHz mono
// WAV and PowerShell's SoundPlayer to play it. ffmpeg's `-t {seconds}` makes it
// self-terminate. Install ffmpeg and, if the default device name differs, set
// record_cmd in config (audio="<your device>").
func defaultRecordCmd() []string {
	return []string{"ffmpeg", "-y", "-f", "dshow", "-i", "audio=default", "-ar", "16000", "-ac", "1", "-t", "{seconds}", "{file}"}
}

func defaultPlayCmd() []string {
	return []string{"powershell", "-NoProfile", "-Command", "(New-Object Media.SoundPlayer '{file}').PlaySync()"}
}
