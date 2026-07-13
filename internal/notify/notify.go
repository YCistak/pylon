// Package notify shows desktop notifications through a configurable command,
// the same engine-agnostic pattern the voice package uses for TTS. The default
// is `notify-send` on Linux (dunst/mako/GNOME) and `osascript` on macOS; any
// command works via the "{title}" and "{body}" placeholders. This lets Pylon
// surface things like the morning briefing on screen, not just spoken.
package notify

import (
	"context"
	"errors"
	"os/exec"
	"strings"
)

// Notifier posts a desktop notification.
type Notifier interface {
	Notify(ctx context.Context, title, body string) error
}

// runFunc executes a command; a fake replaces it in tests.
type runFunc func(ctx context.Context, name string, args []string) error

// cmdNotifier posts via a configurable command template. Empty cmd → the per-OS
// default; a nil/absent command makes Notify a no-op error.
type cmdNotifier struct {
	cmd []string
	run runFunc
}

// New builds a Notifier from a command template. Empty cmd uses the per-OS
// default (see defaults_*.go).
func New(cmd []string) Notifier {
	if len(cmd) == 0 {
		cmd = defaultNotifyCmd()
	}
	return &cmdNotifier{cmd: cmd, run: execRun}
}

func (n *cmdNotifier) Notify(ctx context.Context, title, body string) error {
	if len(n.cmd) == 0 {
		return errors.New("notify: no notify command (unsupported OS or empty config)")
	}
	args := substitute(n.cmd[1:], title, body)
	return n.run(ctx, n.cmd[0], args)
}

// substitute replaces the {title}/{body} placeholders inside each arg, so a
// template can put them in separate argv entries (notify-send) or inside one
// scripted string (osascript).
func substitute(tmpl []string, title, body string) []string {
	out := make([]string, len(tmpl))
	r := strings.NewReplacer("{title}", title, "{body}", body)
	for i, a := range tmpl {
		out[i] = r.Replace(a)
	}
	return out
}

func execRun(ctx context.Context, name string, args []string) error {
	return exec.CommandContext(ctx, name, args...).Run()
}
