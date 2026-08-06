package hotkey

import (
	"context"
	"fmt"
	"strings"
)

// hyprland binds through `hyprctl`, which reaches the running compositor over
// its control socket — no config file is read or written.
//
// Two dialects have to be handled. Hyprland's original hyprlang config takes
// `hyprctl keyword bind`; the Lua config that replaces it in 0.57 rejects that
// with "keyword can't work with non-legacy parsers. Use eval." and wants a Lua
// call instead. Which one a session accepts depends on the config the user
// wrote, not on the version, so we try the modern one and fall back.
type hyprland struct{ run runner }

func (h *hyprland) Name() string { return "Hyprland" }

func (h *hyprland) Bind(ctx context.Context, combo Combo, cmd string) error {
	// Drop our previous binding first: both dialects append rather than
	// replace, so changing the shortcut would otherwise leave the old one live.
	_ = h.Unbind(ctx, combo)

	lua := fmt.Sprintf(`hl.bind(%s, hl.dsp.exec_cmd(%s))`, luaString(hyprLuaCombo(combo)), luaString(cmd))
	legacy := fmt.Sprintf(`bind %s, %s, exec, %s`, strings.Join(combo.Mods, " "), combo.Key, cmd)
	return h.try(ctx,
		[]string{"eval", lua},
		[]string{"keyword", legacy},
	)
}

func (h *hyprland) Unbind(ctx context.Context, combo Combo) error {
	lua := fmt.Sprintf(`hl.unbind(%s)`, luaString(hyprLuaCombo(combo)))
	legacy := fmt.Sprintf(`unbind %s, %s`, strings.Join(combo.Mods, " "), combo.Key)
	return h.try(ctx,
		[]string{"eval", lua},
		[]string{"keyword", legacy},
	)
}

// try runs each hyprctl form until one reports success. hyprctl exits 0 even
// when it refuses the command, printing the reason instead, so the output is
// what has to be checked.
func (h *hyprland) try(ctx context.Context, forms ...[]string) error {
	var last string
	for _, args := range forms {
		out, err := h.run(ctx, "hyprctl", args...)
		if err == nil && isHyprOK(out) {
			return nil
		}
		last = out
		if err != nil && out == "" {
			last = err.Error()
		}
	}
	return fmt.Errorf("hyprctl: %s", last)
}

// isHyprOK reports whether hyprctl accepted the command. Success is a bare
// "ok"; refusals come back as prose ("keyword can't work with…") or "error: …".
func isHyprOK(out string) bool {
	return strings.EqualFold(strings.TrimSpace(out), "ok")
}

// hyprLuaCombo spells a combo the way hl.bind expects: "SUPER + SHIFT + P".
func hyprLuaCombo(c Combo) string {
	return strings.Join(append(append([]string{}, c.Mods...), c.Key), " + ")
}

// luaString quotes a Go string as a Lua literal. The value reaches hyprctl as a
// single argv entry (no shell), so only Lua's own escaping matters.
func luaString(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`, "\r", `\r`)
	return `"` + r.Replace(s) + `"`
}
