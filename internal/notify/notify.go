// Package notify shows desktop notifications through a configurable command,
// the same engine-agnostic pattern the voice package uses for TTS. The default
// is `notify-send` on Linux (dunst/mako/GNOME) and `osascript` on macOS; any
// command works via the "{title}" and "{body}" placeholders. This lets Pylon
// surface things like the morning briefing on screen, not just spoken.
package notify

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
)

// Notifier posts a desktop notification.
type Notifier interface {
	Notify(ctx context.Context, title, body string) error
}

// runFunc executes a command with extra environment variables; a fake
// replaces it in tests.
type runFunc func(ctx context.Context, name string, args, extraEnv []string) error

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
	// title/body are also exported as PYLON_TITLE/PYLON_BODY so a command can
	// read them out of band (e.g. the macOS osascript default via `system
	// attribute`) instead of interpolating untrusted content — calendar/PR
	// titles, etc. — into a script string, which would be an injection sink.
	args := substitute(n.cmd[1:], title, body)
	env := []string{"PYLON_TITLE=" + title, "PYLON_BODY=" + body}
	return n.run(ctx, n.cmd[0], args, env)
}

// substitute replaces the {title}/{body} placeholders inside each arg. Safe
// for templates that pass the values as separate argv entries (notify-send)
// since there is no shell. Templates that would otherwise embed the values in
// a scripted string should instead read PYLON_TITLE/PYLON_BODY from the
// environment (see defaults_darwin.go).
func substitute(tmpl []string, title, body string) []string {
	out := make([]string, len(tmpl))
	r := strings.NewReplacer("{title}", title, "{body}", body)
	for i, a := range tmpl {
		out[i] = r.Replace(a)
	}
	return out
}

func execRun(ctx context.Context, name string, args, extraEnv []string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = append(os.Environ(), extraEnv...)
	return cmd.Run()
}
