//go:build darwin

package voice

// macOS defaults: sox (`brew install sox`) captures a fixed-length 16 kHz mono
// WAV; afplay (built-in) plays it. sox self-terminates via `trim 0 {seconds}`,
// so no signal handling is needed.
func defaultRecordCmd() []string {
	return []string{"sox", "-d", "-r", "16000", "-c", "1", "-b", "16", "{file}", "trim", "0", "{seconds}"}
}

// silenceRecordCmd is the same sox capture, ended by silence instead of by the
// full window. `-d` is sox's default input device, so no separate `rec` needed.
func silenceRecordCmd() []string {
	return soxSilenceRecordCmd("sox", "-d")
}

func defaultPlayCmd() []string {
	return []string{"afplay", "{file}"}
}
