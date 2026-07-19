// Package banner shows a short line of text as an on-screen desktop banner by
// handing it to a configured command. Pylon owns the text; the command owns the
// presentation — on Linux/Wayland that command is a layer-shell overlay
// (scripts/briefing_banner.py), but any program that reads stdin works, which
// keeps the daemon CGo-free and cross-platform (the OS-specific part lives
// entirely in the configured command, mirroring how voice.Speaker wraps TTS).
//
// The banner is a fire-and-forget GUI: Show spawns it and returns immediately,
// leaving it to dismiss itself. An empty command makes Show a silent no-op, so
// the banner stays opt-in via config.
package banner

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// runFunc launches name+args with stdin and returns without waiting for the
// process to finish. It is a field so tests can inject a fake.
type runFunc func(stdin []byte, name string, args []string) error

// Presenter shows text via a configured command.
type Presenter struct {
	cmd []string
	run runFunc
}

// NewPresenter builds a Presenter from a command template (e.g.
// ["python3", "scripts/briefing_banner.py"]). An empty command disables it.
func NewPresenter(cmd []string) *Presenter {
	return &Presenter{cmd: cmd, run: spawn}
}

// Show hands text to the banner command on stdin and returns at once — the
// banner lives and dismisses on its own. Empty text or an unconfigured command
// is a no-op. The context is accepted for call-site symmetry but not used to
// bound the banner's lifetime: a briefing that outlives a short request timeout
// must not be killed with it.
func (p *Presenter) Show(_ context.Context, text string) error {
	text = strings.TrimSpace(text)
	if text == "" || len(p.cmd) == 0 {
		return nil
	}
	return p.run([]byte(text), p.cmd[0], p.cmd[1:])
}

// spawn starts the command detached with stdin fed, then reaps it in the
// background so it never becomes a zombie. It does not block on the GUI.
func spawn(stdin []byte, name string, args []string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdin = bytes.NewReader(stdin)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("banner: %s: %w", name, err)
	}
	go func() { _ = cmd.Wait() }()
	return nil
}
