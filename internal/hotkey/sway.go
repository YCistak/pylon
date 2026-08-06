package hotkey

import (
	"context"
	"fmt"
	"strings"
)

// sway binds through `swaymsg`, which talks to the running compositor over its
// IPC socket. Unlike Hyprland, `bindsym` replaces an existing binding for the
// same combo, so no explicit unbind is needed before rebinding.
type sway struct{ run runner }

func (s *sway) Name() string { return "Sway" }

func (s *sway) Bind(ctx context.Context, combo Combo, cmd string) error {
	return s.exec(ctx, fmt.Sprintf("bindsym %s exec %s", swayCombo(combo), cmd))
}

func (s *sway) Unbind(ctx context.Context, combo Combo) error {
	return s.exec(ctx, fmt.Sprintf("unbindsym %s", swayCombo(combo)))
}

func (s *sway) exec(ctx context.Context, command string) error {
	out, err := s.run(ctx, "swaymsg", "-t", "command", command)
	if err != nil {
		if out != "" {
			return fmt.Errorf("swaymsg: %s", out)
		}
		return fmt.Errorf("swaymsg: %w", err)
	}
	// swaymsg reports failures in its JSON reply while still exiting 0.
	if strings.Contains(out, `"success": false`) || strings.Contains(out, `"success":false`) {
		return fmt.Errorf("swaymsg: %s", out)
	}
	return nil
}

// swayModifiers are sway's names for the modifiers; the key itself stays as
// written, since sway matches keysyms like "p" or "space" case-insensitively.
var swayModifiers = map[string]string{
	"SUPER": "Mod4",
	"CTRL":  "Control",
	"ALT":   "Mod1",
	"SHIFT": "Shift",
}

func swayCombo(c Combo) string {
	parts := make([]string, 0, len(c.Mods)+1)
	for _, m := range c.Mods {
		parts = append(parts, swayModifiers[m])
	}
	return strings.Join(append(parts, strings.ToLower(c.Key)), "+")
}
