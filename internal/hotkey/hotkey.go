// Package hotkey registers Pylon's push-to-talk shortcut with the running
// compositor.
//
// A desktop app cannot grab a global hotkey for itself on Wayland, so the
// binding has to live in the window manager. Editing the user's WM config to
// get one is a poor trade: it is their file, the edit has to be found again to
// change or remove it, and a bad write breaks their session. Both compositors
// handled here accept bindings over their control socket instead, so Pylon
// registers the shortcut at runtime and touches nothing on disk. The binding
// lasts as long as the compositor session; the daemon re-applies it on start.
package hotkey

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// Combo is a normalized shortcut: modifiers plus one key, e.g. SUPER+P.
type Combo struct {
	Mods []string // SUPER, CTRL, ALT, SHIFT — in that order
	Key  string   // single character upper-cased, or a named key like "space"
}

// modOrder is the canonical modifier order, so the same shortcut always
// produces the same string and unbinding matches what was bound.
var modOrder = []string{"SUPER", "CTRL", "ALT", "SHIFT"}

// modAliases maps what users and other config formats call each modifier onto
// the canonical name.
var modAliases = map[string]string{
	"SUPER": "SUPER", "MOD4": "SUPER", "WIN": "SUPER", "CMD": "SUPER", "META": "SUPER",
	"CTRL": "CTRL", "CONTROL": "CTRL",
	"ALT": "ALT", "MOD1": "ALT",
	"SHIFT": "SHIFT",
}

// Parse reads a combo written as "SUPER+P" or "super + shift + space".
func Parse(s string) (Combo, error) {
	var c Combo
	for _, part := range strings.Split(s, "+") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if mod, ok := modAliases[strings.ToUpper(part)]; ok {
			if !contains(c.Mods, mod) {
				c.Mods = append(c.Mods, mod)
			}
			continue
		}
		if c.Key != "" {
			return Combo{}, fmt.Errorf("hotkey %q: birden fazla tuş var (%q ve %q)", s, c.Key, part)
		}
		c.Key = normalizeKey(part)
	}
	if c.Key == "" {
		return Combo{}, fmt.Errorf("hotkey %q: tuş yok", s)
	}
	c.Mods = sortMods(c.Mods)
	return c, nil
}

// normalizeKey upper-cases single characters so "p" and "P" are one shortcut.
// Longer names are left exactly as written: they span function keys (F5),
// keysyms (space, Print) and media keys (XF86AudioPlay), and folding their case
// would only risk mangling one of those. Unbind matches on the stored string,
// so preserving it keeps bind and unbind in agreement.
func normalizeKey(k string) string {
	if len([]rune(k)) == 1 {
		return strings.ToUpper(k)
	}
	return k
}

func sortMods(mods []string) []string {
	out := make([]string, 0, len(mods))
	for _, m := range modOrder {
		if contains(mods, m) {
			out = append(out, m)
		}
	}
	return out
}

func contains(xs []string, x string) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}

// String renders the canonical form, which is also what Parse accepts.
func (c Combo) String() string {
	return strings.Join(append(append([]string{}, c.Mods...), c.Key), "+")
}

// Manager binds and unbinds a shortcut in one compositor.
type Manager interface {
	// Name identifies the compositor, for logs and for telling the user which
	// path handled their shortcut.
	Name() string
	// Bind registers combo to run cmd, replacing any binding Pylon made before.
	Bind(ctx context.Context, combo Combo, cmd string) error
	// Unbind removes a binding Pylon made.
	Unbind(ctx context.Context, combo Combo) error
}

// runner executes a command and returns its combined output. Swapped in tests.
type runner func(ctx context.Context, name string, args ...string) (string, error)

func execRun(ctx context.Context, name string, args ...string) (string, error) {
	out, err := exec.CommandContext(ctx, name, args...).CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

// Detect returns a Manager for the compositor in this session, or nil when
// there is no way to register a binding at runtime — every other desktop
// (GNOME, KDE, macOS, Windows) needs the user to add it themselves.
func Detect() Manager {
	switch {
	case os.Getenv("HYPRLAND_INSTANCE_SIGNATURE") != "":
		return &hyprland{run: execRun}
	case os.Getenv("SWAYSOCK") != "":
		return &sway{run: execRun}
	}
	return nil
}
