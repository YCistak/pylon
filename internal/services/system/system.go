// Package system executes local machine-control actions: lock the screen,
// change the volume, control the active media player, and close an app. These
// are the media/lock intents the local router already recognizes (they used to
// resolve but do nothing) plus a new "close X". Commands are Linux-first
// (loginctl / pactl / playerctl / pkill); other OSes are not wired yet and
// return a graceful message. It needs no configuration, so it is always
// registered and owns these actions end to end (declaration + execution).
package system

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

	"github.com/YCistak/pylon/internal/i18n"
	"github.com/YCistak/pylon/internal/intent"
)

// ActionClose force-quits an application by name. The lock/media/volume actions
// reuse the intent package's existing constants so the local router's matches
// dispatch straight here.
const ActionClose intent.Action = "system.close"

// runner runs one external command; a fake implements it in tests.
type runner interface {
	run(ctx context.Context, name string, args ...string) error
}

// System is the machine-control Service.
type System struct {
	run runner
}

// New builds the service backed by real command execution.
func New() *System { return &System{run: execRunner{}} }

func (s *System) Name() string { return "system" }

func (s *System) Actions() []intent.ActionSpec {
	return []intent.ActionSpec{
		{Name: intent.ActionLockScreen},
		{Name: intent.ActionMediaPlay},
		{Name: intent.ActionMediaPause},
		{Name: intent.ActionMediaNext},
		{Name: intent.ActionMediaPrev},
		{Name: intent.ActionVolumeUp},
		{Name: intent.ActionVolumeDown},
		{Name: intent.ActionMute},
		{
			Name: ActionClose,
			Args: []string{"app"},
			Desc: `"system.close": force-quit a running application. "app" is its process/command name as a single lowercase word (code, chrome, spotify, discord, steam). Use for "X'i kapat", "şunu kapat", "close X".`,
		},
	}
}

func (s *System) Execute(ctx context.Context, action intent.Action, args map[string]string) (string, error) {
	switch action {
	case intent.ActionLockScreen:
		// Try systemd-logind first, then the freedesktop screensaver.
		if s.tryFirst(ctx,
			cmd{"loginctl", []string{"lock-session"}},
			cmd{"xdg-screensaver", []string{"lock"}},
		) {
			return i18n.T("system.lock.ok"), nil
		}
		return i18n.T("system.lock.fail"), nil

	case intent.ActionVolumeUp:
		// Unmute as well, so "volume up" both wakes and raises the output.
		_ = s.run.run(ctx, "pactl", "set-sink-mute", "@DEFAULT_SINK@", "0")
		if err := s.run.run(ctx, "pactl", "set-sink-volume", "@DEFAULT_SINK@", "+10%"); err != nil {
			return i18n.T("system.volume.fail"), nil
		}
		return i18n.T("system.volume.up"), nil

	case intent.ActionVolumeDown:
		if err := s.run.run(ctx, "pactl", "set-sink-volume", "@DEFAULT_SINK@", "-10%"); err != nil {
			return i18n.T("system.volume.fail"), nil
		}
		return i18n.T("system.volume.down"), nil

	case intent.ActionMute:
		if err := s.run.run(ctx, "pactl", "set-sink-mute", "@DEFAULT_SINK@", "1"); err != nil {
			return i18n.T("system.volume.fail"), nil
		}
		return i18n.T("system.volume.muted"), nil

	case intent.ActionMediaPlay:
		return s.player(ctx, "play", i18n.T("system.media.play"))
	case intent.ActionMediaPause:
		return s.player(ctx, "pause", i18n.T("system.media.pause"))
	case intent.ActionMediaNext:
		return s.player(ctx, "next", i18n.T("system.media.next"))
	case intent.ActionMediaPrev:
		return s.player(ctx, "previous", i18n.T("system.media.prev"))

	case ActionClose:
		app := strings.TrimSpace(args["app"])
		if app == "" {
			return i18n.T("system.close.which"), nil
		}
		// Never let Pylon be asked to kill itself.
		if isSelf(app) {
			return i18n.T("system.close.self"), nil
		}
		// Match by process NAME (comm), not full command line: `pkill -f` matches
		// any substring in any process's args — including the very command that
		// invoked us — so a stray arg could kill unintended processes. Process-name
		// matching is precise (code, chrome, spotify) and won't hit the daemon.
		if err := s.run.run(ctx, "pkill", app); err != nil {
			return i18n.T("system.close.fail", app), nil
		}
		return i18n.T("system.close.ok", app), nil

	default:
		return "", fmt.Errorf("system: unknown action %q", action)
	}
}

// player runs a playerctl command against the active MPRIS player.
func (s *System) player(ctx context.Context, verb, ok string) (string, error) {
	if err := s.run.run(ctx, "playerctl", verb); err != nil {
		return i18n.T("system.media.no_player"), nil
	}
	return ok, nil
}

// cmd is a command + args pair for tryFirst.
type cmd struct {
	name string
	args []string
}

// tryFirst runs commands in order and reports whether any succeeded.
func (s *System) tryFirst(ctx context.Context, cmds ...cmd) bool {
	for _, c := range cmds {
		if s.run.run(ctx, c.name, c.args...) == nil {
			return true
		}
	}
	return false
}

// isSelf guards against closing Pylon itself.
func isSelf(app string) bool {
	a := strings.ToLower(strings.TrimSpace(app))
	return a == "pylon" || a == "pylon-ui"
}

// execRunner runs commands for real.
type execRunner struct{}

func (execRunner) run(ctx context.Context, name string, args ...string) error {
	out, err := exec.CommandContext(ctx, name, args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %w: %s", name, err, strings.TrimSpace(string(out)))
	}
	return nil
}
