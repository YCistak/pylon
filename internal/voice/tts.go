package voice

import (
	"context"
	"fmt"
	"os"
	"strings"
)

// Speaker renders text to speech and plays it.
type Speaker interface {
	Say(ctx context.Context, text string) error
}

// cmdSpeaker synthesizes via a configurable command (text on stdin → WAV at the
// "{file}" placeholder) and plays the result with the configured player. The
// engine is thus pluggable: piper, an XTTS server client (curl), espeak, etc.
type cmdSpeaker struct {
	ttsCmd  []string
	playCmd []string
	run     runFunc
	tmpWav  func() (string, func(), error)
}

// NewSpeaker builds a Speaker from a synthesis command template. playCmd
// defaults per-OS when empty. An empty ttsCmd makes Say a no-op error.
func NewSpeaker(ttsCmd, playCmd []string) Speaker {
	if len(playCmd) == 0 {
		playCmd = defaultPlayCmd()
	}
	return &cmdSpeaker{
		ttsCmd:  ttsCmd,
		playCmd: playCmd,
		run:     execRun,
		tmpWav:  tempWav,
	}
}

func (s *cmdSpeaker) Say(ctx context.Context, text string) error {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	if len(s.ttsCmd) == 0 {
		return fmt.Errorf("voice: tts_cmd not configured")
	}

	wav, cleanup, err := s.tmpWav()
	if err != nil {
		return err
	}
	defer cleanup()

	// Synthesize: text on stdin, output WAV at {file}.
	if _, err := s.run(ctx, []byte(text), s.ttsCmd[0], substituteArgs(s.ttsCmd[1:], wav, 0, 0, 0)); err != nil {
		return err
	}
	_, err = s.run(ctx, nil, s.playCmd[0], substituteArgs(s.playCmd[1:], wav, 0, 0, 0))
	return err
}

// tempWav creates a temp .wav path and a cleanup func.
func tempWav() (string, func(), error) {
	f, err := os.CreateTemp("", "pylon-tts-*.wav")
	if err != nil {
		return "", func() {}, err
	}
	name := f.Name()
	f.Close()
	return name, func() { os.Remove(name) }, nil
}
