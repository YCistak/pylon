package hotkey

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestParseNormalizes(t *testing.T) {
	cases := map[string]string{
		"SUPER+P":              "SUPER+P",
		"super + p":            "SUPER+P",
		"shift+super+p":        "SUPER+SHIFT+P", // canonical modifier order
		"mod4+alt+space":       "SUPER+ALT+space",
		"cmd+K":                "SUPER+K",
		"CONTROL + SHIFT + F5": "CTRL+SHIFT+F5",
	}
	for in, want := range cases {
		got, err := Parse(in)
		if err != nil {
			t.Fatalf("Parse(%q): %v", in, err)
		}
		if got.String() != want {
			t.Fatalf("Parse(%q) = %q, want %q", in, got.String(), want)
		}
	}
}

// Round-tripping matters: the stored string is what Unbind is later asked to
// match, so String() must produce something Parse accepts unchanged.
func TestParseRoundTrips(t *testing.T) {
	for _, in := range []string{"SUPER+P", "SUPER+SHIFT+space", "CTRL+ALT+DELETE"} {
		c, err := Parse(in)
		if err != nil {
			t.Fatalf("Parse(%q): %v", in, err)
		}
		again, err := Parse(c.String())
		if err != nil || again.String() != c.String() {
			t.Fatalf("round trip of %q gave %q (%v)", in, again.String(), err)
		}
	}
}

func TestParseRejectsBadInput(t *testing.T) {
	for _, in := range []string{"", "SUPER", "SUPER+", "SUPER+P+Q"} {
		if c, err := Parse(in); err == nil {
			t.Fatalf("Parse(%q) should have failed, got %q", in, c.String())
		}
	}
}

// fakeRunner records the commands issued and replies with scripted output.
type fakeRunner struct {
	calls []string
	// reply maps a substring of the arguments onto the output to return.
	reply func(args []string) (string, error)
}

func (f *fakeRunner) run(_ context.Context, name string, args ...string) (string, error) {
	f.calls = append(f.calls, name+" "+strings.Join(args, " "))
	if f.reply != nil {
		return f.reply(args)
	}
	return "ok", nil
}

func TestHyprlandBindUsesLuaFirst(t *testing.T) {
	f := &fakeRunner{}
	h := &hyprland{run: f.run}

	combo, _ := Parse("SUPER+P")
	if err := h.Bind(context.Background(), combo, "/opt/pylon listen"); err != nil {
		t.Fatalf("bind: %v", err)
	}

	// Unbind first, then bind — both through the Lua dialect, so the legacy
	// form is never reached on a session that accepts eval.
	joined := strings.Join(f.calls, "\n")
	if !strings.Contains(joined, `eval hl.unbind("SUPER + P")`) {
		t.Fatalf("no unbind before bind:\n%s", joined)
	}
	if !strings.Contains(joined, `eval hl.bind("SUPER + P", hl.dsp.exec_cmd("/opt/pylon listen"))`) {
		t.Fatalf("bind not issued as Lua:\n%s", joined)
	}
	if strings.Contains(joined, "keyword") {
		t.Fatalf("legacy form used even though eval succeeded:\n%s", joined)
	}
}

// Hyprland's hyprlang parser refuses `eval`; the legacy `keyword` form has to
// take over. hyprctl exits 0 either way, so only the output distinguishes them.
func TestHyprlandFallsBackToKeyword(t *testing.T) {
	f := &fakeRunner{reply: func(args []string) (string, error) {
		if args[0] == "eval" {
			return "error: eval is not supported", nil
		}
		return "ok", nil
	}}
	h := &hyprland{run: f.run}

	combo, _ := Parse("SUPER+P")
	if err := h.Bind(context.Background(), combo, "pylon listen"); err != nil {
		t.Fatalf("bind: %v", err)
	}
	if !strings.Contains(strings.Join(f.calls, "\n"), "keyword bind SUPER, P, exec, pylon listen") {
		t.Fatalf("legacy form not used:\n%s", strings.Join(f.calls, "\n"))
	}
}

// The real refusal seen on Hyprland 0.56 with a Lua config, in the other
// direction: `keyword` is rejected in prose while exiting 0.
func TestHyprlandTreatsProseRefusalAsFailure(t *testing.T) {
	f := &fakeRunner{reply: func([]string) (string, error) {
		return "keyword can't work with non-legacy parsers. Use eval.", nil
	}}
	h := &hyprland{run: f.run}

	combo, _ := Parse("SUPER+P")
	err := h.Bind(context.Background(), combo, "pylon listen")
	if err == nil {
		t.Fatal("a refusal that exits 0 must still be an error")
	}
	if !strings.Contains(err.Error(), "non-legacy") {
		t.Fatalf("error should carry hyprctl's reason, got %v", err)
	}
}

func TestHyprlandQuotesLuaStrings(t *testing.T) {
	f := &fakeRunner{}
	h := &hyprland{run: f.run}

	combo, _ := Parse("SUPER+P")
	// A path with a quote and a backslash must not break out of the literal.
	if err := h.Bind(context.Background(), combo, `/o"pt\pylon listen`); err != nil {
		t.Fatalf("bind: %v", err)
	}
	if !strings.Contains(strings.Join(f.calls, "\n"), `exec_cmd("/o\"pt\\pylon listen")`) {
		t.Fatalf("lua string not escaped:\n%s", strings.Join(f.calls, "\n"))
	}
}

func TestSwayBind(t *testing.T) {
	f := &fakeRunner{reply: func([]string) (string, error) { return `[{"success": true}]`, nil }}
	s := &sway{run: f.run}

	combo, _ := Parse("SUPER+SHIFT+P")
	if err := s.Bind(context.Background(), combo, "pylon listen"); err != nil {
		t.Fatalf("bind: %v", err)
	}
	if !strings.Contains(f.calls[0], "bindsym Mod4+Shift+p exec pylon listen") {
		t.Fatalf("unexpected command: %s", f.calls[0])
	}
}

// swaymsg reports a refusal inside its JSON reply while exiting 0.
func TestSwayReportsJSONFailure(t *testing.T) {
	f := &fakeRunner{reply: func([]string) (string, error) {
		return `[{"success": false, "error": "unknown key"}]`, nil
	}}
	s := &sway{run: f.run}

	combo, _ := Parse("SUPER+P")
	if err := s.Bind(context.Background(), combo, "pylon listen"); err == nil {
		t.Fatal("a JSON failure must surface as an error")
	}
}

func TestSwaySurfacesExecError(t *testing.T) {
	f := &fakeRunner{reply: func([]string) (string, error) {
		return "", errors.New("swaymsg bulunamadı")
	}}
	s := &sway{run: f.run}

	combo, _ := Parse("SUPER+P")
	if err := s.Bind(context.Background(), combo, "pylon listen"); err == nil {
		t.Fatal("expected the exec failure to surface")
	}
}

// Detect must stay silent on desktops with no runtime binding API, so callers
// fall back to telling the user how to add it themselves.
func TestDetectWithoutCompositor(t *testing.T) {
	t.Setenv("HYPRLAND_INSTANCE_SIGNATURE", "")
	t.Setenv("SWAYSOCK", "")
	if m := Detect(); m != nil {
		t.Fatalf("Detect() = %s, want nil", m.Name())
	}
}

func TestDetectHyprland(t *testing.T) {
	t.Setenv("SWAYSOCK", "")
	t.Setenv("HYPRLAND_INSTANCE_SIGNATURE", "abc123")
	m := Detect()
	if m == nil || m.Name() != "Hyprland" {
		t.Fatalf("Detect() = %v", m)
	}
}
