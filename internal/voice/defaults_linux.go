//go:build linux

package voice

// Linux defaults target PipeWire (pw-record/pw-play), the modern default on
// Arch/CachyOS. pw-record captures until interrupted; the Recorder sends SIGINT
// after the capture window so the WAV header is finalized. Override in config
// for PulseAudio (parecord/paplay) or ALSA (arecord -d {seconds} / aplay).
func defaultRecordCmd() []string {
	return []string{"pw-record", "--channels=1", "--rate=16000", "--format=s16", "{file}"}
}

// silenceRecordCmd ends the capture once you stop talking. pw-record has no
// such mode, so this uses sox's `rec` (package `sox`); when it is missing the
// Recorder falls back to defaultRecordCmd and the fixed window.
func silenceRecordCmd() []string {
	return soxSilenceRecordCmd("rec")
}

func defaultPlayCmd() []string {
	return []string{"pw-play", "{file}"}
}
