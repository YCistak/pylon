package voice

import (
	"context"
	"fmt"
	"os"
)

// Options configures a Pipeline. main.go maps config.Voice onto it, keeping this
// package independent of the config package.
type Options struct {
	STTBin   string
	STTModel string
	Language string

	TTSCmd []string // synthesis command: text on stdin → WAV at "{file}"

	RecordCmd     []string
	RecordSeconds int
	PlayCmd       []string
}

// Pipeline is the voice loop split into its two halves: Capture (mic → text) and
// Speak (text → audio). The caller drives the daemon round-trip in between, so
// the same Pipeline serves `pylon listen` without owning IPC.
type Pipeline struct {
	rec Recorder
	stt Transcriber
	tts Speaker

	tmpWav func() (string, func(), error)
}

// NewPipeline assembles the recorder, transcriber, and speaker from Options.
func NewPipeline(o Options) *Pipeline {
	return &Pipeline{
		rec:    NewRecorder(o.RecordCmd, o.RecordSeconds),
		stt:    NewTranscriber(o.STTBin, o.STTModel, o.Language),
		tts:    NewSpeaker(o.TTSCmd, o.PlayCmd),
		tmpWav: tempRecording,
	}
}

// Capture records a push-to-talk window and returns the transcript.
func (p *Pipeline) Capture(ctx context.Context) (string, error) {
	wav, cleanup, err := p.tmpWav()
	if err != nil {
		return "", err
	}
	defer cleanup()

	if err := p.rec.Record(ctx, wav); err != nil {
		return "", fmt.Errorf("record: %w", err)
	}
	text, err := p.stt.Transcribe(ctx, wav)
	if err != nil {
		return "", fmt.Errorf("transcribe: %w", err)
	}
	return text, nil
}

// Speak renders and plays a reply. Empty text is a no-op.
func (p *Pipeline) Speak(ctx context.Context, text string) error {
	return p.tts.Say(ctx, text)
}

// tempRecording creates a temp .wav path for a capture and a cleanup func.
func tempRecording() (string, func(), error) {
	f, err := os.CreateTemp("", "pylon-rec-*.wav")
	if err != nil {
		return "", func() {}, err
	}
	name := f.Name()
	f.Close()
	return name, func() { os.Remove(name) }, nil
}
